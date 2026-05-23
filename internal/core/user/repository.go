package user

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/aditya-lucis/vampifox/internal/shadow"
)

// ═══════════════════════════════════════════════════════════════
//  Repository
// ═══════════════════════════════════════════════════════════════

// Repository mengelola persistensi User ke database tenant.
//
// PENTING: db yang diberikan ke Repository HARUS sudah di-scope
// ke schema tenant yang benar via fangs.For(scope).
// Repository tidak peduli dengan routing tenant — itu tanggung
// jawab handler dan middleware.
type Repository struct {
	db     *gorm.DB
	shadow *shadow.TenantShadow
	logger *zap.Logger
}

// NewRepository membuat Repository baru.
//
// db    — *gorm.DB yang sudah di-scope ke schema tenant
// ts    — TenantShadow untuk tenant yang sama
// logger — zap logger
func NewRepository(db *gorm.DB, ts *shadow.TenantShadow, logger *zap.Logger) *Repository {
	return &Repository{
		db:     db,
		shadow: ts,
		logger: logger.Named("user.repo"),
	}
}

// ── Cache keys ────────────────────────────────────────────────────

const userCacheTTL = 10 * time.Minute

func cacheKeyByID(id string) string       { return shadow.BuildKey("user", "id", id) }
func cacheKeyByEmail(email string) string { return shadow.BuildKey("user", "email", email) }

// ── Finders ───────────────────────────────────────────────────────

// FindByID mencari user berdasarkan UUID.
func (r *Repository) FindByID(ctx context.Context, id string) (*User, error) {
	var u User
	// Coba cache
	if err := r.shadow.Recall(ctx, cacheKeyByID(id), &u); err == nil {
		return &u, nil
	}

	// Fallback DB
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&u)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}

	_ = r.shadow.Haunt(ctx, cacheKeyByID(id), u, userCacheTTL)
	return &u, nil
}

// FindByEmail mencari user berdasarkan email (case-insensitive).
func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	// Coba cache
	if err := r.shadow.Recall(ctx, cacheKeyByEmail(email), &u); err == nil {
		return &u, nil
	}

	result := r.db.WithContext(ctx).
		Where("LOWER(email) = LOWER(?)", email).
		First(&u)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}

	_ = r.shadow.Haunt(ctx, cacheKeyByEmail(email), u, userCacheTTL)
	return &u, nil
}

// EmailExists memeriksa apakah email sudah terdaftar di tenant ini.
func (r *Repository) EmailExists(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&User{}).
		Where("LOWER(email) = LOWER(?)", email).
		Count(&count).Error
	return count > 0, err
}

// ── Writers ───────────────────────────────────────────────────────

// Create menyimpan user baru ke database.
func (r *Repository) Create(ctx context.Context, u *User) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return err
	}
	r.populateCache(ctx, u)
	r.logger.Info("User dibuat",
		zap.String("id", u.ID.String()),
		zap.String("email", u.Email),
	)
	return nil
}

// Update menyimpan perubahan user dan invalidate cache.
func (r *Repository) Update(ctx context.Context, u *User) error {
	u.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(u).Error; err != nil {
		return err
	}
	r.invalidateCache(ctx, u)
	r.populateCache(ctx, u)
	return nil
}

// UpdateLastLogin mencatat waktu login terakhir.
// Dipisah dari Update supaya tidak trigger full save saat login.
func (r *Repository) UpdateLastLogin(ctx context.Context, u *User) error {
	now := time.Now()
	if err := r.db.WithContext(ctx).
		Model(u).
		Update("last_login_at", now).Error; err != nil {
		return err
	}
	u.LastLoginAt = &now
	r.invalidateCache(ctx, u)
	return nil
}

// UpdateStatus mengubah status user.
func (r *Repository) UpdateStatus(ctx context.Context, u *User, status Status) error {
	if err := r.db.WithContext(ctx).
		Model(u).
		Updates(map[string]any{
			"status":     status,
			"updated_at": time.Now(),
		}).Error; err != nil {
		return err
	}
	u.Status = status
	r.invalidateCache(ctx, u)
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────

func (r *Repository) populateCache(ctx context.Context, u *User) {
	_ = r.shadow.Haunt(ctx, cacheKeyByID(u.ID.String()), u, userCacheTTL)
	_ = r.shadow.Haunt(ctx, cacheKeyByEmail(u.Email), u, userCacheTTL)
}

func (r *Repository) invalidateCache(ctx context.Context, u *User) {
	_ = r.shadow.Vanish(ctx, cacheKeyByID(u.ID.String()))
	_ = r.shadow.Vanish(ctx, cacheKeyByEmail(u.Email))
}
