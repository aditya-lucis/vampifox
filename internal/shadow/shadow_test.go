package shadow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/config"
)

// ── Test helpers ──────────────────────────────────────────────────

// newTestShadow membuat Shadow yang terhubung ke miniredis (in-process).
// Tidak butuh Redis server — aman dijalankan di CI/CD.
func newTestShadow(t *testing.T) (*Shadow, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("gagal start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	cfg := config.ShadowConfig{
		Addr:         mr.Addr(),
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     5,
		DefaultTTL:   15 * time.Minute,
	}

	s, err := New(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	return s, mr
}

func ctx() context.Context { return context.Background() }

// ── Test: New ────────────────────────────────────────────────────

func TestNew_Success(t *testing.T) {
	s, _ := newTestShadow(t)
	if s == nil {
		t.Fatal("New() tidak boleh nil")
	}
}

func TestNew_NilLogger(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	cfg := config.ShadowConfig{Addr: mr.Addr()}
	_, err = New(cfg, nil)
	if err == nil {
		t.Error("New() dengan nil logger harus error")
	}
}

func TestNew_InvalidAddr(t *testing.T) {
	cfg := config.ShadowConfig{
		Addr:        "localhost:1", // port yang pasti tidak ada Redis
		DialTimeout: 1 * time.Second,
	}
	_, err := New(cfg, zap.NewNop())
	if err == nil {
		t.Error("New() dengan alamat invalid harus error")
	}
}

// ── Test: ForTenant ───────────────────────────────────────────────

func TestForTenant_Namespace(t *testing.T) {
	s, _ := newTestShadow(t)

	ts := s.ForTenant("pt-maju-jaya")
	if ts == nil {
		t.Fatal("ForTenant() tidak boleh nil")
	}
	if ts.Namespace() != "vfx:pt-maju-jaya:" {
		t.Errorf("Namespace() = %q, want %q", ts.Namespace(), "vfx:pt-maju-jaya:")
	}
	if ts.Slug() != "pt-maju-jaya" {
		t.Errorf("Slug() = %q, want pt-maju-jaya", ts.Slug())
	}
}

func TestForTenant_IsolationBetweenTenants(t *testing.T) {
	s, _ := newTestShadow(t)

	ts1 := s.ForTenant("tenant-a")
	ts2 := s.ForTenant("tenant-b")

	type data struct{ Val string }
	ttl := time.Minute

	// Simpan di tenant A
	_ = ts1.Haunt(ctx(), "user:123", data{"alice"}, ttl)

	// Tenant B tidak boleh bisa baca
	var dest data
	err := ts2.Recall(ctx(), "user:123", &dest)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("tenant-b seharusnya tidak bisa baca data tenant-a, tapi got: %v", err)
	}
}

// ── Test: Haunt & Recall ──────────────────────────────────────────

func TestHaunt_Recall_BasicTypes(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	type Invoice struct {
		ID     string
		Amount float64
		Paid   bool
	}

	original := Invoice{ID: "inv-001", Amount: 150000.50, Paid: false}
	if err := ts.Haunt(ctx(), "invoice:inv-001", original, time.Minute); err != nil {
		t.Fatalf("Haunt() gagal: %v", err)
	}

	var retrieved Invoice
	if err := ts.Recall(ctx(), "invoice:inv-001", &retrieved); err != nil {
		t.Fatalf("Recall() gagal: %v", err)
	}

	if retrieved.ID != original.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, original.ID)
	}
	if retrieved.Amount != original.Amount {
		t.Errorf("Amount = %f, want %f", retrieved.Amount, original.Amount)
	}
	if retrieved.Paid != original.Paid {
		t.Errorf("Paid = %v, want %v", retrieved.Paid, original.Paid)
	}
}

func TestRecall_NotFound(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	var dest struct{ Val string }
	err := ts.Recall(ctx(), "tidak:ada", &dest)

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Recall() key tidak ada harus return ErrNotFound, got: %v", err)
	}
}

func TestHaunt_NilValue(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	err := ts.Haunt(ctx(), "key", nil, time.Minute)
	if !errors.Is(err, ErrNilValue) {
		t.Errorf("Haunt(nil) harus return ErrNilValue, got: %v", err)
	}
}

func TestHaunt_TTL_Expires(t *testing.T) {
	s, mr := newTestShadow(t)
	ts := s.ForTenant("test")

	_ = ts.Haunt(ctx(), "temp:key", map[string]int{"v": 1}, 1*time.Second)

	// Maju waktu miniredis 2 detik
	mr.FastForward(2 * time.Second)

	var dest map[string]int
	err := ts.Recall(ctx(), "temp:key", &dest)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("key yang sudah expired harus ErrNotFound, got: %v", err)
	}
}

// ── Test: HauntNX ────────────────────────────────────────────────

func TestHauntNX(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	// Pertama kali — harus berhasil
	ok, err := ts.HauntNX(ctx(), "lock:job-123", "locked", time.Minute)
	if err != nil {
		t.Fatalf("HauntNX() gagal: %v", err)
	}
	if !ok {
		t.Error("HauntNX() pertama harus return true")
	}

	// Kedua kali — key sudah ada, harus return false
	ok, err = ts.HauntNX(ctx(), "lock:job-123", "locked-again", time.Minute)
	if err != nil {
		t.Fatalf("HauntNX() kedua gagal: %v", err)
	}
	if ok {
		t.Error("HauntNX() kedua harus return false (key sudah ada)")
	}
}

// ── Test: RecallOrSet ─────────────────────────────────────────────

func TestRecallOrSet_CacheMiss(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	called := 0
	type Data struct{ V int }

	var dest Data
	err := ts.RecallOrSet(ctx(), "item:123", time.Minute, func() (any, error) {
		called++
		return Data{V: 42}, nil
	}, &dest)

	if err != nil {
		t.Fatalf("RecallOrSet() gagal: %v", err)
	}
	if called != 1 {
		t.Errorf("fn dipanggil %d kali, want 1", called)
	}
	if dest.V != 42 {
		t.Errorf("dest.V = %d, want 42", dest.V)
	}
}

func TestRecallOrSet_CacheHit(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	type Data struct{ V int }

	// Pre-populate cache
	_ = ts.Haunt(ctx(), "item:999", Data{V: 99}, time.Minute)

	called := 0
	var dest Data
	err := ts.RecallOrSet(ctx(), "item:999", time.Minute, func() (any, error) {
		called++
		return Data{V: 0}, nil // tidak seharusnya dipanggil
	}, &dest)

	if err != nil {
		t.Fatalf("RecallOrSet() gagal: %v", err)
	}
	if called != 0 {
		t.Error("fn tidak seharusnya dipanggil saat cache hit")
	}
	if dest.V != 99 {
		t.Errorf("dest.V = %d, want 99", dest.V)
	}
}

// ── Test: Vanish ─────────────────────────────────────────────────

func TestVanish(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	_ = ts.Haunt(ctx(), "del:me", "value", time.Minute)

	if err := ts.Vanish(ctx(), "del:me"); err != nil {
		t.Fatalf("Vanish() gagal: %v", err)
	}

	var dest string
	if !errors.Is(ts.Recall(ctx(), "del:me", &dest), ErrNotFound) {
		t.Error("key harus sudah hilang setelah Vanish()")
	}
}

func TestVanishMany(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	keys := []string{"k1", "k2", "k3"}
	for _, k := range keys {
		_ = ts.Haunt(ctx(), k, k, time.Minute)
	}

	if err := ts.VanishMany(ctx(), keys...); err != nil {
		t.Fatalf("VanishMany() gagal: %v", err)
	}

	for _, k := range keys {
		var dest string
		if !errors.Is(ts.Recall(ctx(), k, &dest), ErrNotFound) {
			t.Errorf("key '%s' harus sudah hilang setelah VanishMany()", k)
		}
	}
}

// ── Test: Dispel ─────────────────────────────────────────────────

func TestDispel_Wildcard(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	// Simpan beberapa key report
	_ = ts.Haunt(ctx(), "report:2024:Q1", "data1", time.Minute)
	_ = ts.Haunt(ctx(), "report:2024:Q2", "data2", time.Minute)
	_ = ts.Haunt(ctx(), "report:2024:Q3", "data3", time.Minute)
	_ = ts.Haunt(ctx(), "invoice:001", "data4", time.Minute) // harus tetap ada

	deleted, err := ts.Dispel(ctx(), "report:*")
	if err != nil {
		t.Fatalf("Dispel() gagal: %v", err)
	}
	if deleted != 3 {
		t.Errorf("Dispel() deleted = %d, want 3", deleted)
	}

	// invoice harus masih ada
	var dest string
	if errors.Is(ts.Recall(ctx(), "invoice:001", &dest), ErrNotFound) {
		t.Error("invoice:001 tidak seharusnya ikut terhapus")
	}
}

// ── Test: Exists & TTL ───────────────────────────────────────────

func TestExists(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	_ = ts.Haunt(ctx(), "ada", "val", time.Minute)

	ok, err := ts.Exists(ctx(), "ada")
	if err != nil || !ok {
		t.Errorf("Exists('ada') = %v, %v; want true, nil", ok, err)
	}

	ok, err = ts.Exists(ctx(), "tidak-ada")
	if err != nil || ok {
		t.Errorf("Exists('tidak-ada') = %v, %v; want false, nil", ok, err)
	}
}

func TestTTL(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	_ = ts.Haunt(ctx(), "expiring", "val", 30*time.Second)

	ttl, err := ts.TTL(ctx(), "expiring")
	if err != nil {
		t.Fatalf("TTL() gagal: %v", err)
	}
	// TTL harus antara 0 dan 30 detik
	if ttl <= 0 || ttl > 30*time.Second {
		t.Errorf("TTL() = %v, want antara 0 dan 30s", ttl)
	}
}

func TestRefresh(t *testing.T) {
	s, mr := newTestShadow(t)
	ts := s.ForTenant("test")

	_ = ts.Haunt(ctx(), "session:xyz", "data", 10*time.Second)

	// Maju 5 detik
	mr.FastForward(5 * time.Second)

	// Refresh TTL menjadi 20 detik lagi
	if err := ts.Refresh(ctx(), "session:xyz", 20*time.Second); err != nil {
		t.Fatalf("Refresh() gagal: %v", err)
	}

	// Maju 15 detik lagi (total 20 detik sejak Refresh)
	mr.FastForward(15 * time.Second)

	// Key harus masih ada karena TTL di-refresh
	var dest string
	if errors.Is(ts.Recall(ctx(), "session:xyz", &dest), ErrNotFound) {
		t.Error("key harus masih ada setelah Refresh()")
	}
}

func TestRefresh_NotFound(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	err := ts.Refresh(ctx(), "tidak:ada", time.Minute)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Refresh() key tidak ada harus ErrNotFound, got: %v", err)
	}
}

// ── Test: Counter ────────────────────────────────────────────────

func TestIncrement(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	v1, err := ts.Increment(ctx(), "counter:login")
	if err != nil || v1 != 1 {
		t.Errorf("Increment() pertama = %d, %v; want 1, nil", v1, err)
	}

	v2, _ := ts.Increment(ctx(), "counter:login")
	v3, _ := ts.Increment(ctx(), "counter:login")

	if v2 != 2 || v3 != 3 {
		t.Errorf("counter = %d, %d; want 2, 3", v2, v3)
	}
}

func TestIncrementBy(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("test")

	v, err := ts.IncrementBy(ctx(), "stock:item-A", 100)
	if err != nil || v != 100 {
		t.Errorf("IncrementBy(100) = %d, %v; want 100, nil", v, err)
	}

	v, _ = ts.IncrementBy(ctx(), "stock:item-A", -30)
	if v != 70 {
		t.Errorf("IncrementBy(-30) = %d, want 70", v)
	}
}

// ── Test: Ping ────────────────────────────────────────────────────

func TestPing(t *testing.T) {
	s, _ := newTestShadow(t)
	if err := s.Ping(ctx()); err != nil {
		t.Errorf("Ping() gagal: %v", err)
	}
}

// ── Test: BuildKey ────────────────────────────────────────────────

func TestBuildKey(t *testing.T) {
	tests := []struct {
		parts []string
		want  string
	}{
		{[]string{"invoice", "abc123"}, "invoice:abc123"},
		{[]string{"report", "2024", "Q1"}, "report:2024:Q1"},
		{[]string{"a"}, "a"},
		{[]string{"user", "123", "profile"}, "user:123:profile"},
	}
	for _, tt := range tests {
		got := BuildKey(tt.parts...)
		if got != tt.want {
			t.Errorf("BuildKey(%v) = %q, want %q", tt.parts, got, tt.want)
		}
	}
}

// ── Test: FlushTenant ────────────────────────────────────────────

func TestFlushTenant(t *testing.T) {
	s, _ := newTestShadow(t)
	ts := s.ForTenant("flush-test")

	for i := 0; i < 5; i++ {
		_ = ts.Haunt(ctx(), BuildKey("key", string(rune('0'+i))), "val", time.Minute)
	}

	n, err := ts.FlushTenant(ctx())
	if err != nil {
		t.Fatalf("FlushTenant() gagal: %v", err)
	}
	if n != 5 {
		t.Errorf("FlushTenant() deleted = %d, want 5", n)
	}
}
