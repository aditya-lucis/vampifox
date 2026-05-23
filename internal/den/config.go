package den

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ═══════════════════════════════════════════════════════════════
//  VampiFox Config — Strongly-typed configuration structs.
//  Semua nilai dari YAML di-load ke sini via Viper.
//  Tidak ada magic string di luar file ini.
// ═══════════════════════════════════════════════════════════════

// VampConfig adalah root config VampiFox.
// Seluruh konfigurasi aplikasi hidup di sini.
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

// AppConfig konfigurasi umum aplikasi.
type AppConfig struct {
	Name     string `mapstructure:"name"`
	Version  string `mapstructure:"version"`
	Env      string `mapstructure:"env"`      // development | staging | production
	Timezone string `mapstructure:"timezone"`
	Debug    bool   `mapstructure:"debug"`
}

// IsDevelopment returns true jika environment adalah development.
func (a AppConfig) IsDevelopment() bool { return a.Env == "development" }

// IsProduction returns true jika environment adalah production.
func (a AppConfig) IsProduction() bool { return a.Env == "production" }

// ── Server ────────────────────────────────────────────────────────

// ServerConfig konfigurasi HTTP server.
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	CORS            CORSConfig    `mapstructure:"cors"`
}

// Addr mengembalikan alamat lengkap server, e.g. "0.0.0.0:8080".
func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// CORSConfig konfigurasi CORS.
type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowedMethods   []string `mapstructure:"allowed_methods"`
	AllowedHeaders   []string `mapstructure:"allowed_headers"`
	ExposeHeaders    []string `mapstructure:"expose_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	MaxAge           int      `mapstructure:"max_age"`
}

// ── Fangs (Database) ──────────────────────────────────────────────

// DBDriver adalah tipe driver database yang didukung VampiFox.
type DBDriver string

const (
	DBDriverPostgres  DBDriver = "postgres"
	DBDriverMySQL     DBDriver = "mysql"
	DBDriverSQLServer DBDriver = "sqlserver"
	DBDriverSQLite    DBDriver = "sqlite"
)

// FangsConfig konfigurasi koneksi database.
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

// Validate memastikan konfigurasi Fangs valid sebelum digunakan.
func (f FangsConfig) Validate() error {
	switch f.Driver {
	case DBDriverPostgres, DBDriverMySQL, DBDriverSQLServer, DBDriverSQLite:
		// valid
	default:
		return fmt.Errorf(
			"[Fangs] driver '%s' tidak didukung. Pilihan: postgres, mysql, sqlserver, sqlite",
			f.Driver,
		)
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

// DSN membangun Data Source Name sesuai driver yang dipilih.
func (f FangsConfig) DSN() string {
	switch f.Driver {
	case DBDriverPostgres:
		return fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Jakarta",
			f.Host, f.Port, f.User, f.Password, f.DBName, f.SSLMode,
		)
	case DBDriverMySQL:
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			f.User, f.Password, f.Host, f.Port, f.DBName,
		)
	case DBDriverSQLServer:
		return fmt.Sprintf(
			"sqlserver://%s:%s@%s:%d?database=%s",
			f.User, f.Password, f.Host, f.Port, f.DBName,
		)
	case DBDriverSQLite:
		return f.SQLitePath
	default:
		return ""
	}
}

// ── Shadow (Cache) ────────────────────────────────────────────────

// ShadowConfig konfigurasi Redis cache.
type ShadowConfig struct {
	Addr             string        `mapstructure:"addr"`
	Password         string        `mapstructure:"password"`
	DB               int           `mapstructure:"db"`
	DialTimeout      time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout      time.Duration `mapstructure:"read_timeout"`
	WriteTimeout     time.Duration `mapstructure:"write_timeout"`
	PoolSize         int           `mapstructure:"pool_size"`
	DefaultTTL       time.Duration `mapstructure:"default_ttl"`
	SessionTTL       time.Duration `mapstructure:"session_ttl"`
	TenantCacheTTL   time.Duration `mapstructure:"tenant_cache_ttl"`
}

// ── Sanctum (Auth) ────────────────────────────────────────────────

// SanctumConfig konfigurasi JWT dan password hashing.
type SanctumConfig struct {
	AccessSecret  string        `mapstructure:"access_secret"`
	RefreshSecret string        `mapstructure:"refresh_secret"`
	AccessTTL     time.Duration `mapstructure:"access_ttl"`
	RefreshTTL    time.Duration `mapstructure:"refresh_ttl"`
	Issuer        string        `mapstructure:"issuer"`
	BcryptCost    int           `mapstructure:"bcrypt_cost"`
}

// Validate memastikan secret tidak menggunakan nilai default.
func (s SanctumConfig) Validate() error {
	if strings.HasPrefix(s.AccessSecret, "GANTI_INI") {
		return fmt.Errorf("[Sanctum] access_secret masih menggunakan nilai default! Ganti sebelum production")
	}
	if strings.HasPrefix(s.RefreshSecret, "GANTI_INI") {
		return fmt.Errorf("[Sanctum] refresh_secret masih menggunakan nilai default! Ganti sebelum production")
	}
	if len(s.AccessSecret) < 32 {
		return fmt.Errorf("[Sanctum] access_secret minimal 32 karakter untuk keamanan")
	}
	if s.AccessSecret == s.RefreshSecret {
		return fmt.Errorf("[Sanctum] access_secret dan refresh_secret tidak boleh sama")
	}
	return nil
}

// ── Storage ───────────────────────────────────────────────────────

// StorageConfig konfigurasi file storage.
type StorageConfig struct {
	Provider  string `mapstructure:"provider"` // minio | s3 | local
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
	UseSSL    bool   `mapstructure:"use_ssl"`
	Region    string `mapstructure:"region"`
	LocalPath string `mapstructure:"local_path"`
}

// ── Queue ─────────────────────────────────────────────────────────

// QueueConfig konfigurasi NATS JetStream.
type QueueConfig struct {
	URL     string `mapstructure:"url"`
	Stream  string `mapstructure:"stream"`
	Enabled bool   `mapstructure:"enabled"`
}

// ── Mailer ────────────────────────────────────────────────────────

// MailerConfig konfigurasi email.
type MailerConfig struct {
	Provider  string `mapstructure:"provider"` // smtp | sendgrid | ses
	Host      string `mapstructure:"host"`
	Port      int    `mapstructure:"port"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	FromName  string `mapstructure:"from_name"`
	FromEmail string `mapstructure:"from_email"`
	UseTLS    bool   `mapstructure:"use_tls"`
}

// ── Moonphase (Scheduler) ─────────────────────────────────────────

// MoonphaseConfig konfigurasi job scheduler.
type MoonphaseConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Timezone string `mapstructure:"timezone"`
}

// ── Log ───────────────────────────────────────────────────────────

// LogConfig konfigurasi logging.
type LogConfig struct {
	Level       string `mapstructure:"level"`    // debug | info | warn | error
	Format      string `mapstructure:"format"`   // console | json
	Output      string `mapstructure:"output"`   // stdout | file | both
	FilePath    string `mapstructure:"file_path"`
	MaxSizeMB   int    `mapstructure:"max_size_mb"`
	MaxBackups  int    `mapstructure:"max_backups"`
	MaxAgeDays  int    `mapstructure:"max_age_days"`
	Compress    bool   `mapstructure:"compress"`
}

// ── Telemetry ─────────────────────────────────────────────────────

// TelemetryConfig konfigurasi OpenTelemetry.
type TelemetryConfig struct {
	Enabled      bool    `mapstructure:"enabled"`
	OTLPEndpoint string  `mapstructure:"otlp_endpoint"`
	ServiceName  string  `mapstructure:"service_name"`
	Environment  string  `mapstructure:"environment"`
	SampleRate   float64 `mapstructure:"sample_rate"`
}

// ═══════════════════════════════════════════════════════════════
//  Loader — membaca config dari file YAML + env override
// ═══════════════════════════════════════════════════════════════

// LoadConfig membaca file konfigurasi dan env vars lalu
// mengembalikan VampConfig yang sudah terisi.
//
// Prioritas (tertinggi ke terendah):
//  1. Environment variables (prefix: VFX_)
//  2. File YAML yang ditentukan di configPath
//  3. Nilai default bawaan
//
// Contoh env override:
//
//	VFX_FANGS_HOST=db.prod.internal
//	VFX_SANCTUM_ACCESS_SECRET=supersecret
func LoadConfig(configPath string) (*VampConfig, error) {
	v := viper.New()

	// ── Defaults ─────────────────────────────────────────────────
	setDefaults(v)

	// ── File ─────────────────────────────────────────────────────
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Cari di lokasi standar
		v.SetConfigName("vampifox")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/vampifox")
	}

	if err := v.ReadInConfig(); err != nil {
		// Kalau file tidak ada tapi path tidak di-set eksplisit, lanjut pakai defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok || configPath != "" {
			return nil, fmt.Errorf("[Config] gagal membaca file config: %w", err)
		}
	}

	// ── Environment Variables ─────────────────────────────────────
	// Semua env var dengan prefix VFX_ otomatis di-map.
	// Contoh: VFX_FANGS_HOST → fangs.host
	v.SetEnvPrefix("VFX")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// ── Unmarshal ─────────────────────────────────────────────────
	var cfg VampConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("[Config] gagal mem-parse konfigurasi: %w", err)
	}

	return &cfg, nil
}

// setDefaults menetapkan nilai default untuk semua field config.
// Nilai ini akan dipakai jika field tidak ada di YAML atau env var.
func setDefaults(v *viper.Viper) {
	// App
	v.SetDefault("app.name", "VampiFox")
	v.SetDefault("app.version", "0.1.0-nightfall")
	v.SetDefault("app.env", "development")
	v.SetDefault("app.timezone", "Asia/Jakarta")
	v.SetDefault("app.debug", false)

	// Server
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "15s")
	v.SetDefault("server.cors.allowed_origins", []string{"*"})
	v.SetDefault("server.cors.allowed_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	v.SetDefault("server.cors.allowed_headers", []string{"Authorization", "Content-Type", "X-VampiFox-Tenant"})
	v.SetDefault("server.cors.expose_headers", []string{"X-Request-ID"})
	v.SetDefault("server.cors.allow_credentials", true)
	v.SetDefault("server.cors.max_age", 86400)

	// Fangs
	v.SetDefault("fangs.driver", "postgres")
	v.SetDefault("fangs.host", "localhost")
	v.SetDefault("fangs.port", 5432)
	v.SetDefault("fangs.sslmode", "disable")
	v.SetDefault("fangs.max_open_conns", 25)
	v.SetDefault("fangs.max_idle_conns", 5)
	v.SetDefault("fangs.conn_max_lifetime", "1h")
	v.SetDefault("fangs.conn_max_idle_time", "30m")
	v.SetDefault("fangs.sqlite_path", "./vampifox.db")
	v.SetDefault("fangs.log_queries", false)
	v.SetDefault("fangs.slow_query_threshold", "200ms")

	// Shadow
	v.SetDefault("shadow.addr", "localhost:6379")
	v.SetDefault("shadow.db", 0)
	v.SetDefault("shadow.dial_timeout", "5s")
	v.SetDefault("shadow.read_timeout", "3s")
	v.SetDefault("shadow.write_timeout", "3s")
	v.SetDefault("shadow.pool_size", 10)
	v.SetDefault("shadow.default_ttl", "15m")
	v.SetDefault("shadow.session_ttl", "168h")
	v.SetDefault("shadow.tenant_cache_ttl", "5m")

	// Sanctum
	v.SetDefault("sanctum.access_ttl", "15m")
	v.SetDefault("sanctum.refresh_ttl", "168h")
	v.SetDefault("sanctum.issuer", "vampifox")
	v.SetDefault("sanctum.bcrypt_cost", 12)

	// Storage
	v.SetDefault("storage.provider", "local")
	v.SetDefault("storage.use_ssl", false)
	v.SetDefault("storage.region", "ap-southeast-1")
	v.SetDefault("storage.local_path", "./storage")

	// Queue
	v.SetDefault("queue.url", "nats://localhost:4222")
	v.SetDefault("queue.stream", "VAMPIFOX")
	v.SetDefault("queue.enabled", false)

	// Mailer
	v.SetDefault("mailer.provider", "smtp")
	v.SetDefault("mailer.host", "localhost")
	v.SetDefault("mailer.port", 1025)
	v.SetDefault("mailer.from_name", "VampiFox")
	v.SetDefault("mailer.from_email", "noreply@vampifox.io")
	v.SetDefault("mailer.use_tls", false)

	// Moonphase
	v.SetDefault("moonphase.enabled", true)
	v.SetDefault("moonphase.timezone", "Asia/Jakarta")

	// Log
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "console")
	v.SetDefault("log.output", "stdout")
	v.SetDefault("log.file_path", "./logs/vampifox.log")
	v.SetDefault("log.max_size_mb", 100)
	v.SetDefault("log.max_backups", 5)
	v.SetDefault("log.max_age_days", 30)
	v.SetDefault("log.compress", true)

	// Telemetry
	v.SetDefault("telemetry.enabled", false)
	v.SetDefault("telemetry.service_name", "vampifox")
	v.SetDefault("telemetry.environment", "development")
	v.SetDefault("telemetry.sample_rate", 1.0)
}