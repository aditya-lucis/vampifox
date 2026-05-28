package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/api/middleware"
	rest "github.com/aditya-lucis/vampifox/internal/api/response"
	"github.com/aditya-lucis/vampifox/internal/core/auth"
	"github.com/aditya-lucis/vampifox/internal/core/user"
)

// RegisterAuthRoutes mendaftarkan semua route autentikasi.
func RegisterAuthRoutes(rg *gin.RouterGroup, factory *auth.ServiceFactory, logger *zap.Logger) {
	h := &authHandler{factory: factory, logger: logger}
	rg.POST("/login", h.login)
	rg.POST("/refresh", h.refresh)
	rg.POST("/logout", h.logout)
}

type authHandler struct {
	factory *auth.ServiceFactory
	logger  *zap.Logger
}

// svc membuat auth.Service yang di-scope ke tenant dari context.
// Dipanggil di awal setiap handler.
func (h *authHandler) svc(c *gin.Context) (*auth.Service, bool) {
	t := middleware.GetTenant(c)
	if t == nil {
		rest.InternalError(c, "Tenant context tidak tersedia.")
		return nil, false
	}
	return h.factory.ForTenant(t), true
}

// ── POST /auth/login ──────────────────────────────────────────────

type loginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    string    `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	User         userBrief `json:"user"`
}

type userBrief struct {
	ID       string   `json:"id"`
	Email    string   `json:"email"`
	FullName string   `json:"full_name"`
	Roles    []string `json:"roles"`
}

func (h *authHandler) login(c *gin.Context) {
	svc, ok := h.svc(c)
	if !ok {
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		rest.BadRequest(c, "Format request tidak valid: "+err.Error())
		return
	}

	t := middleware.GetTenant(c)
	result, err := svc.Login(c.Request.Context(), t.ID, t.TenantSlug, auth.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		h.logger.Debug("Login gagal", zap.String("email", req.Email), zap.Error(err))
		loginErrToHTTP(c, err)
		return
	}

	rest.OK(c, loginResponse{
		AccessToken:  result.Tokens.AccessToken,
		RefreshToken: result.Tokens.RefreshToken,
		ExpiresAt:    result.Tokens.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		TokenType:    result.Tokens.TokenType,
		User: userBrief{
			ID:       result.User.ID.String(),
			Email:    result.User.Email,
			FullName: result.User.FullName,
			Roles:    result.User.Roles,
		},
	})
}

// ── POST /auth/refresh ────────────────────────────────────────────

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *authHandler) refresh(c *gin.Context) {
	svc, ok := h.svc(c)
	if !ok {
		return
	}

	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		rest.BadRequest(c, "refresh_token wajib diisi.")
		return
	}

	t := middleware.GetTenant(c)
	tokens, err := svc.Refresh(c.Request.Context(), req.RefreshToken, t.ID, t.TenantSlug)
	if err != nil {
		loginErrToHTTP(c, err)
		return
	}

	rest.OK(c, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_at":    tokens.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		"token_type":    tokens.TokenType,
	})
}

// ── POST /auth/logout ─────────────────────────────────────────────

type logoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *authHandler) logout(c *gin.Context) {
	svc, ok := h.svc(c)
	if !ok {
		return
	}

	var req logoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		rest.BadRequest(c, "refresh_token wajib diisi.")
		return
	}

	if err := svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		h.logger.Warn("Logout error (non-fatal)", zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Berhasil logout. Sampai malam berikutnya.",
	})
}

// ── Error mapping ─────────────────────────────────────────────────

func loginErrToHTTP(c *gin.Context, err error) {
	switch err {
	case user.ErrWrongPassword:
		rest.Unauthorized(c, "Email atau password salah.")
	case user.ErrInactive:
		rest.Forbidden(c, "Akun tidak aktif. Hubungi administrator.")
	case auth.ErrTokenExpired:
		rest.Unauthorized(c, "Token kadaluarsa. Minta token baru.")
	case auth.ErrTokenRevoked:
		rest.Unauthorized(c, "Sesi dicabut. Silakan login ulang.")
	case auth.ErrInvalidToken:
		rest.Unauthorized(c, "Token tidak valid.")
	default:
		rest.InternalError(c, "Terjadi kesalahan. Silakan coba lagi.")
	}
}
