// Package fangs — Database layer VampiFox.
package fangs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"

	"github.com/aditya-lucis/vampifox/internal/config"
)

// Fangs mengelola koneksi database VampiFox.
type Fangs struct {
	db     *gorm.DB
	cfg    config.FangsConfig
	logger *zap.Logger
}

// New membuka koneksi ke database.
func New(cfg config.FangsConfig, logger *zap.Logger) (*Fangs, error) {
	if logger == nil {
		return nil, fmt.Errorf("[Fangs] logger tidak boleh nil")
	}

	gormCfg := &gorm.Config{
		PrepareStmt:                              cfg.Driver != config.DBDriverSQLite,
		DisableForeignKeyConstraintWhenMigrating: false,
		Logger: buildGORMLogger(cfg, logger),
	}

	db, err := openDB(cfg, gormCfg)
	if err != nil {
		return nil, err
	}

	if err := configurePool(db, cfg); err != nil {
		return nil, err
	}

	if err := ping(db); err != nil {
		return nil, fmt.Errorf("[Fangs] database tidak merespons (%s@%s/%s): %w",
			cfg.User, cfg.Host, cfg.DBName, err)
	}

	logger.Info("Fangs terhubung",
		zap.String("driver", string(cfg.Driver)),
		zap.String("host", cfg.Host),
		zap.String("dbname", cfg.DBName),
	)

	return &Fangs{db: db, cfg: cfg, logger: logger}, nil
}

func (f *Fangs) DB() *gorm.DB { return f.db }

func (f *Fangs) For(tenant TenantScope) *gorm.DB {
	schemaName := tenant.SchemaName()
	if schemaName == "" {
		return f.db
	}
	return f.scopeDB(schemaName)
}

func (f *Fangs) scopeDB(schemaName string) *gorm.DB {
	switch f.cfg.Driver {
	case config.DBDriverPostgres:
		return f.db.Exec(fmt.Sprintf("SET search_path TO %s, public", schemaName))
	case config.DBDriverMySQL:
		return f.db.Exec(fmt.Sprintf("USE %s", schemaName))
	case config.DBDriverSQLServer:
		return f.db.Scopes(func(db *gorm.DB) *gorm.DB {
			return db.Session(&gorm.Session{NewDB: true})
		})
	default:
		return f.db
	}
}

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
		query = fmt.Sprintf(`IF NOT EXISTS (SELECT 1 FROM sys.schemas WHERE name = '%s') BEGIN EXEC('CREATE SCHEMA [%s]') END`, schemaName, schemaName)
	case config.DBDriverSQLite:
		return nil
	}
	if err := f.db.WithContext(ctx).Exec(query).Error; err != nil {
		return fmt.Errorf("[Fangs] gagal membuat schema '%s': %w", schemaName, err)
	}
	f.logger.Info("Schema tenant dibuat", zap.String("schema", schemaName))
	return nil
}

func (f *Fangs) DropTenantSchema(ctx context.Context, schemaName string) error {
	if schemaName == "" {
		return fmt.Errorf("[Fangs] schema name tidak boleh kosong")
	}
	for _, reserved := range []string{"public", "vfx_system", "dbo"} {
		if schemaName == reserved {
			return fmt.Errorf("[Fangs] menghapus schema sistem '%s' tidak diizinkan", schemaName)
		}
	}
	var query string
	switch f.cfg.Driver {
	case config.DBDriverPostgres:
		query = fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName)
	case config.DBDriverMySQL:
		query = fmt.Sprintf("DROP DATABASE IF EXISTS %s", schemaName)
	case config.DBDriverSQLite:
		return nil
	default:
		return fmt.Errorf("[Fangs] DropSchema tidak didukung untuk driver %s", f.cfg.Driver)
	}
	if err := f.db.WithContext(ctx).Exec(query).Error; err != nil {
		return fmt.Errorf("[Fangs] gagal drop schema '%s': %w", schemaName, err)
	}
	f.logger.Warn("Schema tenant dihapus", zap.String("schema", schemaName))
	return nil
}

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
	case config.DBDriverSQLite:
		return true, nil
	default:
		return false, fmt.Errorf("[Fangs] TenantSchemaExists tidak didukung untuk driver %s", f.cfg.Driver)
	}
	if err := f.db.WithContext(ctx).Raw(query, args...).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (f *Fangs) Ping(ctx context.Context) error {
	sqlDB, err := f.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (f *Fangs) Stats() (sql.DBStats, error) {
	sqlDB, err := f.db.DB()
	if err != nil {
		return sql.DBStats{}, err
	}
	return sqlDB.Stats(), nil
}

func (f *Fangs) Close() error {
	sqlDB, err := f.db.DB()
	if err != nil {
		return err
	}
	if err := sqlDB.Close(); err != nil {
		return err
	}
	f.logger.Info("Fangs ditutup")
	return nil
}

// TenantScope interface untuk fangs.For()
type TenantScope interface {
	Slug() string
	SchemaName() string
}

// Internal helpers

func openDB(cfg config.FangsConfig, gormCfg *gorm.Config) (*gorm.DB, error) {
	dsn := cfg.DSN()
	switch cfg.Driver {
	case config.DBDriverPostgres:
		return gorm.Open(postgres.Open(dsn), gormCfg)
	case config.DBDriverMySQL:
		return gorm.Open(mysql.Open(dsn), gormCfg)
	case config.DBDriverSQLServer:
		return gorm.Open(sqlserver.Open(dsn), gormCfg)
	case config.DBDriverSQLite:
		return gorm.Open(sqlite.Open(dsn), gormCfg)
	default:
		return nil, fmt.Errorf("[Fangs] driver '%s' tidak dikenal", cfg.Driver)
	}
}

func configurePool(db *gorm.DB, cfg config.FangsConfig) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
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

func ping(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return sqlDB.PingContext(ctx)
}

func buildGORMLogger(cfg config.FangsConfig, logger *zap.Logger) gormlogger.Interface {
	level := gormlogger.Warn
	if cfg.LogQueries {
		level = gormlogger.Info
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

type zapGORMLogger struct {
	zap                   *zap.Logger
	level                 gormlogger.LogLevel
	slowThreshold         time.Duration
	skipErrRecordNotFound bool
}

func (l *zapGORMLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	n := *l; n.level = level; return &n
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
func (l *zapGORMLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := []zap.Field{zap.Duration("elapsed", elapsed), zap.Int64("rows", rows), zap.String("sql", sql)}
	switch {
	case err != nil && !(l.skipErrRecordNotFound && isNotFound(err)):
		l.zap.Error("Query error", append(fields, zap.Error(err))...)
	case elapsed > l.slowThreshold && l.slowThreshold > 0:
		l.zap.Warn("Slow query", append(fields, zap.Duration("threshold", l.slowThreshold))...)
	case l.level >= gormlogger.Info:
		l.zap.Debug("Query", fields...)
	}
}
func isNotFound(err error) bool {
	return err != nil && err.Error() == "record not found"
}
