package main

import (
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/config"
	"github.com/aditya-lucis/vampifox/internal/core/tenant"
	"github.com/aditya-lucis/vampifox/internal/fangs"
	"github.com/aditya-lucis/vampifox/internal/shadow"
)

// foxServices adalah kumpulan service yang diinisialisasi oleh foxctl.
// Berbeda dari den.Services — tidak ada HTTP server, auth, atau RBAC.
// Hanya infrastruktur yang dibutuhkan CLI.
type foxServices struct {
	cfg        *config.VampConfig
	fangs      *fangs.Fangs
	shadow     *shadow.Shadow
	tenantRepo *tenant.Repository
	tenantSvc  *tenant.Service
	logger     *zap.Logger
}

// loadServices membaca config dan menginisialisasi service yang dibutuhkan foxctl.
// configPath boleh kosong — akan auto-discover vampifox.yaml.
func loadServices(configPath string) (*foxServices, error) {
	// ── Config ────────────────────────────────────────────────────
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("gagal memuat config: %w", err)
	}

	if err := cfg.Fangs.Validate(); err != nil {
		return nil, err
	}

	// Logger sederhana untuk CLI — selalu console, level info
	logCfg := cfg.Log
	logCfg.Format = "console"
	logCfg.Output = "stdout"
	if logCfg.Level == "" {
		logCfg.Level = "info"
	}

	logger, _ := buildCLILogger()

	// ── Fangs ─────────────────────────────────────────────────────
	f, err := fangs.New(cfg.Fangs, logger)
	if err != nil {
		return nil, fmt.Errorf("gagal terhubung ke database: %w\n  Driver: %s\n  Host: %s", err, cfg.Fangs.Driver, cfg.Fangs.Host)
	}

	// ── Shadow ────────────────────────────────────────────────────
	sh, err := shadow.New(cfg.Shadow, logger)
	if err != nil {
		// Shadow (Redis) opsional untuk CLI — lanjut tanpa cache
		logger.Warn("Redis tidak tersedia, cache dinonaktifkan", zap.Error(err))
		sh = nil
	}

	// ── Tenant services ───────────────────────────────────────────
	var tenantRepo *tenant.Repository
	var tenantSvc *tenant.Service

	if sh != nil {
		tenantRepo = tenant.NewRepository(f.DB(), sh, logger)
		tenantSvc = tenant.NewService(tenantRepo, f, logger)
	}

	return &foxServices{
		cfg:        cfg,
		fangs:      f,
		shadow:     sh,
		tenantRepo: tenantRepo,
		tenantSvc:  tenantSvc,
		logger:     logger,
	}, nil
}

// close menutup semua koneksi.
func (s *foxServices) close() {
	if s.fangs != nil {
		_ = s.fangs.Close()
	}
	if s.shadow != nil {
		_ = s.shadow.Close()
	}
}

// buildCLILogger membuat logger sederhana untuk output CLI.
func buildCLILogger() (*zap.Logger, error) {
	return zap.NewDevelopment(zap.WithCaller(false))
}

// configPath membaca --config flag atau env var VAMPIFOX_CONFIG.
func getConfigPath(cmd interface{ Flags() interface{ GetString(string) (string, error) } }) string {
	// Coba dari flag --config
	if p, err := cmd.Flags().GetString("config"); err == nil && p != "" {
		return p
	}
	// Coba dari env var
	if p := os.Getenv("VAMPIFOX_CONFIG"); p != "" {
		return p
	}
	return ""
}
