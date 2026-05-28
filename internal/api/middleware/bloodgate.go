package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/core/auth"
)

// ═══════════════════════════════════════════════════════════════
//  Bloodgate — autentikasi JWT
// ═══════════════════════════════════════════════════════════════

// Bloodgate middleware memverifikasi JWT access token di setiap request.
//
// Menerima *auth.TokenValidator — bukan *auth.Service — karena
// validasi token tidak butuh database, hanya Sanctum + Redis.
// Ini membuat Bloodgate aman sebagai singleton middleware.
//
// Urutan middleware yang benar:
//
//	Territory → Bloodgate → Covenant → Handler
func Bloodgate(validator *auth.TokenValidator, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := extractBearerToken(c.Request)
		if err != nil {
			logger.Debug("Bloodgate: token tidak ada",
				zap.String("path", c.Request.URL.Path),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, newErrorResponse(c,
				"NOT_INVITED",
				"Sertakan Bearer token yang valid di header Authorization.",
			))
			return
		}

		claims, err := validator.ValidateAccessToken(c.Request.Context(), tokenStr)
		if err != nil {
			status, code, msg := authErrToHTTP(err)
			logger.Debug("Bloodgate: token ditolak",
				zap.String("path", c.Request.URL.Path),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(status, newErrorResponse(c, code, msg))
			return
		}

		// Inject claims ke context
		c.Set(KeyBloodClaims, claims)

		logger.Debug("Bloodgate: akses diterima",
			zap.String("user_id", claims.UserID.String()),
			zap.String("tenant", claims.TenantSlug),
			zap.Strings("roles", claims.Roles),
		)

		c.Next()
	}
}

// BloodgateOptional seperti Bloodgate tapi tidak reject jika token tidak ada.
func BloodgateOptional(validator *auth.TokenValidator, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := extractBearerToken(c.Request)
		if err != nil {
			c.Next()
			return
		}
		claims, err := validator.ValidateAccessToken(c.Request.Context(), tokenStr)
		if err != nil {
			logger.Debug("BloodgateOptional: token tidak valid, lanjut anonymous",
				zap.Error(err),
			)
			c.Next()
			return
		}
		c.Set(KeyBloodClaims, claims)
		c.Next()
	}
}

// ── Helpers ───────────────────────────────────────────────────────

func extractBearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", errors.New("Authorization header tidak ada")
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", errors.New("format harus: Bearer <token>")
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("token kosong")
	}
	return token, nil
}

func authErrToHTTP(err error) (status int, code, message string) {
	switch {
	case errors.Is(err, auth.ErrTokenExpired):
		return http.StatusUnauthorized, "INVITATION_EXPIRED",
			"Token kadaluarsa. Minta token baru via /auth/refresh."
	case errors.Is(err, auth.ErrTokenRevoked):
		return http.StatusUnauthorized, "INVITATION_REVOKED",
			"Token dicabut. Silakan login ulang."
	case errors.Is(err, auth.ErrInvalidToken):
		return http.StatusUnauthorized, "INVALID_INVITATION",
			"Token tidak valid."
	default:
		return http.StatusUnauthorized, "NOT_INVITED", "Autentikasi gagal."
	}
}
