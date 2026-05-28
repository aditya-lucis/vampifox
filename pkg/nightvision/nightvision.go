// Package nightvision — Observability layer VampiFox.
//
// "NightVision" karena vampire bisa melihat jelas di kegelapan —
// bahkan saat sistem error atau lambat, NightVision tetap mencatat
// semuanya dengan presisi.
//
// NightVision menyediakan:
//   - Structured logger berbasis Zap (sudah ada di den/logger.go)
//   - Context propagation untuk request ID dan tenant
//   - Helper untuk log dengan context VampiFox
package nightvision

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ── Context keys ──────────────────────────────────────────────────

type contextKey string

const (
	loggerCtxKey    contextKey = "vfx_logger"
	requestIDCtxKey contextKey = "vfx_request_id"
	tenantCtxKey    contextKey = "vfx_tenant_slug"
	userIDCtxKey    contextKey = "vfx_user_id"
)

// ── Context helpers ───────────────────────────────────────────────

// WithLogger menyuntikkan logger ke context.
// Dipakai di Den untuk propagasi logger ke seluruh aplikasi.
func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey, logger)
}

// FromContext mengambil logger dari context.
// Jika tidak ada, mengembalikan zap.NewNop() (no-op logger).
func FromContext(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(loggerCtxKey).(*zap.Logger); ok && l != nil {
		return l
	}
	return zap.NewNop()
}

// WithRequestID menyuntikkan request ID ke context dan logger.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	ctx = context.WithValue(ctx, requestIDCtxKey, requestID)
	if l, ok := ctx.Value(loggerCtxKey).(*zap.Logger); ok {
		ctx = context.WithValue(ctx, loggerCtxKey, l.With(zap.String("request_id", requestID)))
	}
	return ctx
}

// WithTenantSlug menyuntikkan tenant slug ke context dan logger.
func WithTenantSlug(ctx context.Context, slug string) context.Context {
	ctx = context.WithValue(ctx, tenantCtxKey, slug)
	if l, ok := ctx.Value(loggerCtxKey).(*zap.Logger); ok {
		ctx = context.WithValue(ctx, loggerCtxKey, l.With(zap.String("tenant", slug)))
	}
	return ctx
}

// WithUserID menyuntikkan user ID ke context dan logger.
func WithUserID(ctx context.Context, userID string) context.Context {
	ctx = context.WithValue(ctx, userIDCtxKey, userID)
	if l, ok := ctx.Value(loggerCtxKey).(*zap.Logger); ok {
		ctx = context.WithValue(ctx, loggerCtxKey, l.With(zap.String("user_id", userID)))
	}
	return ctx
}

// RequestIDFromContext mengambil request ID dari context.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDCtxKey).(string)
	return id
}

// TenantSlugFromContext mengambil tenant slug dari context.
func TenantSlugFromContext(ctx context.Context) string {
	slug, _ := ctx.Value(tenantCtxKey).(string)
	return slug
}

// ── Named loggers ─────────────────────────────────────────────────

// Named mengembalikan logger dengan nama komponen.
// Dipakai oleh service dan repository untuk namespace log.
//
//	log := nightvision.Named(ctx, "accounting.invoice")
//	log.Info("Invoice dibuat", zap.String("id", id))
//	// output: {"component":"accounting.invoice","msg":"Invoice dibuat","id":"..."}
func Named(ctx context.Context, name string) *zap.Logger {
	return FromContext(ctx).Named(name)
}

// ── Log level helpers ─────────────────────────────────────────────

// ParseLevel mengkonversi string ke zapcore.Level.
func ParseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
