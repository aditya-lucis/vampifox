// Package den — Sarang utama VampiFox.
package den

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
)

// Version di-inject saat build via ldflags.
var Version = "0.1.0-nightfall"

// ── Module Interface ──────────────────────────────────────────────

// Module adalah kontrak yang harus dipenuhi setiap module VampiFox.
type Module interface {
	Name() string
	Version() string
	DependsOn() []string
	Boot(ctx context.Context, d *Den) error
	Shutdown(ctx context.Context) error
}

// ── Den ───────────────────────────────────────────────────────────

// Den adalah DI container dan lifecycle manager VampiFox.
type Den struct {
	cfg      *VampConfig
	logger   *zap.Logger
	modules  []Module
	modMap   map[string]Module
	services *Services
	server   *http.Server
}

// NewDen membuat instance Den baru.
func NewDen(configPath string) (*Den, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("VampiFox gagal memuat konfigurasi: %w", err)
	}

	logger, err := buildLogger(cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("VampiFox gagal menginisialisasi logger: %w", err)
	}

	d := &Den{
		cfg:    cfg,
		logger: logger,
		modMap: make(map[string]Module),
	}

	logger.Info("[VampiFox] Den terbuka",
		zap.String("version", Version),
		zap.String("env", cfg.App.Env),
	)
	return d, nil
}

// RegisterModules mendaftarkan satu atau lebih module ke Den.
func (d *Den) RegisterModules(modules ...Module) error {
	for _, m := range modules {
		name := m.Name()
		if name == "" {
			return fmt.Errorf("[Den] module dengan Name() kosong tidak bisa didaftarkan")
		}
		if _, exists := d.modMap[name]; exists {
			return fmt.Errorf("[Den] module '%s' sudah terdaftar", name)
		}
		d.modules = append(d.modules, m)
		d.modMap[name] = m
		d.logger.Debug("Module terdaftar",
			zap.String("module", name),
			zap.String("version", m.Version()),
		)
	}
	return nil
}

// Awaken membangunkan VampiFox — init services, boot modules, start server.
func (d *Den) Awaken(ctx context.Context) error {
	d.logger.Info("[VampiFox] Sedang terbangun...",
		zap.String("addr", d.cfg.Server.Addr()),
		zap.Int("modules", len(d.modules)),
	)

	if err := d.validateConfig(); err != nil {
		return err
	}

	if tz := d.cfg.App.Timezone; tz != "" {
		_ = os.Setenv("TZ", tz)
	}

	// Wire semua core services
	svc, err := d.wire()
	if err != nil {
		return fmt.Errorf("[Den] gagal wire services: %w", err)
	}
	d.services = svc

	// Resolve module dependencies
	if err := d.resolveDependencies(); err != nil {
		return err
	}

	// Boot semua module
	for _, m := range d.modules {
		d.logger.Info("Booting module",
			zap.String("module", m.Name()),
			zap.String("version", m.Version()),
		)
		if err := m.Boot(ctx, d); err != nil {
			return fmt.Errorf("[Den] module '%s' gagal boot: %w", m.Name(), err)
		}
	}

	// Setup HTTP server — router dibuat di sini tanpa import rest package
	// Router di-build via builder function yang di-set dari luar
	mux := d.buildRouter()

	d.server = &http.Server{
		Addr:         d.cfg.Server.Addr(),
		Handler:      mux,
		ReadTimeout:  d.cfg.Server.ReadTimeout,
		WriteTimeout: d.cfg.Server.WriteTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		d.logger.Info("[VampiFox] Siap melayani",
			zap.String("addr", d.cfg.Server.Addr()),
		)
		if err := d.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("[Den] HTTP server error: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		d.logger.Info("[VampiFox] Sinyal shutdown diterima...")
	case err := <-serverErr:
		return err
	}

	return d.Slumber()
}

// Slumber graceful shutdown.
func (d *Den) Slumber() error {
	d.logger.Info("[VampiFox] Masuk Slumber...")

	timeout := d.cfg.Server.ShutdownTimeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if d.server != nil {
		if err := d.server.Shutdown(ctx); err != nil {
			d.logger.Warn("HTTP server shutdown tidak bersih", zap.Error(err))
		}
	}

	for i := len(d.modules) - 1; i >= 0; i-- {
		m := d.modules[i]
		if err := m.Shutdown(ctx); err != nil {
			d.logger.Warn("Module shutdown error",
				zap.String("module", m.Name()),
				zap.Error(err),
			)
		}
	}

	if d.services != nil {
		if err := d.services.Fangs.Close(); err != nil {
			d.logger.Warn("Fangs close error", zap.Error(err))
		}
		if err := d.services.Shadow.Close(); err != nil {
			d.logger.Warn("Shadow close error", zap.Error(err))
		}
	}

	d.logger.Info("[VampiFox] Tertidur. Sampai malam berikutnya.")
	_ = d.logger.Sync()
	return nil
}

// ── Accessors ─────────────────────────────────────────────────────

func (d *Den) Config() *VampConfig     { return d.cfg }
func (d *Den) Logger() *zap.Logger     { return d.logger }
func (d *Den) GetServices() *Services  { return d.services }

func (d *Den) Module(name string) (Module, bool) {
	m, ok := d.modMap[name]
	return m, ok
}

// ── Internal helpers ──────────────────────────────────────────────

func (d *Den) validateConfig() error {
	if err := d.cfg.Fangs.Validate(); err != nil {
		return err
	}
	if d.cfg.App.IsProduction() {
		if err := d.cfg.Sanctum.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (d *Den) resolveDependencies() error {
	for _, m := range d.modules {
		for _, dep := range m.DependsOn() {
			if _, ok := d.modMap[dep]; !ok {
				return fmt.Errorf(
					"[Den] module '%s' membutuhkan '%s' yang belum terdaftar.\n"+
						"Tambahkan %s.New() ke RegisterModules() SEBELUM %s.New().",
					m.Name(), dep, dep, m.Name(),
				)
			}
		}
	}
	return nil
}
