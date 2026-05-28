package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/core/tenant"
)

// ═══════════════════════════════════════════════════════════════
//  Territory — resolusi tenant dari HTTP request
// ═══════════════════════════════════════════════════════════════

// Territory middleware mengidentifikasi tenant dari HTTP request
// dan menyuntikkannya ke context.
//
// Jika tenant tidak ditemukan atau tidak aktif, request langsung
// ditolak sebelum sampai ke handler.
//
// Resolver mencoba tiga strategi berurutan:
//  1. Header X-VampiFox-Tenant
//  2. Subdomain (e.g. pt-maju.vampifox.io)
//  3. Custom domain (e.g. erp.pt-maju.com)
func Territory(resolver *tenant.Resolver, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := resolver.Resolve(c.Request.Context(), c.Request)
		if err != nil {
			status, code, msg := tenantErrToHTTP(err)
			logger.Debug("Territory: tenant tidak ditemukan",
				zap.String("host", c.Request.Host),
				zap.String("header", c.GetHeader("X-VampiFox-Tenant")),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(status, newErrorResponse(c, code, msg))
			return
		}

		// Inject tenant ke context
		ctx := tenant.WithTenant(c.Request.Context(), result.Tenant)
		c.Request = c.Request.WithContext(ctx)
		c.Set(KeyTenantSlug, result.Tenant.TenantSlug)

		logger.Debug("Territory: tenant ditemukan",
			zap.String("slug", result.Tenant.TenantSlug),
			zap.String("strategy", result.Strategy),
		)

		c.Next()
	}
}

// tenantErrToHTTP mengkonversi error tenant ke HTTP status + kode.
func tenantErrToHTTP(err error) (status int, code, message string) {
	switch {
	case errors.Is(err, tenant.ErrNotFound):
		return http.StatusNotFound,
			"UNKNOWN_TERRITORY",
			"Wilayah kekuasaan tidak dikenal."

	case errors.Is(err, tenant.ErrSuspended):
		return http.StatusForbidden,
			"TERRITORY_SUSPENDED",
			"Wilayah ini sedang disuspend. Hubungi administrator."

	case errors.Is(err, tenant.ErrExpired):
		return http.StatusPaymentRequired,
			"TERRITORY_EXPIRED",
			"Masa berlaku wilayah ini telah habis. Perpanjang langganan Anda."

	default:
		return http.StatusInternalServerError,
			"TERRITORY_ERROR",
			"Terjadi kesalahan saat mengidentifikasi wilayah."
	}
}
