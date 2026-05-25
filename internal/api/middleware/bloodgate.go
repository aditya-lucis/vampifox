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
// "Gerbang darah" — hanya yang membawa token sah yang boleh masuk.
//
// Bloodgate:
//  1. Ambil token dari header Authorization: Bearer <token>
//  2. Validasi signature dan expiry via auth.Service
//  3. Cek blacklist (token revoked / user logged out all devices)
//  4. Inject BloodClaims ke gin.Context
//
// Bloodgate HARUS dipasang setelah Territory middleware karena
// validasi token membutuhkan tenant context.
//
// Contoh penggunaan:
//
//	v1 := router.Group("/api/v1")
//	v1.Use(Territory(resolver, logger))
//	v1.Use(Bloodgate(authSvc, logger))
func Bloodgate(authSvc *auth.Service, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := extractBearerToken(c.Request)
		if err != nil {
			logger.Debug("Bloodgate: token tidak ada atau format salah",
				zap.String("path", c.Request.URL.Path),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, newErrorResponse(c,
				"NOT_INVITED",
				"Kamu tidak diundang — sertakan Bearer token yang valid.",
			))
			return
		}

		// Validasi token + cek blacklist
		claims, err := authSvc.ValidateAccessToken(c.Request.Context(), tokenStr)
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

// BloodgateOptional seperti Bloodgate tapi tidak menolak request
// jika token tidak ada. Berguna untuk endpoint yang bisa diakses
// oleh user anonymous maupun yang sudah login.
//
// Handler bisa cek dengan GetBloodClaims(c) — nil berarti anonymous.
func BloodgateOptional(authSvc *auth.Service, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := extractBearerToken(c.Request)
		if err != nil {
			// Tidak ada token — lanjut sebagai anonymous
			c.Next()
			return
		}

		claims, err := authSvc.ValidateAccessToken(c.Request.Context(), tokenStr)
		if err != nil {
			// Token ada tapi tidak valid — tetap lanjut (optional)
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

// extractBearerToken mengambil token dari header Authorization.
// Format yang diterima: "Authorization: Bearer <token>"
func extractBearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", errors.New("header Authorization tidak ada")
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return "", errors.New("format Authorization header salah")
	}

	scheme := strings.ToLower(parts[0])
	if scheme != "bearer" {
		return "", errors.New("scheme harus Bearer")
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("token kosong")
	}

	return token, nil
}

// authErrToHTTP mengkonversi auth error ke HTTP status yang sesuai.
func authErrToHTTP(err error) (status int, code, message string) {
	switch {
	case errors.Is(err, auth.ErrTokenExpired):
		return http.StatusUnauthorized,
			"INVITATION_EXPIRED",
			"Token sudah kadaluarsa. Minta token baru via /auth/refresh."

	case errors.Is(err, auth.ErrTokenRevoked):
		return http.StatusUnauthorized,
			"INVITATION_REVOKED",
			"Token sudah dicabut. Silakan login ulang."

	case errors.Is(err, auth.ErrInvalidToken):
		return http.StatusUnauthorized,
			"INVALID_INVITATION",
			"Token tidak valid."

	default:
		return http.StatusUnauthorized,
			"NOT_INVITED",
			"Autentikasi gagal."
	}
}
