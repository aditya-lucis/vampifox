package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ═══════════════════════════════════════════════════════════════
//  Logger — HTTP request/response logging
// ═══════════════════════════════════════════════════════════════

// Logger middleware mencatat setiap request dan response
// dengan structured logging via zap.
//
// Setiap log entry menyertakan:
//   - request_id untuk tracing
//   - method, path, status, latency
//   - tenant slug (jika sudah di-resolve)
//   - user_id (jika sudah terauthentikasi)
//   - client IP
//
// Pasang setelah RequestID() supaya request_id sudah tersedia.
func Logger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// Kumpulkan fields
		fields := []zap.Field{
			zap.String("request_id", GetRequestID(c)),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("ip", c.ClientIP()),
		}

		if query != "" {
			fields = append(fields, zap.String("query", query))
		}

		// Tambah tenant jika sudah di-resolve
		if slug, ok := c.Get(KeyTenantSlug); ok {
			fields = append(fields, zap.Any("tenant", slug))
		}

		// Tambah user jika sudah terauthentikasi
		if userID := GetUserID(c); userID != "" {
			fields = append(fields, zap.String("user_id", userID))
		}

		// Catat error jika ada
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
		}

		// Log level berdasarkan status code
		switch {
		case status >= 500:
			logger.Error("Request selesai", fields...)
		case status >= 400:
			logger.Warn("Request selesai", fields...)
		default:
			logger.Info("Request selesai", fields...)
		}
	}
}
