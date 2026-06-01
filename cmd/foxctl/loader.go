package main

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/aditya-lucis/vampifox/internal/config"
	"github.com/aditya-lucis/vampifox/internal/core/tenant"
	"github.com/aditya-lucis/vampifox/internal/fangs"
	"github.com/aditya-lucis/vampifox/internal/shadow"
)

// foxServices adalah service yang dibutuhkan foxctl.
// Lebih ringan dari den.Services — tidak ada HTTP server, auth, RBAC.
type foxServices struct {
	cfg       *config.VampConfig
	fangs     *fangs.Fangs
	shadow    *shadow.Shadow
	tenantSvc *tenant.Service
	logger    *zap.Logger
}

// load membaca config dan menginisialisasi service untuk foxctl.
func load(configPath string) (*foxServices, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("gagal memuat config: %w\n  Pastikan vampifox.yaml ada di ./configs/ atau set VAMPIFOX_CONFIG", err)
	}

	if err := cfg.Fangs.Validate(); err != nil {
		return nil, err
	}

	logger := buildCLILogger()

	// Fangs (wajib)
	f, err := fangs.New(cfg.Fangs, logger)
	if err != nil {
		return nil, fmt.Errorf("gagal terhubung ke database:\n  Driver : %s\n  Host   : %s:%d\n  Error  : %w",
			cfg.Fangs.Driver, cfg.Fangs.Host, cfg.Fangs.Port, err)
	}

	// Shadow (opsional — lanjut tanpa cache jika Redis tidak tersedia)
	sh, err := shadow.New(cfg.Shadow, logger)
	if err != nil {
		logger.Warn("Redis tidak tersedia, beberapa fitur mungkin lambat",
			zap.String("addr", cfg.Shadow.Addr),
		)
		sh = nil
	}

	var tenantSvc *tenant.Service
	if sh != nil {
		repo := tenant.NewRepository(f.DB(), sh, logger)
		tenantSvc = tenant.NewService(repo, f, logger)
	}

	return &foxServices{
		cfg:       cfg,
		fangs:     f,
		shadow:    sh,
		tenantSvc: tenantSvc,
		logger:    logger,
	}, nil
}

// close menutup semua koneksi dengan rapi.
func (s *foxServices) close() {
	if s.fangs != nil {
		_ = s.fangs.Close()
	}
	if s.shadow != nil {
		_ = s.shadow.Close()
	}
}

// buildCLILogger membuat logger yang ramah untuk output terminal.
func buildCLILogger() *zap.Logger {
	cfg := zap.NewDevelopmentEncoderConfig()
	cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.TimeKey = ""     // hapus timestamp — tidak perlu di CLI
	cfg.CallerKey = ""   // hapus caller

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(cfg),
		zapcore.AddSync(os.Stdout),
		zapcore.InfoLevel,
	)
	return zap.New(core)
}


