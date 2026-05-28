// Package config — Konfigurasi VampiFox.
//
// Package ini tidak boleh import package internal VampiFox manapun.
// Semua package lain boleh import config tanpa circular import.
package config

import (
	"fmt"
	"strings"
	"time"
)

// ═══════════════════════════════════════════════════════════════
//  Root config
// ═══════════════════════════════════════════════════════════════

// VampConfig adalah root konfigurasi VampiFox.
type VampConfig struct {
	App       AppConfig       `mapstructure:"app"`
	Server    ServerConfig    `mapstructure:"server"`
	Fangs     FangsConfig     `mapstructure:"fangs"`
	Shadow    ShadowConfig    `mapstructure:"shadow"`
	Sanctum   SanctumConfig   `mapstructure:"sanctum"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Queue     QueueConfig     `mapstructure:"queue"`
	Mailer    MailerConfig    `mapstructure:"mailer"`
	Moonphase MoonphaseConfig `mapstructure:"moonphase"`
	Log       LogConfig       `mapstructure:"log"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
}

// ── App ──────────────────────────────────────────────────────────

type AppConfig struct {
	Name       string `mapstructure:"name"`
	Version    string `mapstructure:"version"`
	Env        string `mapstructure:"env"`
	Timezone   string `mapstructure:"timezone"`
	Debug      bool   `mapstructure:"debug"`
	BaseDomain string `mapstructure:"base_domain"`
}

func (a AppConfig) IsDevelopment() bool { return a.Env == "development" }
func (a AppConfig) IsProduction() bool  { return a.Env == "production" }
func (a AppConfig) BaseDomainStr() string { return a.BaseDomain }

// ── Server ────────────────────────────────────────────────────────

type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	CORS            CORSConfig    `mapstructure:"cors"`
}

func (s ServerConfig) Addr() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowedMethods   []string `mapstructure:"allowed_methods"`
	AllowedHeaders   []string `mapstructure:"allowed_headers"`
	ExposeHeaders    []string `mapstructure:"expose_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	MaxAge           int      `mapstructure:"max_age"`
}

// ── Fangs (Database) ──────────────────────────────────────────────

type DBDriver string

const (
	DBDriverPostgres  DBDriver = "postgres"
	DBDriverMySQL     DBDriver = "mysql"
	DBDriverSQLServer DBDriver = "sqlserver"
	DBDriverSQLite    DBDriver = "sqlite"
)

type FangsConfig struct {
	Driver             DBDriver      `mapstructure:"driver"`
	Host               string        `mapstructure:"host"`
	Port               int           `mapstructure:"port"`
	User               string        `mapstructure:"user"`
	Password           string        `mapstructure:"password"`
	DBName             string        `mapstructure:"dbname"`
	SSLMode            string        `mapstructure:"sslmode"`
	MaxOpenConns       int           `mapstructure:"max_open_conns"`
	MaxIdleConns       int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime    time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime    time.Duration `mapstructure:"conn_max_idle_time"`
	SQLitePath         string        `mapstructure:"sqlite_path"`
	LogQueries         bool          `mapstructure:"log_queries"`
	SlowQueryThreshold time.Duration `mapstructure:"slow_query_threshold"`
}

func (f FangsConfig) Validate() error {
	switch f.Driver {
	case DBDriverPostgres, DBDriverMySQL, DBDriverSQLServer, DBDriverSQLite:
	default:
		return fmt.Errorf("[Fangs] driver '%s' tidak didukung. Pilihan: postgres, mysql, sqlserver, sqlite", f.Driver)
	}
	if f.Driver == DBDriverSQLite {
		if f.SQLitePath == "" {
			return fmt.Errorf("[Fangs] sqlite_path wajib diisi untuk driver sqlite")
		}
		return nil
	}
	if f.Host == "" {
		return fmt.Errorf("[Fangs] host wajib diisi untuk driver %s", f.Driver)
	}
	if f.User == "" {
		return fmt.Errorf("[Fangs] user wajib diisi untuk driver %s", f.Driver)
	}
	if f.DBName == "" {
		return fmt.Errorf("[Fangs] dbname wajib diisi untuk driver %s", f.Driver)
	}
	return nil
}

func (f FangsConfig) DSN() string {
	switch f.Driver {
	case DBDriverPostgres:
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Jakarta",
			f.Host, f.Port, f.User, f.Password, f.DBName, f.SSLMode)
	case DBDriverMySQL:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			f.User, f.Password, f.Host, f.Port, f.DBName)
	case DBDriverSQLServer:
		return fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
			f.User, f.Password, f.Host, f.Port, f.DBName)
	case DBDriverSQLite:
		return f.SQLitePath
	default:
		return ""
	}
}

// ── Shadow (Cache) ────────────────────────────────────────────────

type ShadowConfig struct {
	Addr           string        `mapstructure:"addr"`
	Password       string        `mapstructure:"password"`
	DB             int           `mapstructure:"db"`
	DialTimeout    time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout    time.Duration `mapstructure:"read_timeout"`
	WriteTimeout   time.Duration `mapstructure:"write_timeout"`
	PoolSize       int           `mapstructure:"pool_size"`
	DefaultTTL     time.Duration `mapstructure:"default_ttl"`
	SessionTTL     time.Duration `mapstructure:"session_ttl"`
	TenantCacheTTL time.Duration `mapstructure:"tenant_cache_ttl"`
}

// ── Sanctum (Auth) ────────────────────────────────────────────────

type SanctumConfig struct {
	AccessSecret  string        `mapstructure:"access_secret"`
	RefreshSecret string        `mapstructure:"refresh_secret"`
	AccessTTL     time.Duration `mapstructure:"access_ttl"`
	RefreshTTL    time.Duration `mapstructure:"refresh_ttl"`
	Issuer        string        `mapstructure:"issuer"`
	BcryptCost    int           `mapstructure:"bcrypt_cost"`
}

func (s SanctumConfig) Validate() error {
	if strings.HasPrefix(s.AccessSecret, "GANTI_INI") {
		return fmt.Errorf("[Sanctum] access_secret masih default")
	}
	if strings.HasPrefix(s.RefreshSecret, "GANTI_INI") {
		return fmt.Errorf("[Sanctum] refresh_secret masih default")
	}
	if len(s.AccessSecret) < 32 {
		return fmt.Errorf("[Sanctum] access_secret minimal 32 karakter")
	}
	if s.AccessSecret == s.RefreshSecret {
		return fmt.Errorf("[Sanctum] access_secret dan refresh_secret tidak boleh sama")
	}
	return nil
}

// ── Storage ───────────────────────────────────────────────────────

type StorageConfig struct {
	Provider  string `mapstructure:"provider"`
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
	UseSSL    bool   `mapstructure:"use_ssl"`
	Region    string `mapstructure:"region"`
	LocalPath string `mapstructure:"local_path"`
}

// ── Queue ─────────────────────────────────────────────────────────

type QueueConfig struct {
	URL     string `mapstructure:"url"`
	Stream  string `mapstructure:"stream"`
	Enabled bool   `mapstructure:"enabled"`
}

// ── Mailer ────────────────────────────────────────────────────────

type MailerConfig struct {
	Provider  string `mapstructure:"provider"`
	Host      string `mapstructure:"host"`
	Port      int    `mapstructure:"port"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	FromName  string `mapstructure:"from_name"`
	FromEmail string `mapstructure:"from_email"`
	UseTLS    bool   `mapstructure:"use_tls"`
}

// ── Moonphase ─────────────────────────────────────────────────────

type MoonphaseConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Timezone string `mapstructure:"timezone"`
}

// ── Log ───────────────────────────────────────────────────────────

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Output     string `mapstructure:"output"`
	FilePath   string `mapstructure:"file_path"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
	Compress   bool   `mapstructure:"compress"`
}

// ── Telemetry ─────────────────────────────────────────────────────

type TelemetryConfig struct {
	Enabled      bool    `mapstructure:"enabled"`
	OTLPEndpoint string  `mapstructure:"otlp_endpoint"`
	ServiceName  string  `mapstructure:"service_name"`
	Environment  string  `mapstructure:"environment"`
	SampleRate   float64 `mapstructure:"sample_rate"`
}
