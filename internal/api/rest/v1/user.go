package v1

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/api/middleware"
	rest "github.com/aditya-lucis/vampifox/internal/api/response"
	"github.com/aditya-lucis/vampifox/internal/core/rbac"
	"github.com/aditya-lucis/vampifox/internal/core/tenant"
	"github.com/aditya-lucis/vampifox/internal/core/user"
)

// RegisterUserRoutes mendaftarkan route user management.
func RegisterUserRoutes(
	rg *gin.RouterGroup,
	userFactory *user.ServiceFactory,
	covenant *rbac.Covenant,
	logger *zap.Logger,
) {
	h := &userHandler{factory: userFactory, covenant: covenant, logger: logger}
	rg.GET("/me", h.getMe)
	rg.PATCH("/me", h.updateMe)
	rg.POST("/me/change-password", h.changePassword)
}

type userHandler struct {
	factory  *user.ServiceFactory
	covenant *rbac.Covenant
	logger   *zap.Logger
}

// svc membuat user.Service yang di-scope ke tenant dari context.
func (h *userHandler) svc(c *gin.Context) (*user.Service, func(), bool) {
	t := middleware.GetTenant(c)
	if t == nil {
		rest.InternalError(c, "Tenant context tidak tersedia.")
		return nil, func() {}, false
	}
	scope := tenant.NewScope(t)
	svc, release, err := h.factory.ForTenant(c.Request.Context(), scope)
	if err != nil {
		rest.InternalError(c, "Gagal menginisialisasi layanan.")
		return nil, func() {}, false
	}
	return svc, release, true
}

// ── GET /users/me ─────────────────────────────────────────────────

func (h *userHandler) getMe(c *gin.Context) {
	claims := middleware.GetBloodClaims(c)
	if claims == nil {
		rest.Unauthorized(c, "Tidak terauthentikasi.")
		return
	}

	svc, release, ok := h.svc(c)
	defer release()
	if !ok {
		return
	}

	u, err := svc.FindByID(c.Request.Context(), claims.UserID.String())
	if err != nil {
		if err == user.ErrNotFound {
			rest.NotFound(c, "User tidak ditemukan.")
			return
		}
		rest.InternalError(c, "Gagal mengambil data user.")
		return
	}

	rest.OK(c, gin.H{
		"id":         u.ID.String(),
		"email":      u.Email,
		"full_name":  u.FullName,
		"avatar":     u.Avatar,
		"roles":      u.Roles,
		"status":     u.Status,
		"last_login": u.LastLoginAt,
		"created_at": u.CreatedAt,
	})
}

// ── PATCH /users/me ───────────────────────────────────────────────

type updateMeRequest struct {
	FullName string `json:"full_name"`
	Avatar   string `json:"avatar"`
}

func (h *userHandler) updateMe(c *gin.Context) {
	claims := middleware.GetBloodClaims(c)
	if claims == nil {
		rest.Unauthorized(c, "Tidak terauthentikasi.")
		return
	}

	var req updateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		rest.BadRequest(c, "Format request tidak valid.")
		return
	}

	svc, release, ok := h.svc(c)
	defer release()
	if !ok {
		return
	}

	u, err := svc.UpdateProfile(c.Request.Context(), claims.UserID.String(), user.UpdateProfileInput{
		FullName: req.FullName,
		Avatar:   req.Avatar,
	})
	if err != nil {
		h.logger.Warn("UpdateProfile gagal",
			zap.String("user_id", claims.UserID.String()),
			zap.Error(err),
		)
		rest.BadRequest(c, err.Error())
		return
	}

	rest.OK(c, gin.H{
		"id":        u.ID.String(),
		"email":     u.Email,
		"full_name": u.FullName,
		"avatar":    u.Avatar,
	})
}

// ── POST /users/me/change-password ───────────────────────────────

type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

func (h *userHandler) changePassword(c *gin.Context) {
	claims := middleware.GetBloodClaims(c)
	if claims == nil {
		rest.Unauthorized(c, "Tidak terauthentikasi.")
		return
	}

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		rest.BadRequest(c, "old_password dan new_password wajib diisi.")
		return
	}

	svc, release, ok := h.svc(c)
	defer release()
	if !ok {
		return
	}

	err := svc.ChangePassword(c.Request.Context(), claims.UserID.String(), user.ChangePasswordInput{
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
	})
	if err != nil {
		switch err {
		case user.ErrWrongPassword:
			rest.BadRequest(c, "Password lama salah.")
		default:
			rest.BadRequest(c, err.Error())
		}
		return
	}

	rest.OK(c, gin.H{"message": "Password berhasil diganti."})
}
