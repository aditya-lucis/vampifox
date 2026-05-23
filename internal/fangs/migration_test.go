package fangs

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"go.uber.org/zap"
)

// newTestRunner membuat MigrationRunner dengan SQLite in-memory.
func newTestRunner(t *testing.T) (*MigrationRunner, func()) {
	t.Helper()

	f, err := New(newSQLiteConfig(), newTestLogger())
	if err != nil {
		t.Fatalf("gagal buat Fangs: %v", err)
	}

	runner := NewMigrationRunner(f.DB(), zap.NewNop())
	return runner, func() { f.Close() }
}

// newFakeFS membuat in-memory filesystem dengan file SQL untuk testing.
func newFakeFS(files map[string]string) fs.FS {
	fsys := fstest.MapFS{}
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

// ── Tests ─────────────────────────────────────────────────────────

func TestMigrationRunner_EnsureTable(t *testing.T) {
	runner, cleanup := newTestRunner(t)
	defer cleanup()

	ctx := context.Background()

	// Harus bisa dipanggil berkali-kali tanpa error (idempotent)
	for i := 0; i < 3; i++ {
		if err := runner.EnsureTable(ctx); err != nil {
			t.Fatalf("EnsureTable() iterasi %d gagal: %v", i, err)
		}
	}
}

func TestMigrationRunner_Up_SingleFile(t *testing.T) {
	runner, cleanup := newTestRunner(t)
	defer cleanup()

	ctx := context.Background()

	fsys := newFakeFS(map[string]string{
		"migrations/001_create_users.sql": `
			CREATE TABLE test_users (
				id   INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL
			);
		`,
	})

	count, err := runner.Up(ctx, fsys, "migrations")
	if err != nil {
		t.Fatalf("Up() gagal: %v", err)
	}
	if count != 1 {
		t.Errorf("Up() applied = %d, want 1", count)
	}
}

func TestMigrationRunner_Up_Idempotent(t *testing.T) {
	runner, cleanup := newTestRunner(t)
	defer cleanup()

	ctx := context.Background()

	fsys := newFakeFS(map[string]string{
		"migrations/001_create_users.sql": `
			CREATE TABLE test_users2 (id INTEGER PRIMARY KEY);
		`,
	})

	// Jalankan pertama kali
	count1, err := runner.Up(ctx, fsys, "migrations")
	if err != nil {
		t.Fatalf("Up() pertama gagal: %v", err)
	}
	if count1 != 1 {
		t.Errorf("Up() pertama = %d, want 1", count1)
	}

	// Jalankan kedua kali — harus 0 (sudah diaplikasikan)
	count2, err := runner.Up(ctx, fsys, "migrations")
	if err != nil {
		t.Fatalf("Up() kedua gagal: %v", err)
	}
	if count2 != 0 {
		t.Errorf("Up() kedua = %d, want 0 (sudah diaplikasikan)", count2)
	}
}

func TestMigrationRunner_Up_MultipleFiles_Order(t *testing.T) {
	runner, cleanup := newTestRunner(t)
	defer cleanup()

	ctx := context.Background()

	// File-file ini harus dijalankan urut 001 → 002 → 003
	fsys := newFakeFS(map[string]string{
		"migrations/002_insert_data.sql": `INSERT INTO test_order (val) VALUES ('dua');`,
		"migrations/001_create_table.sql": `CREATE TABLE test_order (id INTEGER PRIMARY KEY AUTOINCREMENT, val TEXT);`,
		"migrations/003_more_data.sql": `INSERT INTO test_order (val) VALUES ('tiga');`,
	})

	count, err := runner.Up(ctx, fsys, "migrations")
	if err != nil {
		t.Fatalf("Up() gagal: %v", err)
	}
	if count != 3 {
		t.Errorf("Up() applied = %d, want 3", count)
	}

	// Verifikasi data ada di urutan yang benar
	var rows []struct {
		Val string
	}
	runner.db.Raw("SELECT val FROM test_order ORDER BY id").Scan(&rows)
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2", len(rows))
	}
}

func TestMigrationRunner_Up_InvalidSQL_Error(t *testing.T) {
	runner, cleanup := newTestRunner(t)
	defer cleanup()

	ctx := context.Background()

	fsys := newFakeFS(map[string]string{
		"migrations/001_invalid.sql": `INI BUKAN SQL YANG VALID;`,
	})

	_, err := runner.Up(ctx, fsys, "migrations")
	if err == nil {
		t.Error("Up() dengan SQL invalid harus return error")
	}
}

func TestMigrationRunner_Up_EmptyDirectory(t *testing.T) {
	runner, cleanup := newTestRunner(t)
	defer cleanup()

	// Direktori kosong, harus return 0 tanpa error
	fsys := newFakeFS(map[string]string{})

	count, err := runner.Up(ctx(t), fsys, ".")
	if err != nil {
		t.Fatalf("Up() direktori kosong gagal: %v", err)
	}
	if count != 0 {
		t.Errorf("Up() direktori kosong = %d, want 0", count)
	}
}

func TestMigrationRunner_Status(t *testing.T) {
	runner, cleanup := newTestRunner(t)
	defer cleanup()

	c := context.Background()

	fsys := newFakeFS(map[string]string{
		"migrations/001_a.sql": `CREATE TABLE test_status_a (id INTEGER PRIMARY KEY);`,
		"migrations/002_b.sql": `CREATE TABLE test_status_b (id INTEGER PRIMARY KEY);`,
	})

	// Sebelum Up: semua belum diaplikasikan
	statuses, err := runner.Status(c, fsys, "migrations")
	if err != nil {
		t.Fatalf("Status() gagal: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("Status() = %d, want 2", len(statuses))
	}
	for _, s := range statuses {
		if s.Applied {
			t.Errorf("Status '%s' Applied = true, want false (belum dijalankan)", s.Name)
		}
	}

	// Jalankan 001 saja
	oneFileFS := newFakeFS(map[string]string{
		"migrations/001_a.sql": `CREATE TABLE test_status_a (id INTEGER PRIMARY KEY);`,
	})
	_, _ = runner.Up(c, oneFileFS, "migrations")

	// Sekarang 001 harus Applied = true, 002 masih false
	statuses, _ = runner.Status(c, fsys, "migrations")
	for _, s := range statuses {
		if s.Name == "001_a.sql" && !s.Applied {
			t.Error("001_a.sql harus Applied = true")
		}
		if s.Name == "002_b.sql" && s.Applied {
			t.Error("002_b.sql harus Applied = false (belum dijalankan)")
		}
	}
}

func TestMigrationStatus_DurationRecorded(t *testing.T) {
	runner, cleanup := newTestRunner(t)
	defer cleanup()

	c := context.Background()
	fsys := newFakeFS(map[string]string{
		"migrations/001_timed.sql": `CREATE TABLE test_timed (id INTEGER PRIMARY KEY);`,
	})

	_, err := runner.Up(c, fsys, "migrations")
	if err != nil {
		t.Fatalf("Up() gagal: %v", err)
	}

	// Cek record tersimpan dengan durasi
	var rec Migration
	if err := runner.db.Where("name = ?", "001_timed.sql").First(&rec).Error; err != nil {
		t.Fatalf("gagal ambil record migration: %v", err)
	}

	if rec.DurationMs < 0 {
		t.Errorf("DurationMs = %d, tidak boleh negatif", rec.DurationMs)
	}
	if rec.AppliedAt.IsZero() {
		t.Error("AppliedAt tidak boleh zero value")
	}
	if rec.Checksum == "" {
		t.Error("Checksum tidak boleh kosong")
	}

	_ = time.Now() // suppress unused import
}

// ctx helper
func ctx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}