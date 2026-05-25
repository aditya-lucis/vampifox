package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/core/rbac"
)

// ═══════════════════════════════════════════════════════════════
//  Covenant — otorisasi RBAC
// ═══════════════════════════════════════════════════════════════

// Covenant middleware memeriksa apakah user yang sudah terauthentikasi
// memiliki permission yang dibutuhkan untuk mengakses sebuah route.
// "Perjanjian" — menentukan apa yang boleh dan tidak boleh dilakukan.
//
// Covenant HARUS dipasang setelah Bloodgate middleware karena
// membutuhkan BloodClaims yang sudah diinject oleh Bloodgate.
//
// Contoh penggunaan:
//
//	// Hanya user dengan permission invoice.create
//	POST("/invoices", Covenant(covenant, "accounting:invoice.create"), handler)
//
//	// Bisa menggunakan permission builder
//	POST("/invoices", Covenant(covenant, rbac.Perm("accounting", "invoice", "create")), handler)
func Covenant(c *rbac.Covenant, required rbac.Permission, logger *zap.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		claims := GetBloodClaims(ctx)
		if claims == nil {
			// Bloodgate belum dijalankan — ini bug konfigurasi
			logger.Error("Covenant dipanggil tanpa Bloodgate — periksa urutan middleware",
				zap.String("path", ctx.Request.URL.Path),
			)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, newErrorResponse(ctx,
				"MIDDLEWARE_ORDER_ERROR",
				"Konfigurasi middleware tidak benar.",
			))
			return
		}

		if !c.Can(claims.Roles, required) {
			logger.Debug("Covenant: akses ditolak",
				zap.String("user_id", claims.UserID.String()),
				zap.String("permission", string(required)),
				zap.Strings("roles", claims.Roles),
			)
			ctx.AbortWithStatusJSON(http.StatusForbidden, newErrorResponse(ctx,
				"INSUFFICIENT_BLOODLINE",
				"Kamu tidak punya kekuatan untuk melakukan ini.",
			))
			return
		}

		ctx.Next()
	}
}

// CovenantAny seperti Covenant tapi cukup memiliki SALAH SATU dari permissions.
// Berguna untuk route yang bisa diakses oleh beberapa role berbeda.
//
// Contoh:
//
//	// Manager atau Admin bisa approve
//	PUT("/invoices/:id/approve",
//	    CovenantAny(covenant, "accounting:invoice.approve", "accounting:*"),
//	    handler,
//	)
func CovenantAny(c *rbac.Covenant, logger *zap.Logger, required ...rbac.Permission) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		claims := GetBloodClaims(ctx)
		if claims == nil {
			logger.Error("CovenantAny dipanggil tanpa Bloodgate",
				zap.String("path", ctx.Request.URL.Path),
			)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, newErrorResponse(ctx,
				"MIDDLEWARE_ORDER_ERROR",
				"Konfigurasi middleware tidak benar.",
			))
			return
		}

		if !c.CanAny(claims.Roles, required...) {
			ctx.AbortWithStatusJSON(http.StatusForbidden, newErrorResponse(ctx,
				"INSUFFICIENT_BLOODLINE",
				"Kamu tidak punya kekuatan untuk melakukan ini.",
			))
			return
		}

		ctx.Next()
	}
}

// CovenantRole middleware sederhana yang hanya cek role — tanpa permission granular.
// Berguna untuk route yang hanya boleh diakses role tertentu.
//
// Contoh:
//
//	// Hanya overlord atau elder_vampire
//	DELETE("/tenants/:id", CovenantRole(rbac.RoleOverlord, rbac.RoleElderVampire), handler)
func CovenantRole(allowedRoles ...rbac.Role) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[string(r)] = true
	}

	return func(c *gin.Context) {
		claims := GetBloodClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, newErrorResponse(c,
				"NOT_INVITED",
				"Autentikasi diperlukan.",
			))
			return
		}

		for _, role := range claims.Roles {
			if allowed[role] {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, newErrorResponse(c,
			"INSUFFICIENT_BLOODLINE",
			"Role kamu tidak diizinkan mengakses resource ini.",
		))
	}
}
