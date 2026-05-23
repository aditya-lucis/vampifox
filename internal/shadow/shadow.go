// Package shadow — Cache layer VampiFox menggunakan Redis.
//
// "Shadow" karena cache adalah bayangan dari data asli —
// selalu mengikuti, selalu lebih cepat, dan tidak pernah
// hidup lebih lama dari sumbernya.
//
// Shadow punya dua konsep utama:
//
//  1. Namespace — setiap tenant punya namespace sendiri di Redis.
//     Key "invoices:abc123" milik tenant "pt-maju" akan tersimpan
//     sebagai "vfx:pt-maju:invoices:abc123". Sehingga tidak ada
//     kebocoran data antar tenant.
//
//  2. Cascade — saat data berubah, semua cache yang bergantung
//     padanya otomatis di-invalidate. Lihat cascade.go.
//
// Alur penggunaan:
//
//	s, err := shadow.New(cfg, logger)   // buat Shadow (shared)
//	ts := s.ForTenant("pt-maju-jaya")   // scope ke tenant
//	ts.Haunt(ctx, "invoices:abc", data, 15*time.Minute)
//	ts.Recall(ctx, "invoices:abc", &dest)
//	ts.Vanish(ctx, "invoices:abc")
//	s.Close()
package shadow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aditya-lucis/vampifox/internal/den"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ═══════════════════════════════════════════════════════════════
//  Sentinel errors
// ═══════════════════════════════════════════════════════════════

// ErrNotFound dikembalikan oleh Recall() jika key tidak ada di cache.
// Caller sebaiknya memeriksa ini dengan errors.Is():
//
//	err := ts.Recall(ctx, key, &dest)
//	if errors.Is(err, shadow.ErrNotFound) {
//	    // cache miss — ambil dari database
//	}
var ErrNotFound = errors.New("shadow: key tidak ditemukan di cache")

// ErrNilValue dikembalikan jika value yang akan di-Haunt adalah nil.
var ErrNilValue = errors.New("shadow: tidak bisa menyimpan nil value")

// ═══════════════════════════════════════════════════════════════
//  Shadow — shared instance (satu per aplikasi)
// ═══════════════════════════════════════════════════════════════

// Shadow mengelola koneksi Redis dan membuat TenantShadow per-tenant.
// Gunakan New() untuk membuat instance — jangan buat langsung.
//
// Shadow adalah shared resource — satu instance untuk seluruh aplikasi.
// Isolasi tenant dilakukan di level TenantShadow via namespace prefix.
type Shadow struct {
	client     *redis.Client
	cfg        den.ShadowConfig
	logger     *zap.Logger
	globalPfx  string // prefix global: "vfx:"
}

// New membuat Shadow baru dan memverifikasi koneksi ke Redis.
func New(cfg den.ShadowConfig, logger *zap.Logger) (*Shadow, error) {
	if logger == nil {
		return nil, fmt.Errorf("[Shadow] logger tidak boleh nil")
	}

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
	})

	// Ping untuk memastikan Redis benar-benar bisa dijangkau
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("[Shadow] Redis tidak merespons di %s: %w", cfg.Addr, err)
	}

	s := &Shadow{
		client:    client,
		cfg:       cfg,
		logger:    logger.Named("shadow"),
		globalPfx: "vfx:",
	}

	logger.Info("👤 Shadow terhubung",
		zap.String("addr", cfg.Addr),
		zap.Int("db", cfg.DB),
	)

	return s, nil
}

// ═══════════════════════════════════════════════════════════════
//  ForTenant — scoping ke tenant
// ═══════════════════════════════════════════════════════════════

// ForTenant mengembalikan TenantShadow yang ter-scope ke tenant tertentu.
// Semua operasi cache yang dilakukan via TenantShadow ini akan otomatis
// menggunakan namespace "vfx:{tenantSlug}:".
//
// ForTenant tidak melakukan network call — aman dipanggil per-request.
//
//	ts := shadow.ForTenant("pt-maju-jaya")
//	ts.Haunt(ctx, "user:123", userData, 15*time.Minute)
//	// tersimpan di Redis sebagai "vfx:pt-maju-jaya:user:123"
func (s *Shadow) ForTenant(tenantSlug string) *TenantShadow {
	if tenantSlug == "" {
		s.logger.Warn("[Shadow] ForTenant dipanggil dengan slug kosong — pakai namespace global")
	}
	return &TenantShadow{
		shadow: s,
		ns:     s.globalPfx + tenantSlug + ":",
		slug:   tenantSlug,
	}
}

// ═══════════════════════════════════════════════════════════════
//  Health & Lifecycle
// ═══════════════════════════════════════════════════════════════

// Ping memeriksa koneksi Redis. Cocok untuk health check endpoint.
func (s *Shadow) Ping(ctx context.Context) error {
	if err := s.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("[Shadow] Redis tidak merespons: %w", err)
	}
	return nil
}

// Stats mengembalikan statistik pool koneksi Redis.
func (s *Shadow) Stats() *redis.PoolStats {
	return s.client.PoolStats()
}

// Close menutup koneksi Redis.
// Dipanggil saat Den.Slumber().
func (s *Shadow) Close() error {
	if err := s.client.Close(); err != nil {
		return fmt.Errorf("[Shadow] gagal menutup koneksi Redis: %w", err)
	}
	s.logger.Info("👤 Shadow ditutup — koneksi Redis dilepas")
	return nil
}

// ═══════════════════════════════════════════════════════════════
//  TenantShadow — operasi cache per-tenant
// ═══════════════════════════════════════════════════════════════

// TenantShadow adalah view Shadow yang sudah ter-scope ke satu tenant.
// Semua key otomatis di-prefix dengan namespace tenant.
//
// TenantShadow tidak menyimpan state — aman disimpan di request context
// atau dibuat baru per-handler.
type TenantShadow struct {
	shadow *Shadow
	ns     string // namespace: "vfx:{tenantSlug}:"
	slug   string // tenant slug untuk logging
}

// ── Key builder ───────────────────────────────────────────────────

// k membangun full Redis key dengan namespace tenant.
// "invoices:abc" → "vfx:pt-maju-jaya:invoices:abc"
func (ts *TenantShadow) k(key string) string {
	return ts.ns + key
}

// ── Haunt ────────────────────────────────────────────────────────

// Haunt menyimpan value ke cache dengan TTL.
//
// Value akan di-marshal ke JSON sebelum disimpan.
// TTL 0 berarti key tidak pernah expire (pakai dengan hati-hati).
//
//	ts.Haunt(ctx, "invoice:abc123", invoice, 15*time.Minute)
func (ts *TenantShadow) Haunt(ctx context.Context, key string, val any, ttl time.Duration) error {
	if val == nil {
		return ErrNilValue
	}

	b, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("[Shadow] gagal marshal key '%s': %w", key, err)
	}

	if err := ts.shadow.client.Set(ctx, ts.k(key), b, ttl).Err(); err != nil {
		return fmt.Errorf("[Shadow] gagal Haunt key '%s': %w", key, err)
	}

	ts.shadow.logger.Debug("Haunt",
		zap.String("tenant", ts.slug),
		zap.String("key", key),
		zap.Duration("ttl", ttl),
		zap.Int("bytes", len(b)),
	)

	return nil
}

// HauntNX menyimpan value ke cache HANYA jika key belum ada.
// Berguna untuk distributed lock atau idempotency key.
// Mengembalikan true jika berhasil disimpan, false jika key sudah ada.
func (ts *TenantShadow) HauntNX(ctx context.Context, key string, val any, ttl time.Duration) (bool, error) {
	if val == nil {
		return false, ErrNilValue
	}

	b, err := json.Marshal(val)
	if err != nil {
		return false, fmt.Errorf("[Shadow] gagal marshal key '%s': %w", key, err)
	}

	ok, err := ts.shadow.client.SetNX(ctx, ts.k(key), b, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("[Shadow] gagal HauntNX key '%s': %w", key, err)
	}

	return ok, nil
}

// ── Recall ────────────────────────────────────────────────────────

// Recall mengambil value dari cache dan meng-unmarshal ke dest.
//
// Mengembalikan shadow.ErrNotFound jika key tidak ada.
// dest harus berupa pointer, sama seperti json.Unmarshal.
//
//	var invoice Invoice
//	err := ts.Recall(ctx, "invoice:abc123", &invoice)
//	if errors.Is(err, shadow.ErrNotFound) {
//	    // cache miss
//	}
func (ts *TenantShadow) Recall(ctx context.Context, key string, dest any) error {
	b, err := ts.shadow.client.Get(ctx, ts.k(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrNotFound
		}
		return fmt.Errorf("[Shadow] gagal Recall key '%s': %w", key, err)
	}

	if err := json.Unmarshal(b, dest); err != nil {
		return fmt.Errorf("[Shadow] gagal unmarshal key '%s': %w", key, err)
	}

	ts.shadow.logger.Debug("Recall hit",
		zap.String("tenant", ts.slug),
		zap.String("key", key),
	)

	return nil
}

// RecallOrSet mengambil value dari cache. Jika tidak ada (cache miss),
// memanggil fn() untuk mengambil data asli, menyimpannya ke cache,
// lalu mengembalikannya.
//
// Ini adalah pola cache-aside yang paling umum digunakan.
//
//	invoice, err := ts.RecallOrSet(ctx, "invoice:abc", 15*time.Minute,
//	    func() (any, error) {
//	        return repo.FindInvoice(ctx, "abc")
//	    },
//	)
func (ts *TenantShadow) RecallOrSet(
	ctx context.Context,
	key string,
	ttl time.Duration,
	fn func() (any, error),
	dest any,
) error {
	// Coba dari cache dulu
	err := ts.Recall(ctx, key, dest)
	if err == nil {
		return nil // cache hit
	}
	if !errors.Is(err, ErrNotFound) {
		return err // error lain, bukan sekadar cache miss
	}

	// Cache miss — ambil dari sumber data
	val, err := fn()
	if err != nil {
		return err
	}

	// Simpan ke cache (best-effort — jangan fail kalau cache error)
	if haunErr := ts.Haunt(ctx, key, val, ttl); haunErr != nil {
		ts.shadow.logger.Warn("Gagal Haunt setelah RecallOrSet",
			zap.String("key", key),
			zap.Error(haunErr),
		)
	}

	// Marshal balik ke dest
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

// ── Vanish ────────────────────────────────────────────────────────

// Vanish menghapus satu key dari cache.
//
//	ts.Vanish(ctx, "invoice:abc123")
func (ts *TenantShadow) Vanish(ctx context.Context, key string) error {
	if err := ts.shadow.client.Del(ctx, ts.k(key)).Err(); err != nil {
		return fmt.Errorf("[Shadow] gagal Vanish key '%s': %w", key, err)
	}

	ts.shadow.logger.Debug("Vanish",
		zap.String("tenant", ts.slug),
		zap.String("key", key),
	)

	return nil
}

// VanishMany menghapus banyak key sekaligus dalam satu roundtrip (pipeline).
//
//	ts.VanishMany(ctx, "invoice:abc", "invoice:def", "invoice:ghi")
func (ts *TenantShadow) VanishMany(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	fullKeys := make([]string, len(keys))
	for i, k := range keys {
		fullKeys[i] = ts.k(k)
	}

	if err := ts.shadow.client.Del(ctx, fullKeys...).Err(); err != nil {
		return fmt.Errorf("[Shadow] gagal VanishMany (%d keys): %w", len(keys), err)
	}

	ts.shadow.logger.Debug("VanishMany",
		zap.String("tenant", ts.slug),
		zap.Int("count", len(keys)),
	)

	return nil
}

// Dispel menghapus semua key yang cocok dengan pattern di namespace tenant.
//
// ⚠️ PERINGATAN: Dispel menggunakan Redis SCAN, bukan KEYS.
// Aman untuk production karena tidak blocking, tapi bisa butuh
// beberapa roundtrip untuk dataset besar.
//
//	ts.Dispel(ctx, "invoices:*")      // hapus semua cache invoice tenant ini
//	ts.Dispel(ctx, "report:2024:*")   // hapus semua cache report 2024
func (ts *TenantShadow) Dispel(ctx context.Context, pattern string) (int64, error) {
	fullPattern := ts.k(pattern)

	var deleted int64
	var cursor uint64

	for {
		var keys []string
		var err error

		// Gunakan SCAN bukan KEYS — aman untuk production
		keys, cursor, err = ts.shadow.client.Scan(ctx, cursor, fullPattern, 100).Result()
		if err != nil {
			return deleted, fmt.Errorf("[Shadow] gagal SCAN pattern '%s': %w", pattern, err)
		}

		if len(keys) > 0 {
			n, err := ts.shadow.client.Del(ctx, keys...).Result()
			if err != nil {
				return deleted, fmt.Errorf("[Shadow] gagal DEL batch: %w", err)
			}
			deleted += n
		}

		if cursor == 0 {
			break // scan selesai
		}
	}

	if deleted > 0 {
		ts.shadow.logger.Debug("Dispel",
			zap.String("tenant", ts.slug),
			zap.String("pattern", pattern),
			zap.Int64("deleted", deleted),
		)
	}

	return deleted, nil
}

// ── Exists & TTL ──────────────────────────────────────────────────

// Exists memeriksa apakah key ada di cache.
func (ts *TenantShadow) Exists(ctx context.Context, key string) (bool, error) {
	n, err := ts.shadow.client.Exists(ctx, ts.k(key)).Result()
	if err != nil {
		return false, fmt.Errorf("[Shadow] gagal Exists key '%s': %w", key, err)
	}
	return n > 0, nil
}

// TTL mengembalikan sisa waktu hidup sebuah key.
// Mengembalikan -1 jika key ada tapi tidak punya TTL (persistent).
// Mengembalikan -2 jika key tidak ada.
func (ts *TenantShadow) TTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := ts.shadow.client.TTL(ctx, ts.k(key)).Result()
	if err != nil {
		return 0, fmt.Errorf("[Shadow] gagal TTL key '%s': %w", key, err)
	}
	return ttl, nil
}

// Refresh memperbarui TTL sebuah key tanpa mengubah nilainya.
// Berguna untuk "sliding expiration" pada session.
func (ts *TenantShadow) Refresh(ctx context.Context, key string, ttl time.Duration) error {
	ok, err := ts.shadow.client.Expire(ctx, ts.k(key), ttl).Result()
	if err != nil {
		return fmt.Errorf("[Shadow] gagal Refresh key '%s': %w", key, err)
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// ── Counter ───────────────────────────────────────────────────────

// Increment menaikkan nilai counter atomik.
// Berguna untuk rate limiting, view count, dll.
// Key akan dibuat dengan value 1 jika belum ada.
func (ts *TenantShadow) Increment(ctx context.Context, key string) (int64, error) {
	val, err := ts.shadow.client.Incr(ctx, ts.k(key)).Result()
	if err != nil {
		return 0, fmt.Errorf("[Shadow] gagal Increment key '%s': %w", key, err)
	}
	return val, nil
}

// IncrementBy menaikkan counter dengan nilai tertentu.
func (ts *TenantShadow) IncrementBy(ctx context.Context, key string, n int64) (int64, error) {
	val, err := ts.shadow.client.IncrBy(ctx, ts.k(key), n).Result()
	if err != nil {
		return 0, fmt.Errorf("[Shadow] gagal IncrementBy key '%s': %w", key, err)
	}
	return val, nil
}

// ── Namespace utilities ───────────────────────────────────────────

// FlushTenant menghapus SEMUA cache milik tenant ini.
// ⚠️ Gunakan dengan sangat hati-hati — hanya untuk keperluan debug
// atau saat tenant dihapus.
func (ts *TenantShadow) FlushTenant(ctx context.Context) (int64, error) {
	ts.shadow.logger.Warn("⚠️  FlushTenant dipanggil",
		zap.String("tenant", ts.slug),
	)
	return ts.Dispel(ctx, "*")
}

// Namespace mengembalikan namespace Redis yang dipakai tenant ini.
// Berguna untuk debugging.
func (ts *TenantShadow) Namespace() string {
	return ts.ns
}

// Slug mengembalikan tenant slug.
func (ts *TenantShadow) Slug() string {
	return ts.slug
}

// ── Pipeline ──────────────────────────────────────────────────────

// Pipeline mengembalikan Redis pipeline untuk batch operation.
// Berguna untuk VanishMany yang lebih kompleks atau operasi atomic.
//
// Caller bertanggung jawab memanggil Exec() pada pipeline.
//
//	pipe := ts.Pipeline()
//	pipe.Set(ctx, ts.RawKey("a"), "val1", ttl)
//	pipe.Set(ctx, ts.RawKey("b"), "val2", ttl)
//	pipe.Exec(ctx)
func (ts *TenantShadow) Pipeline() redis.Pipeliner {
	return ts.shadow.client.Pipeline()
}

// RawKey mengembalikan full key dengan namespace — untuk dipakai di Pipeline.
func (ts *TenantShadow) RawKey(key string) string {
	return ts.k(key)
}

// ── Helpers ───────────────────────────────────────────────────────

// BuildKey membangun key cache yang konsisten dari parts.
// Semua parts digabung dengan ":" sebagai separator.
//
//	ts.BuildKey("invoice", invoiceID)        → "invoice:abc123"
//	ts.BuildKey("report", "2024", "Q1")      → "report:2024:Q1"
func BuildKey(parts ...string) string {
	return strings.Join(parts, ":")
}