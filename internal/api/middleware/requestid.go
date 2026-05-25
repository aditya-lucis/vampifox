package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ═══════════════════════════════════════════════════════════════
//  RequestID — inject unique ID ke setiap request
// ═══════════════════════════════════════════════════════════════

// RequestID middleware menginjeksi unique request ID ke setiap request.
// ID ini dipakai untuk distributed tracing dan korelasi log.
//
// Prioritas sumber request ID:
//  1. Header X-Request-ID dari client (jika ada dan valid UUID)
//  2. Generate UUID baru
//
// Request ID di-set ke:
//   - gin.Context (via c.Set(KeyRequestID, id))
//   - Response header X-Request-ID
//
// Harus dipasang sebagai middleware PERTAMA supaya semua log
// downstream bisa menyertakan request ID.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Cek apakah client sudah kirim request ID
		requestID := c.GetHeader("X-Request-ID")

		// Validasi format — harus UUID yang valid
		if requestID == "" || !isValidUUID(requestID) {
			requestID = "vfx-" + uuid.New().String()
		}

		// Simpan ke context dan response header
		c.Set(KeyRequestID, requestID)
		c.Header("X-Request-ID", requestID)

		c.Next()
	}
}

// isValidUUID memeriksa apakah string adalah UUID yang valid.
func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
