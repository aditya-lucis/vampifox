package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/core/auth"
	"github.com/aditya-lucis/vampifox/internal/core/tenant"
	"github.com/gin-gonic/gin"
)

// Bloodgate middleware autentikasi JWT.
// Hanya yang membawa "undangan sah" (JWT valid) yang boleh masuk.
func Bloodgate(validator *auth.TokenValidator, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := extractBearerToken(c.Request)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "NOT_INVITED",
					"message": "Kamu tidak diundang — sertakan Bearer token.",
				},
			})
			return
		}

		claims, err := validator.ValidateAccessToken(c.Request.Context(), tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVITATION_EXPIRED",
					"message": "Undanganmu telah kedaluwarsa. Minta yang baru.",
				},
			})
			return
		}

		// Simpan claims ke context untuk handler berikutnya
		SetBloodClaims(c, claims)
		c.Next()
	}
}

// TenantResolver middleware resolusi tenant dari header atau subdomain.
//
// Urutan resolusi:
//  1. Header X-VampiFox-Tenant (prioritas utama, cocok untuk API client)
//  2. Subdomain, e.g. "pt-maju-jaya.vampifox.com" (cocok untuk web app)
//
// Setelah slug ditemukan, tenant di-load dari Shadow cache atau Fangs (DB).
// Tenant yang suspended atau expired akan ditolak dengan 403.
//
// Setelah middleware ini, handler bisa ambil tenant via:
//
//	t := tenant.MustFromContext(c.Request.Context())
func TenantResolver(svc *tenant.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── Resolusi slug ─────────────────────────────────────────
		slug := c.GetHeader("X-VampiFox-Tenant")

		// Fallback: ambil dari subdomain
		if slug == "" {
			host := c.Request.Host
			parts := strings.Split(host, ".")
			if len(parts) > 2 {
				slug = parts[0]
			}
		}

		if slug == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "UNKNOWN_TERRITORY",
					"message": "Wilayah kekuasaan tidak dikenali. Sertakan header X-VampiFox-Tenant.",
				},
			})
			return
		}

		// ── Load tenant dari cache / DB ───────────────────────────
		t, err := svc.FindBySlug(c.Request.Context(), slug)
		if err != nil {
			switch err {
			case tenant.ErrNotFound:
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "TERRITORY_NOT_FOUND",
						"message": "Tenant '" + slug + "' tidak ditemukan.",
					},
				})
			case tenant.ErrSuspended:
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "TERRITORY_SUSPENDED",
						"message": "Akses tenant ini sedang ditangguhkan.",
					},
				})
			case tenant.ErrExpired:
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "TERRITORY_EXPIRED",
						"message": "Masa berlaku tenant ini sudah habis.",
					},
				})
			default:
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "TERRITORY_ERROR",
						"message": "Gagal memuat data tenant.",
					},
				})
			}
			return
		}

		// ── Inject ke context ─────────────────────────────────────
		ctx := tenant.WithTenant(c.Request.Context(), t)
		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant_slug", t.Slug)

		c.Next()
	}
}

// extractBearerToken mengekstrak token dari Authorization header.
// Format yang diterima: "Bearer <token>"
// Mengembalikan error jika header kosong, tidak pakai Bearer scheme,
// atau token-nya kosong.
func extractBearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", fmt.Errorf("Authorization header tidak ada")
	}
	if !strings.HasPrefix(header, "Bearer ") {
		return "", fmt.Errorf("Authorization header bukan Bearer scheme")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" {
		return "", fmt.Errorf("Bearer token kosong")
	}
	return token, nil
}
