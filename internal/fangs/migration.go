// Package fangs — Migration runner.
//
// VampiFox menggunakan SQL migration files murni (bukan Go structs).
// Keputusan ini disengaja:
//   - SQL migration bisa di-review oleh DBA tanpa harus baca Go code
//   - Mudah di-port antar driver (dengan sedikit penyesuaian dialect)
//   - Tidak ada magic — yang kamu tulis di file SQL persis yang dieksekusi
//
// Struktur folder migration:
//
//	migrations/
//	  system/          ← migration untuk schema sistem (tenants, dll)
//	    001_init.sql
//	    002_add_tenants.sql
//	  modules/
//	    accounting/    ← migration per-module, dijalankan per-tenant
//	      001_accounts.sql
//	      002_invoices.sql
//	    inventory/
//	      001_items.sql
package fangs

import (
	"context"
	"crypto/md5"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ═══════════════════════════════════════════════════════════════
//  Migration model
// ═══════════════════════════════════════════════════════════════

// Migration merepresentasikan satu file migration yang sudah dijalankan.
// Disimpan di tabel _vfx_migrations di setiap schema (sistem maupun tenant).
type Migration struct {
	ID          uint      `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"uniqueIndex;not null"` // nama file, e.g. "001_init.sql"
	Checksum    string    `gorm:"not null"`             // MD5 isi file — deteksi perubahan
	AppliedAt   time.Time `gorm:"not null;autoCreateTime"`
	DurationMs  int64     `gorm:"not null"` // berapa ms migration ini berjalan
}

// TableName override agar GORM pakai nama tabel yang konsisten.
func (Migration) TableName() string { return "_vfx_migrations" }

// ═══════════════════════════════════════════════════════════════
//  MigrationRunner
// ═══════════════════════════════════════════════════════════════

// MigrationRunner menjalankan SQL migration files terhadap satu *gorm.DB.
type MigrationRunner struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewMigrationRunner membuat MigrationRunner baru.
func NewMigrationRunner(db *gorm.DB, logger *zap.Logger) *MigrationRunner {
	return &MigrationRunner{
		db:     db,
		logger: logger.Named("migration"),
	}
}

// EnsureTable memastikan tabel _vfx_migrations ada di database.
// Dipanggil sebelum Up() — aman dipanggil berkali-kali (idempotent).
func (r *MigrationRunner) EnsureTable(ctx context.Context) error {
	if err := r.db.WithContext(ctx).AutoMigrate(&Migration{}); err != nil {
		return fmt.Errorf("[Migration] gagal membuat tabel _vfx_migrations: %w", err)
	}
	return nil
}

// Up menjalankan semua migration dari filesystem yang belum diaplikasikan.
//
// fsys adalah fs.FS yang berisi file-file SQL.
// dir adalah direktori di dalam fsys yang akan di-scan.
//
// Contoh penggunaan dari module accounting:
//
//	//go:embed migrations/*.sql
//	var migrationFS embed.FS
//
//	runner := fangs.NewMigrationRunner(tenantDB, logger)
//	runner.Up(ctx, migrationFS, "migrations")
func (r *MigrationRunner) Up(ctx context.Context, fsys fs.FS, dir string) (int, error) {
	// ── Pastikan tabel tracking ada ──────────────────────────────
	if err := r.EnsureTable(ctx); err != nil {
		return 0, err
	}

	// ── Ambil daftar migration yang sudah dijalankan ──────────────
	applied, err := r.appliedMigrations(ctx)
	if err != nil {
		return 0, err
	}

	// ── Scan file SQL dari filesystem ─────────────────────────────
	files, err := r.scanFiles(fsys, dir)
	if err != nil {
		return 0, err
	}

	// ── Jalankan yang belum diaplikasikan ─────────────────────────
	count := 0
	for _, file := range files {
		name := filepath.Base(file)

		// Skip jika sudah pernah dijalankan
		if _, ok := applied[name]; ok {
			r.logger.Debug("Migration sudah diaplikasikan, skip",
				zap.String("file", name),
			)
			continue
		}

		// Baca isi file
		content, err := fs.ReadFile(fsys, file)
		if err != nil {
			return count, fmt.Errorf("[Migration] gagal membaca file '%s': %w", name, err)
		}

		// Deteksi perubahan: jika file sudah dijalankan tapi checksumnya beda
		// ini peringatan serius — migration tidak boleh diubah setelah diaplikasikan
		checksum := fmt.Sprintf("%x", md5.Sum(content))
		if rec, ok := applied[name]; ok && rec.Checksum != checksum {
			r.logger.Warn("⚠️  Checksum migration berubah setelah diaplikasikan!",
				zap.String("file", name),
				zap.String("recorded_checksum", rec.Checksum),
				zap.String("current_checksum", checksum),
			)
			// Kita tidak halt — hanya warning. Bisa dijadikan error jika policy lebih ketat.
		}

		// Jalankan migration
		if err := r.runFile(ctx, name, string(content), checksum); err != nil {
			return count, err
		}
		count++
	}

	if count > 0 {
		r.logger.Info("Migration selesai",
			zap.Int("applied", count),
			zap.Int("total_files", len(files)),
		)
	} else {
		r.logger.Debug("Tidak ada migration baru")
	}

	return count, nil
}

// Status mengembalikan daftar migration dan status masing-masing.
// Berguna untuk foxctl migrate status.
func (r *MigrationRunner) Status(ctx context.Context, fsys fs.FS, dir string) ([]MigrationStatus, error) {
	applied, err := r.appliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	files, err := r.scanFiles(fsys, dir)
	if err != nil {
		return nil, err
	}

	var statuses []MigrationStatus
	for _, file := range files {
		name := filepath.Base(file)
		rec, ok := applied[name]

		status := MigrationStatus{Name: name}
		if ok {
			status.Applied = true
			status.AppliedAt = rec.AppliedAt
			status.DurationMs = rec.DurationMs
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// MigrationStatus status satu migration untuk ditampilkan di CLI.
type MigrationStatus struct {
	Name       string
	Applied    bool
	AppliedAt  time.Time
	DurationMs int64
}

// ═══════════════════════════════════════════════════════════════
//  Internal helpers
// ═══════════════════════════════════════════════════════════════

// runFile menjalankan satu file SQL di dalam satu transaksi.
func (r *MigrationRunner) runFile(ctx context.Context, name, sql, checksum string) error {
	start := time.Now()

	r.logger.Info("Menjalankan migration", zap.String("file", name))

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Pisahkan per statement (beberapa file punya multiple statements)
		statements := splitSQL(sql)
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("statement gagal: %w\nSQL: %s", err, stmt)
			}
		}

		// Catat ke tabel tracking
		record := Migration{
			Name:       name,
			Checksum:   checksum,
			DurationMs: time.Since(start).Milliseconds(),
		}
		return tx.Create(&record).Error
	})

	if err != nil {
		return fmt.Errorf("[Migration] file '%s' gagal: %w", name, err)
	}

	r.logger.Info("✅ Migration berhasil",
		zap.String("file", name),
		zap.Duration("elapsed", time.Since(start)),
	)
	return nil
}

// appliedMigrations mengambil semua migration yang sudah dijalankan dari DB.
func (r *MigrationRunner) appliedMigrations(ctx context.Context) (map[string]Migration, error) {
	var records []Migration
	if err := r.db.WithContext(ctx).Find(&records).Error; err != nil {
		// Jika tabel belum ada, kembalikan map kosong
		if strings.Contains(err.Error(), "not exist") ||
			strings.Contains(err.Error(), "doesn't exist") ||
			strings.Contains(err.Error(), "no such table") {
			return map[string]Migration{}, nil
		}
		return nil, fmt.Errorf("[Migration] gagal membaca tabel tracking: %w", err)
	}

	result := make(map[string]Migration, len(records))
	for _, r := range records {
		result[r.Name] = r
	}
	return result, nil
}

// scanFiles mencari semua file .sql di direktori, diurutkan by nama.
func (r *MigrationRunner) scanFiles(fsys fs.FS, dir string) ([]string, error) {
	var files []string

	err := fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".sql") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("[Migration] gagal scan direktori '%s': %w", dir, err)
	}

	// Urut by nama file — konvensi: 001_..., 002_..., dst.
	sort.Strings(files)
	return files, nil
}

// splitSQL memisahkan SQL string menjadi individual statements.
// Pemisah: semicolon (;) di akhir baris.
func splitSQL(sql string) []string {
	// Hapus komentar -- dan /* */
	var cleaned strings.Builder
	lines := strings.Split(sql, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		cleaned.WriteString(line)
		cleaned.WriteString("\n")
	}

	// Split by semicolon
	parts := strings.Split(cleaned.String(), ";")
	var statements []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			statements = append(statements, p)
		}
	}
	return statements
}
