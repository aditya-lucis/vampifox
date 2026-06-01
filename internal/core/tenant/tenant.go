// Package tenant — Multi-tenancy engine VampiFox.
//
// Setiap "wilayah kekuasaan" adalah satu Tenant — bisa berupa
// perusahaan, sekolah, rumah sakit, atau organisasi apapun.
//
// Strategy isolasi: Schema-per-tenant di PostgreSQL/SQL Server,
// Database-per-tenant di MySQL, prefix-per-tenant di SQLite.
//
// Alur request:
//
//	HTTP Request
//	  → Resolver (baca subdomain / header X-VampiFox-Tenant)
//	  → Repository.FindBySlug() (cek Shadow cache dulu, fallback Fangs)
//	  → Validasi status (active? suspended? expired?)
//	  → WithTenant() → inject ke context
//	  → Handler bisa ambil via FromContext()
package tenant

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// ═══════════════════════════════════════════════════════════════
//  Sentinel errors
// ═══════════════════════════════════════════════════════════════

var (
	// ErrNotFound dikembalikan saat tenant dengan slug/domain tertentu tidak ada.
	ErrNotFound = errors.New("tenant tidak ditemukan")

	// ErrSuspended dikembalikan saat tenant sedang disuspend.
	// Request harus ditolak dengan 403.
	ErrSuspended = errors.New("tenant sedang disuspend")

	// ErrExpired dikembalikan saat masa berlaku tenant habis.
	ErrExpired = errors.New("masa berlaku tenant habis")

	// ErrSlugTaken dikembalikan saat slug sudah dipakai tenant lain.
	ErrSlugTaken = errors.New("slug sudah digunakan tenant lain")

	// ErrInvalidSlug dikembalikan saat slug tidak memenuhi format.
	ErrInvalidSlug = errors.New("slug hanya boleh berisi huruf kecil, angka, dan tanda hubung (-)")
)

// ═══════════════════════════════════════════════════════════════
//  Types
// ═══════════════════════════════════════════════════════════════

// Plan adalah paket langganan tenant.
type Plan string

const (
	PlanStarter    Plan = "starter"
	PlanGrowth     Plan = "growth"
	PlanEnterprise Plan = "enterprise"
)

// Status adalah kondisi operasional tenant saat ini.
type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusExpired   Status = "expired"
)

// Settings adalah konfigurasi bebas per-tenant yang disimpan sebagai JSON.
// Module bisa menyimpan konfigurasi spesifiknya di sini.
// Contoh: {"accounting": {"fiscal_year_start": "01-01"}, "timezone": "Asia/Jakarta"}
type Settings map[string]any

// ═══════════════════════════════════════════════════════════════
//  Tenant model
// ═══════════════════════════════════════════════════════════════

// Tenant merepresentasikan satu wilayah kekuasaan VampiFox.
// Disimpan di schema sistem (vfx_system.tenants), bukan di schema tenant itu sendiri.
type Tenant struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug       string    `gorm:"uniqueIndex;not null;size:100"`
	Name       string    `gorm:"not null;size:255"`
	Domain     string    `gorm:"uniqueIndex;size:255"` // custom domain opsional, e.g. "erp.pt-maju.com"
	Plan       Plan      `gorm:"not null;default:'starter'"`
	Status     Status    `gorm:"not null;default:'active'"`
	SchemaName string    `gorm:"not null;size:100"` // nama schema DB, e.g. "vfx_pt_maju_jaya"
	MaxUsers   int       `gorm:"not null;default:10"`
	StorageGB  int       `gorm:"not null;default:5"`
	Settings   Settings  `gorm:"serializer:json"`
	CreatedAt  time.Time
	UpdatedAt  time.Time

	// SuspendedAt diisi saat tenant disuspend — untuk audit trail.
	SuspendedAt *time.Time
	// ExpiresAt diisi untuk tenant berbasis langganan.
	// Nil berarti tidak ada tanggal kadaluarsa.
	ExpiresAt *time.Time
}

// TableName override agar GORM tahu tabel ini ada di schema sistem.
// Untuk PostgreSQL, prefix "vfx_system." ditangani via search_path.
func (Tenant) TableName() string { return "tenants" }

// ── Business logic methods ────────────────────────────────────────

// IsActive mengembalikan true jika tenant bisa menerima request.
func (t *Tenant) IsActive() bool {
	return t.Status == StatusActive && !t.IsExpired()
}

// IsExpired mengembalikan true jika tenant sudah melewati tanggal ExpiresAt.
func (t *Tenant) IsExpired() bool {
	if t.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*t.ExpiresAt)
}

// ValidateStatus memeriksa status tenant dan mengembalikan error
// yang sesuai jika tenant tidak bisa menerima request.
//
// Dipanggil oleh Resolver setelah berhasil load tenant.
func (t *Tenant) ValidateStatus() error {
	switch {
	case t.Status == StatusSuspended:
		return ErrSuspended
	case t.IsExpired():
		return ErrExpired
	case t.Status == StatusExpired:
		return ErrExpired
	}
	return nil
}

// Setting mengambil nilai setting tertentu dengan type assertion.
// Mengembalikan nil jika key tidak ada.
//
//	tz, _ := tenant.Setting("timezone").(string)
func (t *Tenant) Setting(key string) any {
	if t.Settings == nil {
		return nil
	}
	return t.Settings[key]
}

// SetSetting menetapkan nilai setting tertentu.
func (t *Tenant) SetSetting(key string, val any) {
	if t.Settings == nil {
		t.Settings = make(Settings)
	}
	t.Settings[key] = val
}

// ─ Implements fangs.TenantScope — via Scope adapter di resolver.go ──
//
// *Tenant tidak langsung implement fangs.TenantScope karena field Slug
// konflik dengan nama method. Gunakan tenant.NewScope(t) atau
// tenant.ScopeFromContext(ctx) untuk mendapatkan fangs.TenantScope.

// ═══════════════════════════════════════════════════════════════
//  CreateTenantInput — input untuk membuat tenant baru
// ═══════════════════════════════════════════════════════════════

// CreateInput adalah data yang dibutuhkan untuk membuat tenant baru.
type CreateInput struct {
	Name   string `json:"name"   validate:"required,min=2,max=255"`
	Slug   string `json:"slug"   validate:"required,min=2,max=100"`
	Plan   Plan   `json:"plan"`
	Domain string `json:"domain"`
	// MaxUsers dan StorageGB di-set otomatis berdasarkan Plan jika tidak diisi
	MaxUsers  int `json:"max_users"`
	StorageGB int `json:"storage_gb"`
}

// Validate memvalidasi input sebelum membuat tenant.
func (i *CreateInput) Validate() error {
	if strings.TrimSpace(i.Name) == "" {
		return errors.New("nama tenant wajib diisi")
	}
	if err := ValidateSlug(i.Slug); err != nil {
		return err
	}
	if i.Plan == "" {
		i.Plan = PlanStarter
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════
//  Context helpers
// ═══════════════════════════════════════════════════════════════

// contextKey adalah private type untuk context key — mencegah collision
// dengan package lain yang juga menyimpan nilai di context.
type contextKey string

const tenantCtxKey contextKey = "vfx_tenant"

// WithTenant menyuntikkan *Tenant ke dalam context.
// Dipanggil oleh Resolver middleware setelah tenant berhasil diload.
func WithTenant(ctx context.Context, t *Tenant) context.Context {
	return context.WithValue(ctx, tenantCtxKey, t)
}

// FromContext mengambil *Tenant dari context.
// Mengembalikan (nil, false) jika tenant tidak ada di context —
// biasanya berarti request belum melewati TenantResolver middleware.
func FromContext(ctx context.Context) (*Tenant, bool) {
	t, ok := ctx.Value(tenantCtxKey).(*Tenant)
	return t, ok && t != nil
}

// MustFromContext seperti FromContext tapi panic jika tenant tidak ada.
// Pakai ini hanya di handler yang dijamin sudah melewati TenantResolver.
func MustFromContext(ctx context.Context) *Tenant {
	t, ok := FromContext(ctx)
	if !ok {
		panic("[tenant] MustFromContext dipanggil tanpa TenantResolver middleware")
	}
	return t
}

// ═══════════════════════════════════════════════════════════════
//  Slug utilities
// ═══════════════════════════════════════════════════════════════

// slugPattern mendefinisikan format slug yang valid.
// Hanya huruf kecil, angka, dan tanda hubung. Tidak boleh mulai/akhir dengan -.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

// ValidateSlug memvalidasi format slug.
// Slug valid: "pt-maju-jaya", "rs-sehat-123", "sekolah-abc"
// Slug invalid: "PT Maju", "-invalid", "too-", "ab" (min 3 char)
func ValidateSlug(slug string) error {
	if len(slug) < 3 {
		return errors.New("slug minimal 3 karakter")
	}
	if len(slug) > 100 {
		return errors.New("slug maksimal 100 karakter")
	}
	if !slugPattern.MatchString(slug) {
		return ErrInvalidSlug
	}
	// Tidak boleh menggunakan reserved words
	for _, reserved := range reservedSlugs {
		if slug == reserved {
			return errors.New("slug '" + slug + "' adalah reserved word dan tidak bisa digunakan")
		}
	}
	return nil
}

// NormalizeSlug mengubah string bebas menjadi slug yang valid.
// "PT Maju Jaya Tbk." → "pt-maju-jaya-tbk"
func NormalizeSlug(s string) string {
	// Lowercase
	s = strings.ToLower(s)

	// Ganti karakter non-alphanumeric dengan dash
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteRune('-')
			prevDash = true
		}
	}

	result := strings.Trim(b.String(), "-")
	if len(result) > 100 {
		result = result[:100]
		result = strings.TrimRight(result, "-")
	}
	return result
}

// SchemaNameFor menghasilkan nama schema database dari slug.
// "pt-maju-jaya" → "vfx_pt_maju_jaya"
func SchemaNameFor(slug string) string {
	return "vfx_" + strings.ReplaceAll(slug, "-", "_")
}

// reservedSlugs adalah daftar slug yang tidak boleh dipakai tenant.
var reservedSlugs = []string{
	"www", "api", "admin", "app", "mail", "smtp", "ftp",
	"dashboard", "console", "system", "root", "public",
	"vampifox", "vfx", "static", "assets", "cdn",
	"health", "metrics", "status", "docs",
}
