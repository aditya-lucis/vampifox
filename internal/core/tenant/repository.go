package tenant

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/aditya-lucis/vampifox/internal/shadow"
)

// ═══════════════════════════════════════════════════════════════
//  Repository — data access layer untuk Tenant
// ═══════════════════════════════════════════════════════════════

// Repository mengelola persistensi Tenant ke database sistem.
//
// Semua query tenant menggunakan database sistem (bukan schema tenant),
// karena data tenant disimpan di tabel global vfx_system.tenants.
//
// Caching strategy: Cache-aside via Shadow.
//   - Read: cek Shadow dulu → fallback Fangs → update Shadow
//   - Write: update Fangs → invalidate Shadow
type Repository struct {
	db     *gorm.DB
	shadow *shadow.Shadow
	logger *zap.Logger
}

// NewRepository membuat Repository baru.
// db harus berupa koneksi ke schema sistem (bukan tenant schema).
func NewRepository(db *gorm.DB, sh *shadow.Shadow, logger *zap.Logger) *Repository {
	return &Repository{
		db:     db,
		shadow: sh,
		logger: logger.Named("tenant.repo"),
	}
}

// ── Cache key helpers ─────────────────────────────────────────────

// systemShadow mengembalikan TenantShadow dengan namespace khusus
// untuk data sistem (bukan per-tenant).
func (r *Repository) systemShadow() *shadow.TenantShadow {
	return r.shadow.ForTenant("_system")
}

func cacheKeyBySlug(slug string) string   { return shadow.BuildKey("tenant", "slug", slug) }
func cacheKeyByDomain(domain string) string { return shadow.BuildKey("tenant", "domain", domain) }

const tenantCacheTTL = 5 * time.Minute

// ── Finders ───────────────────────────────────────────────────────

// FindBySlug mencari tenant berdasarkan slug.
// Mencoba Shadow cache terlebih dahulu, fallback ke Fangs jika miss.
func (r *Repository) FindBySlug(ctx context.Context, slug string) (*Tenant, error) {
	ts := r.systemShadow()
	cacheKey := cacheKeyBySlug(slug)

	// Coba dari cache
	var t Tenant
	err := ts.Recall(ctx, cacheKey, &t)
	if err == nil {
		return &t, nil
	}
	if !errors.Is(err, shadow.ErrNotFound) {
		// Error Redis yang tidak terduga — log tapi lanjut ke DB
		r.logger.Warn("Shadow error saat FindBySlug, fallback ke DB",
			zap.String("slug", slug),
			zap.Error(err),
		)
	}

	// Cache miss — ambil dari database
	var tenant Tenant
	result := r.db.WithContext(ctx).
		Where("slug = ?", slug).
		First(&tenant)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}

	// Simpan ke cache (best-effort)
	_ = ts.Haunt(ctx, cacheKey, tenant, tenantCacheTTL)

	return &tenant, nil
}

// FindByDomain mencari tenant berdasarkan custom domain.
func (r *Repository) FindByDomain(ctx context.Context, domain string) (*Tenant, error) {
	ts := r.systemShadow()
	cacheKey := cacheKeyByDomain(domain)

	var t Tenant
	if err := ts.Recall(ctx, cacheKey, &t); err == nil {
		return &t, nil
	}

	var tenant Tenant
	result := r.db.WithContext(ctx).
		Where("domain = ?", domain).
		First(&tenant)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}

	_ = ts.Haunt(ctx, cacheKey, tenant, tenantCacheTTL)
	return &tenant, nil
}

// SlugExists memeriksa apakah slug sudah digunakan.
func (r *Repository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Tenant{}).
		Where("slug = ?", slug).
		Count(&count).Error
	return count > 0, err
}

// DomainExists memeriksa apakah domain sudah digunakan.
func (r *Repository) DomainExists(ctx context.Context, domain string) (bool, error) {
	if domain == "" {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Tenant{}).
		Where("domain = ?", domain).
		Count(&count).Error
	return count > 0, err
}

// ── Writers ───────────────────────────────────────────────────────

// Create menyimpan tenant baru ke database.
func (r *Repository) Create(ctx context.Context, t *Tenant) error {
	result := r.db.WithContext(ctx).Create(t)
	if result.Error != nil {
		return result.Error
	}

	// Cache tenant yang baru dibuat
	ts := r.systemShadow()
	_ = ts.Haunt(ctx, cacheKeyBySlug(t.Slug), t, tenantCacheTTL)
	if t.Domain != "" {
		_ = ts.Haunt(ctx, cacheKeyByDomain(t.Domain), t, tenantCacheTTL)
	}

	r.logger.Info("Tenant dibuat",
		zap.String("slug", t.Slug),
		zap.String("plan", string(t.Plan)),
	)
	return nil
}

// Update menyimpan perubahan tenant ke database dan invalidate cache.
func (r *Repository) Update(ctx context.Context, t *Tenant) error {
	result := r.db.WithContext(ctx).Save(t)
	if result.Error != nil {
		return result.Error
	}

	// Invalidate cache — akan di-repopulate saat FindBySlug berikutnya
	r.invalidateCache(ctx, t)
	return nil
}

// UpdateStatus mengubah status tenant dan mencatat timestamp.
func (r *Repository) UpdateStatus(ctx context.Context, t *Tenant, status Status) error {
	now := time.Now()
	updates := map[string]any{
		"status":     status,
		"updated_at": now,
	}

	if status == StatusSuspended {
		updates["suspended_at"] = now
	}

	result := r.db.WithContext(ctx).
		Model(t).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}

	t.Status = status
	t.UpdatedAt = now
	if status == StatusSuspended {
		t.SuspendedAt = &now
	}

	r.invalidateCache(ctx, t)

	r.logger.Info("Status tenant diubah",
		zap.String("slug", t.Slug),
		zap.String("status", string(status)),
	)
	return nil
}

// UpdateSettings menyimpan settings tenant.
func (r *Repository) UpdateSettings(ctx context.Context, t *Tenant) error {
	result := r.db.WithContext(ctx).
		Model(t).
		Update("settings", t.Settings)
	if result.Error != nil {
		return result.Error
	}
	r.invalidateCache(ctx, t)
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────

// invalidateCache menghapus semua cache entry untuk tenant ini.
func (r *Repository) invalidateCache(ctx context.Context, t *Tenant) {
	ts := r.systemShadow()
	_ = ts.Vanish(ctx, cacheKeyBySlug(t.Slug))
	if t.Domain != "" {
		_ = ts.Vanish(ctx, cacheKeyByDomain(t.Domain))
	}
}
