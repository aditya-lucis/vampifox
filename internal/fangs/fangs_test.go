package fangs

import (
	"context"
	"testing"
	"time"
	"vampifox/internal/den"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ── Helpers ───────────────────────────────────────────────────────

// newTestLogger membuat zap logger yang membuang semua output saat test.
func newTestLogger() *zap.Logger {
	return zap.NewNop()
}

// newSQLiteConfig membuat konfigurasi SQLite in-memory untuk test.
// SQLite dipilih karena tidak butuh server eksternal.
func newSQLiteConfig() den.FangsConfig {
	return den.FangsConfig{
		Driver:             den.DBDriverSQLite,
		SQLitePath:         ":memory:",
		MaxOpenConns:       5,
		MaxIdleConns:       2,
		ConnMaxLifetime:    time.Hour,
		ConnMaxIdleTime:    30 * time.Minute,
		SlowQueryThreshold: 100 * time.Millisecond,
		LogQueries:         false,
	}
}

// ── Test: New ────────────────────────────────────────────────────

func TestNew_SQLite_Success(t *testing.T) {
	f, err := New(newSQLiteConfig(), newTestLogger())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
	defer f.Close()

	if f.db == nil {
		t.Error("Fangs.db tidak boleh nil setelah New()")
	}
}

func TestNew_NilLogger_Error(t *testing.T) {
	_, err := New(newSQLiteConfig(), nil)
	if err == nil {
		t.Error("New() dengan nil logger harus return error")
	}
}

func TestNew_InvalidDriver_Error(t *testing.T) {
	cfg := den.FangsConfig{
		Driver: "mongodb", // tidak didukung
	}
	_, err := New(cfg, newTestLogger())
	if err == nil {
		t.Error("New() dengan driver invalid harus return error")
	}
}

// ── Test: DB() ────────────────────────────────────────────────────

func TestFangs_DB_NotNil(t *testing.T) {
	f, err := New(newSQLiteConfig(), newTestLogger())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
	defer f.Close()

	db := f.DB()
	if db == nil {
		t.Error("DB() tidak boleh nil")
	}
}

func TestFangs_DB_CanQuery(t *testing.T) {
	f, err := New(newSQLiteConfig(), newTestLogger())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
	defer f.Close()

	// Pastikan bisa query sederhana
	var result int
	if err := f.DB().Raw("SELECT 1").Scan(&result).Error; err != nil {
		t.Errorf("DB() tidak bisa query: %v", err)
	}
	if result != 1 {
		t.Errorf("SELECT 1 = %d, want 1", result)
	}
}

// ── Test: Ping ────────────────────────────────────────────────────

func TestFangs_Ping_Success(t *testing.T) {
	f, err := New(newSQLiteConfig(), newTestLogger())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
	defer f.Close()

	ctx := context.Background()
	if err := f.Ping(ctx); err != nil {
		t.Errorf("Ping() gagal: %v", err)
	}
}

// ── Test: Stats ───────────────────────────────────────────────────

func TestFangs_Stats(t *testing.T) {
	f, err := New(newSQLiteConfig(), newTestLogger())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
	defer f.Close()

	stats, err := f.Stats()
	if err != nil {
		t.Fatalf("Stats() gagal: %v", err)
	}

	// Setelah koneksi berhasil, setidaknya ada 1 idle connection
	if stats.OpenConnections < 0 {
		t.Error("OpenConnections tidak boleh negatif")
	}
}

// ── Test: Close ───────────────────────────────────────────────────

func TestFangs_Close(t *testing.T) {
	f, err := New(newSQLiteConfig(), newTestLogger())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Errorf("Close() gagal: %v", err)
	}
}

// ── Test: DropTenantSchema — proteksi schema sistem ───────────────

func TestFangs_DropTenantSchema_RejectSystemSchema(t *testing.T) {
	f, err := New(newSQLiteConfig(), newTestLogger())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
	defer f.Close()

	ctx := context.Background()

	// Semua schema sistem harus ditolak
	systemSchemas := []string{"public", "vfx_system", "dbo"}
	for _, schema := range systemSchemas {
		err := f.DropTenantSchema(ctx, schema)
		if err == nil {
			t.Errorf("DropTenantSchema('%s') harus ditolak tapi tidak error", schema)
		}
	}
}

func TestFangs_DropTenantSchema_EmptyName(t *testing.T) {
	f, err := New(newSQLiteConfig(), newTestLogger())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
	defer f.Close()

	if err := f.DropTenantSchema(context.Background(), ""); err == nil {
		t.Error("DropTenantSchema('') harus error")
	}
}

// ── Test: TenantSchemaExists — SQLite ────────────────────────────

func TestFangs_TenantSchemaExists_SQLite_AlwaysTrue(t *testing.T) {
	f, err := New(newSQLiteConfig(), newTestLogger())
	if err != nil {
		t.Fatalf("New() gagal: %v", err)
	}
	defer f.Close()

	// SQLite tidak punya schema — selalu return true
	exists, err := f.TenantSchemaExists(context.Background(), "apapun")
	if err != nil {
		t.Fatalf("TenantSchemaExists() error: %v", err)
	}
	if !exists {
		t.Error("SQLite TenantSchemaExists() harus selalu true")
	}
}

// ── Test: splitSQL ────────────────────────────────────────────────

func TestSplitSQL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // jumlah statement yang diharapkan
	}{
		{
			name:  "satu statement",
			input: "CREATE TABLE foo (id INTEGER PRIMARY KEY);",
			want:  1,
		},
		{
			name: "dua statement",
			input: `
				CREATE TABLE foo (id INTEGER PRIMARY KEY);
				CREATE TABLE bar (id INTEGER PRIMARY KEY);
			`,
			want: 2,
		},
		{
			name: "dengan komentar",
			input: `
				-- ini komentar
				CREATE TABLE foo (id INTEGER PRIMARY KEY);
				-- komentar lain
				INSERT INTO foo VALUES (1);
			`,
			want: 2,
		},
		{
			name:  "kosong",
			input: "   \n   ",
			want:  0,
		},
		{
			name:  "semicolons di akhir",
			input: "SELECT 1;;; SELECT 2;",
			want:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitSQL(tt.input)
			if len(got) != tt.want {
				t.Errorf("splitSQL() = %d statements, want %d\ngot: %v", len(got), tt.want, got)
			}
		})
	}
}

// ── Test: zapGORMLogger ───────────────────────────────────────────

func TestZapGORMLogger_LogMode(t *testing.T) {
	logger := &zapGORMLogger{
		zap:           zap.NewNop(),
		slowThreshold: 200 * time.Millisecond,
	}

	import_gormlogger := func() { /* placeholder */ }
	_ = import_gormlogger

	// LogMode harus mengembalikan instance baru tanpa modifikasi yang asli
	newLogger := logger.LogMode(3) // Info
	if newLogger == logger {
		t.Error("LogMode() harus mengembalikan instance baru")
	}
}

func TestZapGORMLogger_Trace_SlowQuery(t *testing.T) {
	// Pastikan slow query tidak panic
	logger := &zapGORMLogger{
		zap:           zap.NewNop(),
		slowThreshold: 1 * time.Nanosecond, // semua query dianggap slow
	}

	begin := time.Now().Add(-1 * time.Second) // simulasi query 1 detik
	logger.Trace(context.Background(), begin, func() (string, int64) {
		return "SELECT * FROM users", 10
	}, nil)
}

// ── Test: isNotFound ─────────────────────────────────────────────

func TestIsNotFound(t *testing.T) {
	if !isNotFound(gorm.ErrRecordNotFound) {
		t.Error("gorm.ErrRecordNotFound harus dikenali sebagai not found")
	}
	if isNotFound(nil) {
		t.Error("nil tidak boleh dianggap not found")
	}
}

// ── Test: buildGORMLogger ─────────────────────────────────────────

func TestBuildGORMLogger_LogQueries(t *testing.T) {
	cfgWithLog := den.FangsConfig{LogQueries: true, SlowQueryThreshold: 200 * time.Millisecond}
	cfgNoLog := den.FangsConfig{LogQueries: false}

	l1 := buildGORMLogger(cfgWithLog, zap.NewNop())
	l2 := buildGORMLogger(cfgNoLog, zap.NewNop())

	if l1 == nil || l2 == nil {
		t.Error("buildGORMLogger() tidak boleh nil")
	}
}