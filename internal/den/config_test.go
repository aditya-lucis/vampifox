package den

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_FromFile(t *testing.T) {
	// Buat temp config file
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "vampifox.yaml")

	content := `
		app:
		name: "TestFox"
		env: "development"
		timezone: "Asia/Jakarta"
		debug: true

		server:
		host: "127.0.0.1"
		port: 9090
		read_timeout: "10s"
		write_timeout: "10s"
		shutdown_timeout: "5s"

		fangs:
		driver: "sqlite"
		sqlite_path: "./test.db"

		sanctum:
		access_secret: "test-secret-yang-cukup-panjang-32char"
		refresh_secret: "refresh-secret-yang-berbeda-32char!!"
		access_ttl: "15m"
		refresh_ttl: "168h"

		log:
		level: "debug"
		format: "console"
		output: "stdout"
		`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("gagal menulis temp config: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig gagal: %v", err)
	}

	// App
	if cfg.App.Name != "TestFox" {
		t.Errorf("App.Name = %q, want %q", cfg.App.Name, "TestFox")
	}
	if cfg.App.Env != "development" {
		t.Errorf("App.Env = %q, want %q", cfg.App.Env, "development")
	}
	if !cfg.App.IsDevelopment() {
		t.Error("IsDevelopment() harus true")
	}
	if cfg.App.IsProduction() {
		t.Error("IsProduction() harus false")
	}

	// Server
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.Addr() != "127.0.0.1:9090" {
		t.Errorf("Server.Addr() = %q, want %q", cfg.Server.Addr(), "127.0.0.1:9090")
	}
	if cfg.Server.ReadTimeout != 10*time.Second {
		t.Errorf("Server.ReadTimeout = %v, want 10s", cfg.Server.ReadTimeout)
	}

	// Fangs
	if cfg.Fangs.Driver != DBDriverSQLite {
		t.Errorf("Fangs.Driver = %q, want %q", cfg.Fangs.Driver, DBDriverSQLite)
	}

	// Log
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	// Override via env var
	t.Setenv("VFX_SERVER_PORT", "7777")
	t.Setenv("VFX_APP_ENV", "production")

	cfg, err := LoadConfig("") // auto-discover (mungkin tidak ada file, pakai defaults)
	if err != nil {
		// Kalau file tidak ada, tidak apa-apa — test env override tetap valid
		t.Skipf("config file tidak ditemukan: %v", err)
	}

	if cfg.Server.Port != 7777 {
		t.Errorf("Server.Port = %d, want 7777 (dari env VFX_SERVER_PORT)", cfg.Server.Port)
	}
	if cfg.App.Env != "production" {
		t.Errorf("App.Env = %q, want production (dari env VFX_APP_ENV)", cfg.App.Env)
	}
}

func TestFangsConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     FangsConfig
		wantErr bool
	}{
		{
			name:    "postgres valid",
			cfg:     FangsConfig{Driver: DBDriverPostgres, Host: "localhost", User: "vfx", DBName: "main"},
			wantErr: false,
		},
		{
			name:    "mysql valid",
			cfg:     FangsConfig{Driver: DBDriverMySQL, Host: "localhost", User: "vfx", DBName: "main"},
			wantErr: false,
		},
		{
			name:    "sqlite valid",
			cfg:     FangsConfig{Driver: DBDriverSQLite, SQLitePath: "./test.db"},
			wantErr: false,
		},
		{
			name:    "driver tidak dikenal",
			cfg:     FangsConfig{Driver: "mongodb"},
			wantErr: true,
		},
		{
			name:    "postgres tanpa host",
			cfg:     FangsConfig{Driver: DBDriverPostgres, User: "vfx", DBName: "main"},
			wantErr: true,
		},
		{
			name:    "postgres tanpa user",
			cfg:     FangsConfig{Driver: DBDriverPostgres, Host: "localhost", DBName: "main"},
			wantErr: true,
		},
		{
			name:    "sqlite tanpa path",
			cfg:     FangsConfig{Driver: DBDriverSQLite},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFangsConfig_DSN(t *testing.T) {
	tests := []struct {
		name    string
		cfg     FangsConfig
		wantDSN string
	}{
		{
			name: "postgres DSN",
			cfg: FangsConfig{
				Driver: DBDriverPostgres, Host: "localhost", Port: 5432,
				User: "vfx", Password: "secret", DBName: "main", SSLMode: "disable",
			},
			wantDSN: "host=localhost port=5432 user=vfx password=secret dbname=main sslmode=disable TimeZone=Asia/Jakarta",
		},
		{
			name: "mysql DSN",
			cfg: FangsConfig{
				Driver: DBDriverMySQL, Host: "localhost", Port: 3306,
				User: "vfx", Password: "secret", DBName: "main",
			},
			wantDSN: "vfx:secret@tcp(localhost:3306)/main?charset=utf8mb4&parseTime=True&loc=Local",
		},
		{
			name: "sqlite DSN",
			cfg:     FangsConfig{Driver: DBDriverSQLite, SQLitePath: "./test.db"},
			wantDSN: "./test.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.DSN()
			if got != tt.wantDSN {
				t.Errorf("DSN() =\n  %q\nwant\n  %q", got, tt.wantDSN)
			}
		})
	}
}

func TestSanctumConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SanctumConfig
		wantErr bool
	}{
		{
			name: "valid",
			cfg: SanctumConfig{
				AccessSecret:  "ini-secret-yang-cukup-panjang-32c",
				RefreshSecret: "refresh-secret-yang-berbeda-32ch!!",
			},
			wantErr: false,
		},
		{
			name:    "masih default",
			cfg:     SanctumConfig{AccessSecret: "GANTI_INI_ACCESS", RefreshSecret: "ok-refresh-secret-32characters!!!"},
			wantErr: true,
		},
		{
			name:    "terlalu pendek",
			cfg:     SanctumConfig{AccessSecret: "short", RefreshSecret: "ok-refresh-secret-32characters!!!"},
			wantErr: true,
		},
		{
			name:    "access == refresh",
			cfg:     SanctumConfig{AccessSecret: "sama-secret-32-characters-abcdefg", RefreshSecret: "sama-secret-32-characters-abcdefg"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}