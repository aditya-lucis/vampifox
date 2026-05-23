// Package tenant — Multi-tenancy engine VampiFox.
// Setiap "wilayah kekuasaan" vampire adalah satu tenant.
// Strategy: Schema-per-tenant di PostgreSQL untuk isolasi data penuh.
package tenant

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ContextKey untuk menyimpan tenant di context.
type contextKey string

const TenantContextKey contextKey = "vampifox_tenant"

// Tenant merepresentasikan satu wilayah kekuasaan (perusahaan/organisasi).
type Tenant struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug        string    `gorm:"uniqueIndex;not null"` // e.g. "pt-maju-jaya"
	Name        string    `gorm:"not null"`
	Domain      string    `gorm:"uniqueIndex"` // custom domain opsional
	Plan        Plan      `gorm:"not null;default:'starter'"`
	Status      Status    `gorm:"not null;default:'active'"`
	SchemaName  string    `gorm:"not null"` // PostgreSQL schema name
	MaxUsers    int       `gorm:"default:10"`
	StorageGB   int       `gorm:"default:5"`
	Settings    JSONB     `gorm:"type:jsonb"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	SuspendedAt *time.Time
	ExpiresAt   *time.Time
}

type Plan string
type Status string
type JSONB map[string]any

const (
	PlanStarter    Plan = "starter"
	PlanGrowth     Plan = "growth"
	PlanEnterprise Plan = "enterprise"

	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusExpired   Status = "expired"
)

var (
	ErrTenantNotFound  = errors.New("tenant tidak ditemukan")
	ErrTenantSuspended = errors.New("tenant sedang disuspend")
	ErrTenantExpired   = errors.New("masa berlaku tenant habis")
)

// FromContext mengambil tenant dari context request.
func FromContext(ctx context.Context) (*Tenant, bool) {
	t, ok := ctx.Value(TenantContextKey).(*Tenant)
	return t, ok
}

// WithTenant menyuntikkan tenant ke dalam context.
func WithTenant(ctx context.Context, t *Tenant) context.Context {
	return context.WithValue(ctx, TenantContextKey, t)
}

// SchemaFor menghasilkan nama schema PostgreSQL dari tenant slug.
// e.g. "pt-maju-jaya" → "vfx_pt_maju_jaya"
func SchemaFor(slug string) string {
	result := "vfx_"

	for _, c := range slug {
		if c == '-' {
			result += "_"
		} else {
			result += string(c)
		}
	}

	return result
}
