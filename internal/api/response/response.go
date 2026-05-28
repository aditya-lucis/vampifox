// Package rest — REST API layer VampiFox.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/aditya-lucis/vampifox/internal/api/middleware"
)

// ═══════════════════════════════════════════════════════════════
//  Response envelope — format standar semua response VampiFox
// ═══════════════════════════════════════════════════════════════

// Response adalah envelope sukses.
//
//	{
//	  "success": true,
//	  "data": { ... },
//	  "meta": { "page": 1, "total": 354 },
//	  "request_id": "vfx-..."
//	}
type Response struct {
	Success   bool   `json:"success"`
	Data      any    `json:"data,omitempty"`
	Meta      *Meta  `json:"meta,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// Meta informasi pagination.
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// ErrorResponse adalah envelope error.
//
//	{
//	  "success": false,
//	  "error": { "code": "NOT_FOUND", "message": "...", "field": "email" },
//	  "request_id": "vfx-..."
//	}
type ErrorResponse struct {
	Success   bool        `json:"success"`
	Error     ErrorDetail `json:"error"`
	RequestID string      `json:"request_id,omitempty"`
}

// ErrorDetail detail error.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"` // diisi jika validation error
}

// ── Response helpers ──────────────────────────────────────────────

// OK mengirim response 200 dengan data.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Success:   true,
		Data:      data,
		RequestID: middleware.GetRequestID(c),
	})
}

// Created mengirim response 201.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Response{
		Success:   true,
		Data:      data,
		RequestID: middleware.GetRequestID(c),
	})
}

// Paginated mengirim response 200 dengan data dan metadata pagination.
func Paginated(c *gin.Context, data any, meta Meta) {
	c.JSON(http.StatusOK, Response{
		Success:   true,
		Data:      data,
		Meta:      &meta,
		RequestID: middleware.GetRequestID(c),
	})
}

// NoContent mengirim response 204 tanpa body.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// ── Error helpers ─────────────────────────────────────────────────

func errResp(c *gin.Context, status int, code, message, field string) {
	c.AbortWithStatusJSON(status, ErrorResponse{
		Success: false,
		Error: ErrorDetail{
			Code:    code,
			Message: message,
			Field:   field,
		},
		RequestID: middleware.GetRequestID(c),
	})
}

// BadRequest 400.
func BadRequest(c *gin.Context, message string) {
	errResp(c, http.StatusBadRequest, "BAD_REQUEST", message, "")
}

// ValidationError 400 dengan field.
func ValidationError(c *gin.Context, field, message string) {
	errResp(c, http.StatusBadRequest, "VALIDATION_ERROR", message, field)
}

// Unauthorized 401.
func Unauthorized(c *gin.Context, message string) {
	errResp(c, http.StatusUnauthorized, "UNAUTHORIZED", message, "")
}

// Forbidden 403.
func Forbidden(c *gin.Context, message string) {
	errResp(c, http.StatusForbidden, "FORBIDDEN", message, "")
}

// NotFound 404.
func NotFound(c *gin.Context, message string) {
	errResp(c, http.StatusNotFound, "NOT_FOUND", message, "")
}

// Conflict 409.
func Conflict(c *gin.Context, message string) {
	errResp(c, http.StatusConflict, "CONFLICT", message, "")
}

// InternalError 500.
func InternalError(c *gin.Context, message string) {
	errResp(c, http.StatusInternalServerError, "INTERNAL_ERROR", message, "")
}

// NewMeta membuat Meta dari hasil query berhalaman.
func NewMeta(page, limit int, total int64) Meta {
	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}
	return Meta{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
