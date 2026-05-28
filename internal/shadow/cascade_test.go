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

// newTestTS membuat TenantShadow untuk testing.
func newTestTS(t *testing.T, slug string) (*TenantShadow, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("gagal start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	cfg := config.ShadowConfig{
		Addr:         mr.Addr(),
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	s, err := New(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	return s.ForTenant(slug), mr
}

// ── Test: RegisterDeps ───────────────────────────────────────────

func TestCascade_RegisterDeps(t *testing.T) {
	c := NewCascade(zap.NewNop())

	c.RegisterDeps("invoice", []string{
		"invoice:{id}",
		"report:ar:*",
		"dashboard:finance",
	})

	deps := c.Deps("invoice")
	if len(deps) != 3 {
		t.Errorf("Deps('invoice') = %d, want 3", len(deps))
	}

	// RegisterDeps kedua kali harus menambah, bukan replace
	c.RegisterDeps("invoice", []string{"summary:*"})
	deps = c.Deps("invoice")
	if len(deps) != 4 {
		t.Errorf("Deps('invoice') setelah tambah = %d, want 4", len(deps))
	}
}

func TestCascade_Deps_NotRegistered(t *testing.T) {
	c := NewCascade(zap.NewNop())
	deps := c.Deps("belum-ada")
	if deps != nil {
		t.Errorf("Deps entity tidak terdaftar harus nil, got %v", deps)
	}
}

// ── Test: Invalidate ─────────────────────────────────────────────

func TestCascade_Invalidate_ExactKey(t *testing.T) {
	c := NewCascade(zap.NewNop())
	ts, _ := newTestTS(t, "test-tenant")

	c.RegisterDeps("invoice", []string{
		"invoice:{id}",      // key eksak dengan ID
		"dashboard:finance", // key eksak tanpa ID
	})

	// Isi cache
	_ = ts.Haunt(context.Background(), "invoice:inv-001", "data", time.Minute)
	_ = ts.Haunt(context.Background(), "dashboard:finance", "data", time.Minute)
	_ = ts.Haunt(context.Background(), "invoice:inv-002", "data", time.Minute) // tidak boleh terhapus

	// Invalidate invoice inv-001
	n, err := c.Invalidate(context.Background(), ts, "invoice", "inv-001")
	if err != nil {
		t.Fatalf("Invalidate() gagal: %v", err)
	}
	if n < 1 {
		t.Errorf("Invalidate() deleted = %d, want >= 1", n)
	}

	// invoice:inv-001 harus hilang
	var dest string
	if !errors.Is(ts.Recall(context.Background(), "invoice:inv-001", &dest), ErrNotFound) {
		t.Error("invoice:inv-001 harus sudah di-invalidate")
	}

	// dashboard:finance harus hilang
	if !errors.Is(ts.Recall(context.Background(), "dashboard:finance", &dest), ErrNotFound) {
		t.Error("dashboard:finance harus sudah di-invalidate")
	}

	// invoice:inv-002 harus masih ada
	if errors.Is(ts.Recall(context.Background(), "invoice:inv-002", &dest), ErrNotFound) {
		t.Error("invoice:inv-002 tidak seharusnya terhapus")
	}
}

func TestCascade_Invalidate_WildcardPattern(t *testing.T) {
	c := NewCascade(zap.NewNop())
	ts, _ := newTestTS(t, "test-tenant")

	c.RegisterDeps("product", []string{
		"report:sales:*",     // hapus semua cache report sales
	})

	_ = ts.Haunt(context.Background(), "report:sales:2024:Q1", "d1", time.Minute)
	_ = ts.Haunt(context.Background(), "report:sales:2024:Q2", "d2", time.Minute)
	_ = ts.Haunt(context.Background(), "report:sales:2024:Q3", "d3", time.Minute)
	_ = ts.Haunt(context.Background(), "report:finance:2024", "d4", time.Minute) // tidak terhapus

	n, err := c.Invalidate(context.Background(), ts, "product", "any-id")
	if err != nil {
		t.Fatalf("Invalidate() gagal: %v", err)
	}
	if n != 3 {
		t.Errorf("Invalidate() deleted = %d, want 3", n)
	}

	// report:finance harus masih ada
	var dest string
	if errors.Is(ts.Recall(context.Background(), "report:finance:2024", &dest), ErrNotFound) {
		t.Error("report:finance:2024 tidak seharusnya terhapus")
	}
}

func TestCascade_Invalidate_NoRegisteredDeps(t *testing.T) {
	c := NewCascade(zap.NewNop())
	ts, _ := newTestTS(t, "test")

	// Entity tidak terdaftar — harus 0 deleted, no error
	n, err := c.Invalidate(context.Background(), ts, "unknown-entity", "id")
	if err != nil {
		t.Errorf("Invalidate() entity tidak terdaftar harus tidak error, got: %v", err)
	}
	if n != 0 {
		t.Errorf("Invalidate() entity tidak terdaftar harus 0, got: %d", n)
	}
}

func TestCascade_Invalidate_IDPlaceholder(t *testing.T) {
	c := NewCascade(zap.NewNop())
	ts, _ := newTestTS(t, "test")

	c.RegisterDeps("customer", []string{
		"customer:{id}:invoices",
		"customer:{id}:balance",
		"customer:{id}:profile",
	})

	customerID := "cust-999"
	_ = ts.Haunt(context.Background(), "customer:cust-999:invoices", "inv", time.Minute)
	_ = ts.Haunt(context.Background(), "customer:cust-999:balance", "bal", time.Minute)
	_ = ts.Haunt(context.Background(), "customer:cust-999:profile", "pro", time.Minute)
	_ = ts.Haunt(context.Background(), "customer:cust-888:profile", "other", time.Minute) // tenant lain

	n, err := c.Invalidate(context.Background(), ts, "customer", customerID)
	if err != nil {
		t.Fatalf("Invalidate() gagal: %v", err)
	}
	if n != 3 {
		t.Errorf("Invalidate() deleted = %d, want 3", n)
	}

	// customer:cust-888 harus tidak tersentuh
	var dest string
	if errors.Is(ts.Recall(context.Background(), "customer:cust-888:profile", &dest), ErrNotFound) {
		t.Error("customer:cust-888:profile tidak seharusnya terhapus")
	}
}

// ── Test: InvalidateMany ─────────────────────────────────────────

func TestCascade_InvalidateMany(t *testing.T) {
	c := NewCascade(zap.NewNop())
	ts, _ := newTestTS(t, "test")

	c.RegisterDeps("item", []string{"item:{id}"})

	ids := []string{"item-1", "item-2", "item-3"}
	for _, id := range ids {
		_ = ts.Haunt(context.Background(), "item:"+id, "val", time.Minute)
	}

	n, err := c.InvalidateMany(context.Background(), ts, "item", ids)
	if err != nil {
		t.Fatalf("InvalidateMany() gagal: %v", err)
	}
	if n != 3 {
		t.Errorf("InvalidateMany() deleted = %d, want 3", n)
	}
}

// ── Test: InvalidateType ─────────────────────────────────────────

func TestCascade_InvalidateType(t *testing.T) {
	c := NewCascade(zap.NewNop())
	ts, _ := newTestTS(t, "test")

	_ = ts.Haunt(context.Background(), "product:001", "a", time.Minute)
	_ = ts.Haunt(context.Background(), "product:002", "b", time.Minute)
	_ = ts.Haunt(context.Background(), "product:003", "c", time.Minute)
	_ = ts.Haunt(context.Background(), "invoice:001", "d", time.Minute) // tidak terhapus

	n, err := c.InvalidateType(context.Background(), ts, "product")
	if err != nil {
		t.Fatalf("InvalidateType() gagal: %v", err)
	}
	if n != 3 {
		t.Errorf("InvalidateType() deleted = %d, want 3", n)
	}

	// invoice harus masih ada
	var dest string
	if errors.Is(ts.Recall(context.Background(), "invoice:001", &dest), ErrNotFound) {
		t.Error("invoice:001 tidak seharusnya terhapus oleh InvalidateType('product')")
	}
}

// ── Test: Graph inspection ───────────────────────────────────────

func TestCascade_AllEntityTypes(t *testing.T) {
	c := NewCascade(zap.NewNop())

	c.RegisterDeps("invoice", []string{"a"})
	c.RegisterDeps("product", []string{"b"})
	c.RegisterDeps("customer", []string{"c"})

	types := c.AllEntityTypes()
	if len(types) != 3 {
		t.Errorf("AllEntityTypes() = %d, want 3", len(types))
	}
}

func TestCascade_TotalDeps(t *testing.T) {
	c := NewCascade(zap.NewNop())

	c.RegisterDeps("invoice", []string{"a", "b", "c"})
	c.RegisterDeps("product", []string{"d", "e"})

	if total := c.TotalDeps(); total != 5 {
		t.Errorf("TotalDeps() = %d, want 5", total)
	}
}

// ── Test: Thread safety ──────────────────────────────────────────

func TestCascade_ConcurrentRegister(t *testing.T) {
	c := NewCascade(zap.NewNop())

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(i int) {
			c.RegisterDeps("entity", []string{"pattern:*"})
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Tidak boleh ada data race atau panic
	deps := c.Deps("entity")
	if len(deps) == 0 {
		t.Error("Deps harus ada setelah concurrent RegisterDeps")
	}
}
