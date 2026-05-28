package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Load membaca file konfigurasi dan env vars.
// Prioritas: env var VFX_* > file YAML > defaults.
func Load(configPath string) (*VampConfig, error) {
	v := viper.New()
	setDefaults(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("vampifox")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/vampifox")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok || configPath != "" {
			return nil, fmt.Errorf("[Config] gagal membaca file config: %w", err)
		}
	}

	v.SetEnvPrefix("VFX")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg VampConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("[Config] gagal mem-parse konfigurasi: %w", err)
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "VampiFox")
	v.SetDefault("app.version", "0.1.0-nightfall")
	v.SetDefault("app.env", "development")
	v.SetDefault("app.timezone", "Asia/Jakarta")
	v.SetDefault("app.debug", false)
	v.SetDefault("app.base_domain", "")

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

	v.SetDefault("shadow.addr", "localhost:6379")
	v.SetDefault("shadow.db", 0)
	v.SetDefault("shadow.dial_timeout", "5s")
	v.SetDefault("shadow.read_timeout", "3s")
	v.SetDefault("shadow.write_timeout", "3s")
	v.SetDefault("shadow.pool_size", 10)
	v.SetDefault("shadow.default_ttl", "15m")
	v.SetDefault("shadow.session_ttl", "168h")
	v.SetDefault("shadow.tenant_cache_ttl", "5m")

	v.SetDefault("sanctum.access_ttl", "15m")
	v.SetDefault("sanctum.refresh_ttl", "168h")
	v.SetDefault("sanctum.issuer", "vampifox")
	v.SetDefault("sanctum.bcrypt_cost", 12)

	v.SetDefault("storage.provider", "local")
	v.SetDefault("storage.use_ssl", false)
	v.SetDefault("storage.local_path", "./storage")

	v.SetDefault("queue.url", "nats://localhost:4222")
	v.SetDefault("queue.stream", "VAMPIFOX")
	v.SetDefault("queue.enabled", false)

	v.SetDefault("mailer.provider", "smtp")
	v.SetDefault("mailer.host", "localhost")
	v.SetDefault("mailer.port", 1025)
	v.SetDefault("mailer.from_name", "VampiFox")
	v.SetDefault("mailer.from_email", "noreply@vampifox.io")
	v.SetDefault("mailer.use_tls", false)

	v.SetDefault("moonphase.enabled", true)
	v.SetDefault("moonphase.timezone", "Asia/Jakarta")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "console")
	v.SetDefault("log.output", "stdout")
	v.SetDefault("log.file_path", "./logs/vampifox.log")
	v.SetDefault("log.max_size_mb", 100)
	v.SetDefault("log.max_backups", 5)
	v.SetDefault("log.max_age_days", 30)
	v.SetDefault("log.compress", true)

	v.SetDefault("telemetry.enabled", false)
	v.SetDefault("telemetry.service_name", "vampifox")
	v.SetDefault("telemetry.environment", "development")
	v.SetDefault("telemetry.sample_rate", 1.0)
}
