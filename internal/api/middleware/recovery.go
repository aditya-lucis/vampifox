package middleware

import (
	"errors"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ═══════════════════════════════════════════════════════════════
//  Recovery — panic recovery middleware
// ═══════════════════════════════════════════════════════════════

// Recovery middleware menangkap panic di handler dan mengubahnya
// menjadi response 500 yang proper, alih-alih crash seluruh server.
//
// Setiap panic di-log dengan stack trace lengkap untuk debugging.
// Response yang dikembalikan ke client tidak menyertakan detail internal.
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				// Ambil stack trace
				stack := debug.Stack()

				// Konversi ke error
				var err error
				switch v := rec.(type) {
				case error:
					err = v
				case string:
					err = errors.New(v)
				default:
					err = errors.New("unknown panic")
				}

				// Log dengan stack trace — ini penting untuk debugging
				logger.Error("PANIC — request handler crashed",
					zap.String("request_id", GetRequestID(c)),
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.Error(err),
					zap.ByteString("stack", stack),
				)

				// Jangan bocorkan detail internal ke client
				c.AbortWithStatusJSON(http.StatusInternalServerError, newErrorResponse(c,
					"INTERNAL_ERROR",
					"Terjadi kesalahan internal. Tim kami sudah diberitahu.",
				))
			}
		}()
		c.Next()
	}
}
