// Package middleware — Middleware layer VampiFox.
// "Bloodgate" = gerbang darah, pemeriksaan identitas sebelum masuk.
package middleware

import (
	"net/http"
	"strings"
	"vampifox/internal/core/auth"
	"vampifox/internal/core/rbac"
	"vampifox/internal/core/tenant"

	"github.com/gin-gonic/gin"
)

// Bloodgate middleware autentikasi JWT.
// Hanya yang membawa "undangan sah" (JWT valid) yang boleh masuk.
func Bloodgate(sanctum *auth.Sanctum) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")

		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Kamu tidak diundang — sertakan Bearer token.",
				"code":  "NOT_INVITED",
			})
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := sanctum.Verify(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": err.Error(),
				"code":  "INVITATION_EXPIRED",
			})
			return
		}

		// Suntikkan tenant & user info ke context
		c.Set("blood_claims", claims)
		c.Set("tenant_id", claims.TenantID.String())
		c.Set("user_id", claims.UserID.String())
		c.Set("user_roles", claims.Roles)

		c.Next()
	}
}

// Covenant middleware otorisasi — cek apakah role punya permission.
// "Perjanjian" apa yang berlaku untuk rute ini?
func Covenant(permission rbac.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles, exists := c.Get("user_roles")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Identitasmu tidak dikenali.",
				"code":  "UNKNOWN_BLOODLINE",
			})
			return
		}

		roleSlice, ok := roles.([]string)
		if !ok || !rbac.CanEnter(roleSlice, permission) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":    "Kamu tidak punya kekuatan untuk ini.",
				"code":     "INSUFFICIENT_BLOODLINE",
				"required": permission,
			})
			return
		}

		c.Next()
	}
}

// TenantResolver middleware resolusi tenant dari subdomain atau header.
// "vf-tenant: pt-maju-jaya" atau "pt-maju-jaya.vampifox.com"
func TenantResolver() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Coba dari header dulu
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
				"error": "Wilayah kekuasaan tidak dikenali.",
				"code":  "UNKNOWN_TERRITORY",
			})
			return
		}

		// TODO: load tenant dari DB/cache berdasarkan slug
		// sementara mock
		t := &tenant.Tenant{
			Slug: slug,
		}

		ctx := tenant.WithTenant(
			c.Request.Context(),
			t,
		)

		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant_slug", slug)

		c.Next()
	}
}
