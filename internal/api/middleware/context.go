// Package middleware — Middleware layer VampiFox.
//
// Middleware VampiFox mengikuti konvensi penamaan tematik:
//
//   - Bloodgate  — autentikasi JWT (siapa kamu?)
//   - Covenant   — otorisasi RBAC (boleh tidak?)
//   - Territory  — resolusi tenant (wilayah mana?)
//   - RequestID  — inject request ID untuk tracing
//   - Recovery   — panic recovery
//
// Urutan middleware yang benar pada route yang butuh auth:
//
//	router.Use(RequestID())
//	router.Use(Recovery(logger))
//	router.Use(Territory(resolver))   // resolve tenant dulu
//	router.Use(Bloodgate(authSvc))    // auth pakai tenant context
//	route.Use(Covenant(covenant, perm)) // cek permission
package middleware

import (
	"github.com/aditya-lucis/vampifox/internal/core/auth"
	"github.com/aditya-lucis/vampifox/internal/core/rbac"
	"github.com/aditya-lucis/vampifox/internal/core/tenant"
	"github.com/gin-gonic/gin"
)

// ═══════════════════════════════════════════════════════════════
//  Context keys — konstanta untuk gin.Context keys
// ═══════════════════════════════════════════════════════════════

// Key-key yang digunakan untuk menyimpan nilai di gin.Context.
// Selalu gunakan konstanta ini — jangan hardcode string.
const (
	KeyBloodClaims = "vfx_blood_claims" // *auth.BloodClaims
	KeyRequestID   = "vfx_request_id"   // string
	KeyTenantSlug  = "vfx_tenant_slug"  // string
)

// ── Context accessors ─────────────────────────────────────────────

// GetBloodClaims mengambil BloodClaims dari gin.Context.
// Mengembalikan nil jika Bloodgate middleware belum dijalankan.
func GetBloodClaims(c *gin.Context) *auth.BloodClaims {
	val, exists := c.Get(KeyBloodClaims)
	if !exists {
		return nil
	}
	claims, _ := val.(*auth.BloodClaims)
	return claims
}

// GetRequestID mengambil request ID dari gin.Context.
func GetRequestID(c *gin.Context) string {
	val, _ := c.Get(KeyRequestID)
	id, _ := val.(string)
	return id
}

// GetTenant mengambil *tenant.Tenant dari request context.
// Mengembalikan nil jika Territory middleware belum dijalankan.
func GetTenant(c *gin.Context) *tenant.Tenant {
	t, ok := tenant.FromContext(c.Request.Context())
	if !ok {
		return nil
	}
	return t
}

// GetRoles mengambil roles user dari BloodClaims.
func GetRoles(c *gin.Context) []string {
	claims := GetBloodClaims(c)
	if claims == nil {
		return nil
	}
	return claims.Roles
}

// GetUserID mengambil user ID dari BloodClaims.
func GetUserID(c *gin.Context) string {
	claims := GetBloodClaims(c)
	if claims == nil {
		return ""
	}
	return claims.UserID.String()
}

// ── Response helpers ──────────────────────────────────────────────

// ErrorResponse adalah format error response yang konsisten.
type ErrorResponse struct {
	Success   bool        `json:"success"`
	Error     ErrorDetail `json:"error"`
	RequestID string      `json:"request_id,omitempty"`
}

// ErrorDetail detail error.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// newErrorResponse membuat ErrorResponse yang konsisten.
func newErrorResponse(c *gin.Context, code, message string) ErrorResponse {
	return ErrorResponse{
		Success: false,
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
		RequestID: GetRequestID(c),
	}
}

// ── unused import suppression ─────────────────────────────────────
var _ = rbac.RoleOverlord
