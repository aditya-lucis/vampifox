// Package fangs — Database layer VampiFox.
//
// "Fangs" adalah cara vampire menyerap sumber daya.
// Fangs mengelola koneksi ke RDBMS apapun yang didukung GORM:
// PostgreSQL, MySQL, SQL Server, dan SQLite.
//
// Fangs tidak peduli dengan business logic — ia hanya bertanggung jawab
// atas koneksi, pool, logging query, dan isolasi schema per-tenant.
//
// Alur penggunaan:
//
//	f, err := fangs.New(cfg)              // buka koneksi ke database sistem
//	db := f.DB()                          // ambil *gorm.DB untuk query sistem
//	tenantDB, release, err := f.For(ctx, scope)  // scope ke tenant
//	defer release()                       // WAJIB — lepas koneksi ke pool
//	f.Close()                             // tutup semua koneksi
package fangs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/aditya-lucis/vampifox/internal/config"
	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

// schemaNamer adalah alias untuk GORM default NamingStrategy.
// Di-embed ke custom NamingStrategy agar method yang tidak di-override
// tetap berjalan dengan perilaku default.
type schemaNamer = schema.NamingStrategy

// ═══════════════════════════════════════════════════════════════
//  Fangs — main struct
// ═══════════════════════════════════════════════════════════════

// Fangs mengelola koneksi database VampiFox.
// Gunakan New() untuk membuat instance — jangan buat langsung.
type Fangs struct {
	db      *gorm.DB
	cfg     config.FangsConfig
	gormCfg *gorm.Config
	logger  *zap.Logger
}

// New membuka koneksi ke database dan mengembalikan Fangs.
//
// Fangs akan:
//   - Memilih GORM dialect yang sesuai berdasarkan cfg.Driver
//   - Mengkonfigurasi connection pool
//   - Memasang custom query logger (log slow query, log semua query di dev)
//   - Melakukan ping untuk memastikan koneksi berhasil
func New(cfg config.FangsConfig, logger *zap.Logger) (*Fangs, error) {
	if logger == nil {
		return nil, fmt.Errorf("[Fangs] logger tidak boleh nil")
	}

	// ── GORM Config ───────────────────────────────────────────────
	gormCfg := &gorm.Config{
		// PrepareStmt: cache prepared statement — lebih cepat untuk query berulang.
		// Di SQLite kita nonaktifkan karena tidak support concurrent prepared stmt.
		PrepareStmt:                              cfg.Driver != config.DBDriverSQLite,
		DisableForeignKeyConstraintWhenMigrating: false,
		Logger:                                   buildGORMLogger(cfg, logger),
	}

	// ── Buka koneksi sesuai driver ────────────────────────────────
	db, err := openDB(cfg, gormCfg)
	if err != nil {
		return nil, err
	}

	// ── Connection pool ───────────────────────────────────────────
	if err := configurePool(db, cfg); err != nil {
		return nil, err
	}

	// ── Ping — pastikan koneksi benar-benar hidup ─────────────────
	if err := ping(db); err != nil {
		return nil, fmt.Errorf("[Fangs] database tidak merespons (%s@%s/%s): %w",
			cfg.User, cfg.Host, cfg.DBName, err)
	}

	logger.Info("🦷 Fangs terhubung",
		zap.String("driver", string(cfg.Driver)),
		zap.String("host", cfg.Host),
		zap.String("dbname", cfg.DBName),
		zap.Int("max_open_conns", cfg.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.MaxIdleConns),
	)

	return &Fangs{
		db:      db,
		cfg:     cfg,
		gormCfg: gormCfg,
		logger:  logger,
	}, nil
}

// ═══════════════════════════════════════════════════════════════
//  Accessors
// ═══════════════════════════════════════════════════════════════

// DB mengembalikan *gorm.DB koneksi sistem (bukan tenant-specific).
// Pakai ini untuk query ke tabel sistem seperti tenants, users global, dll.
func (f *Fangs) DB() *gorm.DB {
	return f.db
}

// For mengembalikan *gorm.DB yang sudah di-scope ke schema tenant,
// beserta fungsi release() yang WAJIB dipanggil setelah selesai.
//
// Selalu gunakan defer untuk release:
//
//	db, release, err := fangs.For(ctx, tenant)
//	if err != nil { return err }
//	defer release()
//	db.Find(&invoices)
func (f *Fangs) For(ctx context.Context, tenant TenantScope) (*gorm.DB, func(), error) {
	schemaName := tenant.SchemaName()
	if schemaName == "" {
		f.logger.Warn("[Fangs] For() dipanggil tanpa schema name — fallback ke DB sistem",
			zap.String("tenant", tenant.TenantSlug()),
		)
		return f.db, func() {}, nil
	}

	return f.scopeDB(ctx, schemaName)
}

// scopeDB mengembalikan *gorm.DB yang ter-isolasi ke schema tenant.
//
// Setiap driver punya mekanisme berbeda:
//
//   - PostgreSQL  : pin ke satu *sql.Conn, set search_path di sana.
//     Dengan ConnPool = conn, semua query berikutnya via DB yang sama
//     dijamin pakai koneksi yang sama — tidak ada race condition pool.
//
//   - MySQL       : sama, pin ke *sql.Conn lalu USE database.
//
//   - SQL Server  : pakai GORM NamingStrategy.TablePrefix agar setiap
//     query otomatis menggunakan [schema].[table] notation.
//
//   - SQLite      : pakai NamingStrategy.TablePrefix sebagai namespace.
//     SQLite tidak punya schema, prefix adalah workaround terbaik.
//
// PENTING: Untuk PostgreSQL dan MySQL, caller wajib memanggil
// ReleaseTenantConn(db) setelah selesai agar koneksi kembali ke pool.
// Gunakan defer:
//
//	db, release, err := f.scopeDB(ctx, schemaName)
//	if err != nil { ... }
//	defer release()
func (f *Fangs) scopeDB(ctx context.Context, schemaName string) (*gorm.DB, func(), error) {
	switch f.cfg.Driver {
	case config.DBDriverPostgres:
		return f.pinnedConn(ctx, schemaName,
			fmt.Sprintf("SET search_path TO %s, public", schemaName),
		)

	case config.DBDriverMySQL:
		return f.pinnedConn(ctx, schemaName,
			fmt.Sprintf("USE `%s`", schemaName),
		)

	case config.DBDriverSQLServer:
		// SQL Server: buka DB baru dengan NamingStrategy yang inject [schema]. prefix.
		cfgCopy := *f.gormCfg
		cfgCopy.NamingStrategy = sqlServerNaming{schema: schemaName}
		db, err := gorm.Open(sqlserver.Open(f.cfg.DSN()), &cfgCopy)
		if err != nil {
			return nil, func() {}, fmt.Errorf("[Fangs] gagal buka DB SQLServer untuk schema '%s': %w", schemaName, err)
		}
		return db, func() {}, nil

	case config.DBDriverSQLite:
		// SQLite: buka DB baru dengan NamingStrategy yang inject prefix ke table name.
		cfgCopy := *f.gormCfg
		cfgCopy.NamingStrategy = sqlitePrefixNaming{prefix: schemaName + "_"}
		db, err := gorm.Open(sqlite.Open(f.cfg.SQLitePath), &cfgCopy)
		if err != nil {
			return nil, func() {}, fmt.Errorf("[Fangs] gagal buka DB SQLite untuk schema '%s': %w", schemaName, err)
		}
		return db, func() {}, nil

	default:
		return f.db, func() {}, nil
	}
}

// pinnedConn mengambil satu koneksi dari pool, menjalankan initSQL di sana,
// lalu mengembalikan *gorm.DB yang ter-pin ke koneksi tersebut.
//
// release() WAJIB dipanggil setelah selesai — biasanya via defer.
func (f *Fangs) pinnedConn(ctx context.Context, schemaName, initSQL string) (*gorm.DB, func(), error) {
	sqlDB, err := f.db.DB()
	if err != nil {
		return nil, func() {}, fmt.Errorf("[Fangs] gagal mendapat sql.DB: %w", err)
	}

	// Ambil koneksi dedicated dari pool
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, func() {}, fmt.Errorf("[Fangs] gagal mengambil koneksi untuk schema '%s': %w", schemaName, err)
	}

	// Set schema di koneksi ini
	if _, err := conn.ExecContext(ctx, initSQL); err != nil {
		_ = conn.Close()
		return nil, func() {}, fmt.Errorf("[Fangs] gagal set schema '%s': %w", schemaName, err)
	}

	// Buat *gorm.DB yang pakai koneksi ini saja
	db := f.db.Session(&gorm.Session{
		NewDB: true,
	})

	db.Statement.ConnPool = conn

	release := func() {
		if err := conn.Close(); err != nil {
			f.logger.Warn("[Fangs] gagal melepas koneksi tenant",
				zap.String("schema", schemaName),
				zap.Error(err),
			)
		}
	}

	return db, release, nil
}

// ═══════════════════════════════════════════════════════════════
//  Schema Management
// ═══════════════════════════════════════════════════════════════

// CreateTenantSchema membuat schema baru untuk tenant yang baru didaftarkan.
// Dipanggil satu kali saat tenant di-provision, bukan per-request.
func (f *Fangs) CreateTenantSchema(ctx context.Context, schemaName string) error {
	if schemaName == "" {
		return fmt.Errorf("[Fangs] schema name tidak boleh kosong")
	}

	var query string
	switch f.cfg.Driver {
	case config.DBDriverPostgres:
		query = fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schemaName)
	case config.DBDriverMySQL:
		query = fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", schemaName)
	case config.DBDriverSQLServer:
		// SQL Server: CREATE SCHEMA tidak support IF NOT EXISTS, pakai cek manual
		query = fmt.Sprintf(`
			IF NOT EXISTS (SELECT 1 FROM sys.schemas WHERE name = '%s')
			BEGIN
				EXEC('CREATE SCHEMA [%s]')
			END`, schemaName, schemaName)
	case config.DBDriverSQLite:
		// SQLite tidak punya schema — tidak perlu dibuat
		return nil
	}

	if err := f.db.WithContext(ctx).Exec(query).Error; err != nil {
		return fmt.Errorf("[Fangs] gagal membuat schema '%s': %w", schemaName, err)
	}

	f.logger.Info("Schema tenant dibuat",
		zap.String("schema", schemaName),
		zap.String("driver", string(f.cfg.Driver)),
	)
	return nil
}

// DropTenantSchema menghapus schema tenant beserta semua datanya.
// ⚠️  OPERASI INI TIDAK BISA DIBATALKAN. Pastikan sudah ada backup.
func (f *Fangs) DropTenantSchema(ctx context.Context, schemaName string) error {
	if schemaName == "" {
		return fmt.Errorf("[Fangs] schema name tidak boleh kosong")
	}

	// Proteksi: jangan sampai hapus schema sistem
	if schemaName == "public" || schemaName == "vfx_system" || schemaName == "dbo" {
		return fmt.Errorf("[Fangs] menghapus schema sistem '%s' tidak diizinkan", schemaName)
	}

	var query string
	switch f.cfg.Driver {
	case config.DBDriverPostgres:
		query = fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName)
	case config.DBDriverMySQL:
		query = fmt.Sprintf("DROP DATABASE IF EXISTS %s", schemaName)
	case config.DBDriverSQLServer:
		// SQL Server: harus drop semua object dulu, lalu drop schema
		// Untuk sekarang gunakan dynamic SQL
		query = fmt.Sprintf(`
			DECLARE @sql NVARCHAR(MAX) = ''
			SELECT @sql += 'DROP TABLE [%s].[' + TABLE_NAME + '];'
			FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = '%s'
			EXEC sp_executesql @sql
			IF EXISTS (SELECT 1 FROM sys.schemas WHERE name = '%s')
				DROP SCHEMA [%s]`,
			schemaName, schemaName, schemaName, schemaName)
	case config.DBDriverSQLite:
		return nil
	}

	if err := f.db.WithContext(ctx).Exec(query).Error; err != nil {
		return fmt.Errorf("[Fangs] gagal drop schema '%s': %w", schemaName, err)
	}

	f.logger.Warn("⚠️  Schema tenant dihapus",
		zap.String("schema", schemaName),
	)
	return nil
}

// TenantSchemaExists memeriksa apakah schema tenant sudah ada.
func (f *Fangs) TenantSchemaExists(ctx context.Context, schemaName string) (bool, error) {
	var count int64
	var query string
	var args []interface{}

	switch f.cfg.Driver {
	case config.DBDriverPostgres:
		query = "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = $1"
		args = []interface{}{schemaName}
	case config.DBDriverMySQL:
		query = "SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?"
		args = []interface{}{schemaName}
	case config.DBDriverSQLServer:
		query = "SELECT COUNT(*) FROM sys.schemas WHERE name = @p1"
		args = []interface{}{schemaName}
	case config.DBDriverSQLite:
		// SQLite tidak punya schema, selalu "ada"
		return true, nil
	}

	if err := f.db.WithContext(ctx).Raw(query, args...).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("[Fangs] gagal cek schema '%s': %w", schemaName, err)
	}
	return count > 0, nil
}

// ═══════════════════════════════════════════════════════════════
//  Health & Lifecycle
// ═══════════════════════════════════════════════════════════════

// Ping melakukan health check ke database.
// Cocok dipakai di endpoint /health.
func (f *Fangs) Ping(ctx context.Context) error {
	sqlDB, err := f.db.DB()
	if err != nil {
		return fmt.Errorf("[Fangs] gagal mendapatkan sql.DB: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("[Fangs] database tidak merespons: %w", err)
	}
	return nil
}

// Stats mengembalikan statistik connection pool saat ini.
// Berguna untuk monitoring dan debugging.
func (f *Fangs) Stats() (sql.DBStats, error) {
	sqlDB, err := f.db.DB()
	if err != nil {
		return sql.DBStats{}, fmt.Errorf("[Fangs] gagal mendapatkan sql.DB: %w", err)
	}
	return sqlDB.Stats(), nil
}

// Close menutup semua koneksi database.
// Dipanggil saat Den.Slumber().
func (f *Fangs) Close() error {
	sqlDB, err := f.db.DB()
	if err != nil {
		return fmt.Errorf("[Fangs] gagal mendapatkan sql.DB saat close: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("[Fangs] gagal menutup koneksi: %w", err)
	}
	f.logger.Info("🦷 Fangs ditarik — koneksi database ditutup")
	return nil
}

// ═══════════════════════════════════════════════════════════════
//  TenantScope interface
// ═══════════════════════════════════════════════════════════════

// TenantScope adalah interface minimal yang dibutuhkan Fangs
// untuk menentukan schema database yang tepat.
//
// Package tenant mengimplementasikan interface ini via *tenant.Tenant.
// Dengan interface ini, Fangs tidak punya dependency ke package tenant —
// menghindari circular import.
type TenantScope interface {
	// TenantSlug mengembalikan identifier unik tenant, e.g. "pt-maju-jaya".
	TenantSlug() string

	// SchemaName mengembalikan nama schema database tenant,
	// e.g. "vfx_pt_maju_jaya".
	SchemaName() string
}

// ═══════════════════════════════════════════════════════════════
//  Internal helpers
// ═══════════════════════════════════════════════════════════════

// openDB membuka koneksi GORM sesuai driver yang dikonfigurasi.
func openDB(cfg config.FangsConfig, gormCfg *gorm.Config) (*gorm.DB, error) {
	dsn := cfg.DSN()
	if dsn == "" {
		return nil, fmt.Errorf("[Fangs] DSN kosong untuk driver '%s'", cfg.Driver)
	}

	var db *gorm.DB
	var err error

	switch cfg.Driver {
	case config.DBDriverPostgres:
		db, err = gorm.Open(postgres.Open(dsn), gormCfg)

	case config.DBDriverMySQL:
		db, err = gorm.Open(mysql.Open(dsn), gormCfg)

	case config.DBDriverSQLServer:
		db, err = gorm.Open(sqlserver.Open(dsn), gormCfg)

	case config.DBDriverSQLite:
		db, err = gorm.Open(sqlite.Open(dsn), gormCfg)

	default:
		return nil, fmt.Errorf(
			"[Fangs] driver '%s' tidak dikenal. Pilihan: postgres, mysql, sqlserver, sqlite",
			cfg.Driver,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("[Fangs] gagal membuka koneksi %s: %w", cfg.Driver, err)
	}

	return db, nil
}

// configurePool mengatur connection pool sesuai config.
func configurePool(db *gorm.DB, cfg config.FangsConfig) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("[Fangs] gagal mendapatkan sql.DB untuk konfigurasi pool: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25 // default
	}

	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5 // default
	}

	connMaxLifetime := cfg.ConnMaxLifetime
	if connMaxLifetime == 0 {
		connMaxLifetime = time.Hour
	}

	connMaxIdleTime := cfg.ConnMaxIdleTime
	if connMaxIdleTime == 0 {
		connMaxIdleTime = 30 * time.Minute
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	return nil
}

// ping melakukan koneksi test ke database.
func ping(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return sqlDB.PingContext(ctx)
}

// ── NamingStrategy helpers ────────────────────────────────────────

// sqlServerNaming adalah GORM NamingStrategy untuk SQL Server.
// Setiap table name otomatis di-prefix dengan [schema].
// e.g. schema="vfx_tenant_a", table="invoices" → "[vfx_tenant_a].[invoices]"
type sqlServerNaming struct {
	schema string
	schemaNamer
}

// TableName override untuk SQL Server — inject [schema].[table].
func (n sqlServerNaming) TableName(table string) string {
	return fmt.Sprintf("[%s].[%s]", n.schema, table)
}

// sqlitePrefixNaming adalah GORM NamingStrategy untuk SQLite.
// SQLite tidak punya schema — table name di-prefix sebagai workaround.
// e.g. prefix="vfx_tenant_a_", table="invoices" → "vfx_tenant_a_invoices"
type sqlitePrefixNaming struct {
	prefix string
	schemaNamer
}

// TableName override untuk SQLite — inject prefix ke table name.
func (n sqlitePrefixNaming) TableName(table string) string {
	return n.prefix + table
}

// ═══════════════════════════════════════════════════════════════
//  GORM Logger — custom logger yang terintegrasi dengan Zap
// ═══════════════════════════════════════════════════════════════

// buildGORMLogger membuat GORM logger yang terintegrasi dengan zap.
func buildGORMLogger(cfg config.FangsConfig, logger *zap.Logger) gormlogger.Interface {
	level := gormlogger.Warn // default: hanya log warning dan error
	if cfg.LogQueries {
		level = gormlogger.Info // log semua query
	}

	slowThreshold := cfg.SlowQueryThreshold
	if slowThreshold == 0 {
		slowThreshold = 200 * time.Millisecond
	}

	return &zapGORMLogger{
		zap:                   logger.Named("fangs"),
		level:                 level,
		slowThreshold:         slowThreshold,
		skipErrRecordNotFound: true,
	}
}

// zapGORMLogger adalah implementasi gormlogger.Interface yang menggunakan zap.
type zapGORMLogger struct {
	zap                   *zap.Logger
	level                 gormlogger.LogLevel
	slowThreshold         time.Duration
	skipErrRecordNotFound bool
}

func (l *zapGORMLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.level = level
	return &newLogger
}

func (l *zapGORMLogger) Info(_ context.Context, msg string, args ...interface{}) {
	if l.level >= gormlogger.Info {
		l.zap.Sugar().Infof(msg, args...)
	}
}

func (l *zapGORMLogger) Warn(_ context.Context, msg string, args ...interface{}) {
	if l.level >= gormlogger.Warn {
		l.zap.Sugar().Warnf(msg, args...)
	}
}

func (l *zapGORMLogger) Error(_ context.Context, msg string, args ...interface{}) {
	if l.level >= gormlogger.Error {
		l.zap.Sugar().Errorf(msg, args...)
	}
}

func (l *zapGORMLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := []zap.Field{
		zap.Duration("elapsed", elapsed),
		zap.Int64("rows", rows),
		zap.String("sql", sql),
	}

	switch {
	case err != nil && !(l.skipErrRecordNotFound && isNotFound(err)):
		// Error query
		l.zap.Error("Query error", append(fields, zap.Error(err))...)

	case elapsed > l.slowThreshold && l.slowThreshold > 0:
		// Slow query — selalu di-log sebagai warning, terlepas dari level config
		l.zap.Warn("🐢 Slow query terdeteksi",
			append(fields, zap.Duration("threshold", l.slowThreshold))...)

	case l.level >= gormlogger.Info:
		// Normal query — hanya di-log jika log_queries: true
		l.zap.Debug("Query", fields...)
	}
}

// isNotFound memeriksa apakah error adalah gorm.ErrRecordNotFound.
// Kita tidak import gorm di sini untuk menghindari circular — cukup cek string.
func isNotFound(err error) bool {
	return err != nil && err.Error() == "record not found"
}
