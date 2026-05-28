// cascade.go — Shadow Cascade: auto-invalidate cache berdasarkan dependency graph.
//
// Masalah yang dipecahkan:
// Saat invoice diupdate, cache apa saja yang harus di-invalidate?
//   - Cache invoice itu sendiri                  → "invoice:abc123"
//   - Cache list invoice customer                → "customer:xyz:invoices"
//   - Cache summary AR bulan ini                 → "report:ar:2024:11"
//   - Cache dashboard yang menampilkan total AR  → "dashboard:finance"
//
// Tanpa Cascade, developer harus ingat dan manually invalidate semua ini.
// Dengan Cascade, cukup daftarkan dependency-nya sekali — sisanya otomatis.
//
// Cara kerja:
//
//  1. Developer mendaftarkan dependency via RegisterDeps():
//
//     cascade.RegisterDeps("invoice", []string{
//         "customer:*:invoices",
//         "report:ar:*",
//         "dashboard:finance",
//     })
//
//  2. Saat invoice diupdate, panggil Invalidate():
//
//     cascade.Invalidate(ctx, ts, "invoice", "abc123")
//
//  3. Cascade otomatis menghapus semua key yang terdaftar sebagai dependent.
//
// Format key dependency:
//   - "exact:key"      → hapus key persis ini
//   - "prefix:*"       → hapus semua key yang diawali "prefix:"
//   - "{id}"           → diganti dengan entityID yang di-invalidate
package shadow

import (
	"errors"
	"context"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// ═══════════════════════════════════════════════════════════════
//  Cascade
// ═══════════════════════════════════════════════════════════════

// Cascade mengelola dependency graph antar cache key.
// Satu instance Cascade dipakai untuk seluruh aplikasi (shared).
type Cascade struct {
	mu     sync.RWMutex
	deps   map[string][]string // entityType → []dependent key patterns
	logger *zap.Logger
}

// NewCascade membuat Cascade baru.
func NewCascade(logger *zap.Logger) *Cascade {
	return &Cascade{
		deps:   make(map[string][]string),
		logger: logger.Named("cascade"),
	}
}

// ── Registration ──────────────────────────────────────────────────

// RegisterDeps mendaftarkan dependent cache keys untuk sebuah entity type.
//
// entityType adalah identifier tipe data, e.g. "invoice", "customer", "product".
// patterns adalah daftar key pattern yang harus di-invalidate saat entity berubah.
//
// Pattern bisa berupa:
//   - Key eksak: "dashboard:finance"
//   - Wildcard suffix: "report:ar:*"   → semua key yang diawali "report:ar:"
//   - Placeholder ID: "customer:{id}:invoices" → {id} diganti entityID
//
// Contoh penggunaan di module Accounting saat startup:
//
//	cascade.RegisterDeps("invoice", []string{
//	    "invoice:{id}",             // cache invoice itu sendiri
//	    "customer:{id}:invoices",   // list invoice per customer
//	    "report:ar:*",              // semua cache laporan AR
//	    "dashboard:finance",        // dashboard keuangan
//	})
//
// RegisterDeps aman dipanggil dari beberapa goroutine (thread-safe).
// Jika entityType sudah ada, patterns baru ditambahkan (tidak replace).
func (c *Cascade) RegisterDeps(entityType string, patterns []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deps[entityType] = append(c.deps[entityType], patterns...)

	c.logger.Debug("Dependency terdaftar",
		zap.String("entity", entityType),
		zap.Int("patterns", len(patterns)),
		zap.Strings("patterns", patterns),
	)
}

// Deps mengembalikan semua pattern dependency untuk entityType tertentu.
// Mengembalikan nil jika entityType belum terdaftar.
func (c *Cascade) Deps(entityType string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deps[entityType]
}

// ── Invalidation ──────────────────────────────────────────────────

// Invalidate menghapus semua cache yang bergantung pada entity yang berubah.
//
// entityType: tipe entity yang berubah, e.g. "invoice"
// entityID:   ID entity yang berubah, e.g. "abc123"
// ts:         TenantShadow yang akan di-invalidate (scope ke tenant)
//
// Contoh saat invoice diupdate:
//
//	n, err := cascade.Invalidate(ctx, ts, "invoice", "abc123")
//	// → menghapus "invoice:abc123", "customer:abc123:invoices",
//	//   semua "report:ar:*", dan "dashboard:finance"
//
// Mengembalikan total jumlah key yang berhasil dihapus.
func (c *Cascade) Invalidate(ctx context.Context, ts *TenantShadow, entityType, entityID string) (int64, error) {
	c.mu.RLock()
	patterns, ok := c.deps[entityType]
	c.mu.RUnlock()

	if !ok || len(patterns) == 0 {
		c.logger.Debug("Tidak ada dependency terdaftar untuk entity",
			zap.String("entity", entityType),
			zap.String("id", entityID),
		)
		return 0, nil
	}

	var totalDeleted int64

	for _, pattern := range patterns {
		// Resolve placeholder {id}
		resolved := strings.ReplaceAll(pattern, "{id}", entityID)

		var deleted int64
		var err error

		if strings.HasSuffix(resolved, "*") {
			// Pattern wildcard — gunakan Dispel (SCAN-based)
			deleted, err = ts.Dispel(ctx, resolved)
		} else {
			// Key eksak — langsung Delete
			err = ts.Vanish(ctx, resolved)
			if err == nil {
				deleted = 1
			} else if errors.Is(err, ErrNotFound) {
				// Key tidak ada di cache — bukan error, lanjut saja
				err = nil
				deleted = 0
			}
		}

		if err != nil {
			c.logger.Warn("Cascade invalidate gagal untuk pattern",
				zap.String("entity", entityType),
				zap.String("id", entityID),
				zap.String("pattern", resolved),
				zap.Error(err),
			)
			// Lanjut ke pattern berikutnya — partial invalidation lebih baik dari none
			continue
		}

		totalDeleted += deleted
	}

	c.logger.Debug("Cascade invalidate selesai",
		zap.String("entity", entityType),
		zap.String("id", entityID),
		zap.String("tenant", ts.Slug()),
		zap.Int64("deleted", totalDeleted),
		zap.Int("patterns", len(patterns)),
	)

	return totalDeleted, nil
}

// InvalidateMany menginvalidate banyak entity sekaligus.
// Berguna untuk bulk update — e.g. saat 50 invoice diupdate sekaligus.
//
//	cascade.InvalidateMany(ctx, ts, "invoice", []string{"abc", "def", "ghi"})
func (c *Cascade) InvalidateMany(ctx context.Context, ts *TenantShadow, entityType string, entityIDs []string) (int64, error) {
	var total int64
	for _, id := range entityIDs {
		n, err := c.Invalidate(ctx, ts, entityType, id)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// InvalidateType menghapus SEMUA cache untuk satu entity type di tenant,
// tanpa peduli entityID tertentu.
// Berguna saat ada perubahan yang mempengaruhi semua record satu tipe.
//
//	cascade.InvalidateType(ctx, ts, "product")
//	// → hapus semua "product:*" di cache tenant
func (c *Cascade) InvalidateType(ctx context.Context, ts *TenantShadow, entityType string) (int64, error) {
	c.logger.Info("InvalidateType",
		zap.String("entity", entityType),
		zap.String("tenant", ts.Slug()),
	)
	return ts.Dispel(ctx, entityType+":*")
}

// ── Graph inspection ──────────────────────────────────────────────

// AllEntityTypes mengembalikan semua entity type yang terdaftar.
// Berguna untuk debugging dan monitoring.
func (c *Cascade) AllEntityTypes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	types := make([]string, 0, len(c.deps))
	for t := range c.deps {
		types = append(types, t)
	}
	return types
}

// TotalDeps mengembalikan total jumlah pattern dependency yang terdaftar.
func (c *Cascade) TotalDeps() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := 0
	for _, patterns := range c.deps {
		total += len(patterns)
	}
	return total
}
