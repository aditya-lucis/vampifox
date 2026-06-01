// Package den — Sarang utama VampiFox.
//
// Den adalah dependency injection container sekaligus lifecycle manager.
// Semua service (database, cache, logger, modules) lahir dan mati di sini.
//
// Alur hidup Den:
//
//	NewDen()  → load config, init logger
//	Awaken()  → init semua service, jalankan HTTP server
//	<running> → terima request, proses, respond
//	Slumber() → graceful shutdown, tutup semua koneksi
package den

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Version VampiFox — di-inject saat build via ldflags.
var Version = "0.1.0-nightfall"

// ═══════════════════════════════════════════════════════════════
//  Module Interface
// ═══════════════════════════════════════════════════════════════

// Module adalah kontrak yang harus dipenuhi setiap module VampiFox.
// Baik module bawaan (accounting, inventory) maupun module custom
// harus mengimplementasikan interface ini.
type Module interface {
	// Name mengembalikan identifier unik module, e.g. "accounting".
	// Digunakan untuk dependency resolution dan logging.
	Name() string

	// Version mengembalikan versi module dalam format semver, e.g. "1.0.0".
	Version() string

	// DependsOn mengembalikan daftar nama module yang harus sudah terdaftar
	// sebelum module ini bisa diinisialisasi.
	// Kembalikan nil atau slice kosong jika tidak ada dependency.
	DependsOn() []string

	// Boot dipanggil saat Den.Awaken() — tempat module mendaftarkan
	// route, listener event, dan resource lainnya.
	// ctx yang diberikan akan di-cancel saat Slumber() dipanggil.
	Boot(ctx context.Context, d *Den) error

	// Shutdown dipanggil saat Den.Slumber() untuk membersihkan resource.
	// Implementasi harus selesai sebelum ctx deadline habis.
	Shutdown(ctx context.Context) error
}

// ═══════════════════════════════════════════════════════════════
//  Den
// ═══════════════════════════════════════════════════════════════

// Den adalah jantung VampiFox.
//
// Jangan membuat Den langsung — gunakan NewDen().
// Jangan menyimpan Den sebagai field di struct module —
// gunakan parameter yang diberikan di Module.Boot().
type Den struct {
	cfg     *VampConfig
	logger  *zap.Logger
	modules []Module          // urutan registrasi dipertahankan
	modMap  map[string]Module // lookup cepat by name
	router  *gin.Engine
	server  *http.Server
	svc     *Services         // core services — di-wire saat Awaken()
}

// ═══════════════════════════════════════════════════════════════
//  Constructor
// ═══════════════════════════════════════════════════════════════

// NewDen membuat instance Den baru dari file konfigurasi.
//
// configPath boleh kosong — Den akan mencari vampifox.yaml di
// ./configs, ., atau /etc/vampifox secara berurutan.
//
// Contoh:
//
//	d, err := den.NewDen("")                           // auto-discover
//	d, err := den.NewDen("configs/vampifox.yaml")      // eksplisit
//	d, err := den.NewDen(os.Getenv("VAMPIFOX_CONFIG")) // dari env
func NewDen(configPath string) (*Den, error) {
	// ── Load config ───────────────────────────────────────────────
	cfg, err := LoadConfig(configPath)
	if err != nil {
		// Config belum ada logger, pakai fmt langsung
		return nil, fmt.Errorf("VampiFox gagal memuat konfigurasi: %w", err)
	}

	// ── Init logger ───────────────────────────────────────────────
	logger, err := buildLogger(cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("VampiFox gagal menginisialisasi logger: %w", err)
	}

	d := &Den{
		cfg:    cfg,
		logger: logger,
		modMap: make(map[string]Module),
	}

	logger.Info("🦊🧛 Den terbuka",
		zap.String("version", Version),
		zap.String("env", cfg.App.Env),
		zap.String("config", configPath),
	)

	return d, nil
}

// ═══════════════════════════════════════════════════════════════
//  Module Registration
// ═══════════════════════════════════════════════════════════════

// RegisterModules mendaftarkan satu atau lebih module ke Den.
//
// Urutan registrasi penting — module yang didaftarkan lebih awal
// akan di-boot lebih awal. Pastikan module dependency didaftarkan
// sebelum module yang bergantung padanya.
//
// Contoh:
//
//	d.RegisterModules(
//	    accounting.New(),
//	    inventory.New(),
//	    myCustomModule.New(),
//	)
func (d *Den) RegisterModules(modules ...Module) error {
	for _, m := range modules {
		name := m.Name()

		if name == "" {
			return fmt.Errorf("[Den] module dengan Name() kosong tidak bisa didaftarkan")
		}

		if _, exists := d.modMap[name]; exists {
			return fmt.Errorf(
				"[Den] module '%s' sudah terdaftar. Setiap module hanya boleh didaftarkan sekali",
				name,
			)
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

// ═══════════════════════════════════════════════════════════════
//  Awaken — Start
// ═══════════════════════════════════════════════════════════════

// Awaken membangunkan VampiFox — menginisialisasi semua service
// dan menjalankan HTTP server.
//
// Awaken bersifat blocking. Ia akan return hanya ketika:
//   - ctx di-cancel (biasanya oleh sinyal OS SIGINT/SIGTERM)
//   - Terjadi error fatal
//
// Setelah ctx di-cancel, Awaken secara otomatis memanggil Slumber()
// untuk graceful shutdown sebelum return.
func (d *Den) Awaken(ctx context.Context) error {
	d.logger.Info("🌙 VampiFox sedang terbangun...",
		zap.String("addr", d.cfg.Server.Addr()),
		zap.Int("modules", len(d.modules)),
	)

	// ── Validasi config kritis ────────────────────────────────────
	if err := d.validateConfig(); err != nil {
		return err
	}

	// ── Timezone ─────────────────────────────────────────────────
	if err := d.setTimezone(); err != nil {
		d.logger.Warn("Gagal set timezone, pakai UTC", zap.Error(err))
	}

	// ── Resolve module dependencies ───────────────────────────────
	if err := d.resolveDependencies(); err != nil {
		return err
	}

	// ── Init Gin router ───────────────────────────────────────────
	// Router diinit sebelum Boot() agar module bisa mendaftarkan
	// route-nya saat Boot() dipanggil via d.Router().
	if d.cfg.App.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	d.router = d.buildRouter()

	// ── Boot semua module ─────────────────────────────────────────
	for _, m := range d.modules {
		d.logger.Info("Booting module",
			zap.String("module", m.Name()),
			zap.String("version", m.Version()),
		)
		if err := m.Boot(ctx, d); err != nil {
			return fmt.Errorf("[Den] module '%s' gagal boot: %w", m.Name(), err)
		}
	}

	// ── HTTP Server ───────────────────────────────────────────────
	d.server = &http.Server{
		Addr:         d.cfg.Server.Addr(),
		ReadTimeout:  d.cfg.Server.ReadTimeout,
		WriteTimeout: d.cfg.Server.WriteTimeout,
		Handler:      d.router, // Gin engine sebagai handler
	}

	// Jalankan server di goroutine terpisah
	serverErr := make(chan error, 1)
	go func() {
		d.logger.Info("🦊 VampiFox siap melayani",
			zap.String("addr", d.cfg.Server.Addr()),
		)
		if err := d.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("[Den] HTTP server error: %w", err)
		}
	}()

	// ── Tunggu sinyal shutdown atau error ─────────────────────────
	select {
	case <-ctx.Done():
		d.logger.Info("🌅 Sinyal shutdown diterima — VampiFox bersiap tidur...")
	case err := <-serverErr:
		d.logger.Error("HTTP server berhenti karena error", zap.Error(err))
		return err
	}

	return d.Slumber()
}

// ═══════════════════════════════════════════════════════════════
//  Slumber — Graceful Shutdown
// ═══════════════════════════════════════════════════════════════

// Slumber mematikan VampiFox secara graceful.
//
// Urutan shutdown (kebalikan dari boot):
//  1. HTTP server berhenti menerima request baru
//  2. Tunggu request yang sedang berjalan selesai
//  3. Shutdown setiap module (urutan terbalik dari registrasi)
func (d *Den) Slumber() error {
	d.logger.Info("😴 VampiFox masuk Slumber...")

	shutdownTimeout := d.cfg.Server.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = 15 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// ── Stop HTTP server ──────────────────────────────────────────
	if d.server != nil {
		if err := d.server.Shutdown(ctx); err != nil {
			d.logger.Warn("HTTP server shutdown tidak bersih", zap.Error(err))
		} else {
			d.logger.Debug("HTTP server berhasil dimatikan")
		}
	}

	// ── Shutdown modules (urutan terbalik) ────────────────────────
	var shutdownErrors []error
	for i := len(d.modules) - 1; i >= 0; i-- {
		m := d.modules[i]
		d.logger.Debug("Shutdown module", zap.String("module", m.Name()))
		if err := m.Shutdown(ctx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("module '%s': %w", m.Name(), err))
			d.logger.Warn("Module shutdown dengan error",
				zap.String("module", m.Name()),
				zap.Error(err),
			)
		}
	}

	// ── Flush logger ──────────────────────────────────────────────
	d.logger.Info("🧛 VampiFox tertidur. Sampai malam berikutnya.")
	_ = d.logger.Sync() // best-effort

	if len(shutdownErrors) > 0 {
		return fmt.Errorf("[Den] %d module gagal shutdown: %v", len(shutdownErrors), shutdownErrors)
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════
//  Accessors — getter untuk dipakai module
// ═══════════════════════════════════════════════════════════════

// Config mengembalikan VampConfig yang sudah di-load.
// Module bisa mengakses config via d.Config() saat Boot().
func (d *Den) Config() *VampConfig { return d.cfg }

// Logger mengembalikan zap.Logger utama VampiFox.
// Module sebaiknya membuat child logger:
//
//	log := d.Logger().Named("accounting")
func (d *Den) Logger() *zap.Logger { return d.logger }

// Router mengembalikan Gin engine VampiFox.
// Module mendaftarkan route-nya di sini saat Boot():
//
//	func (m *AccountingModule) Boot(ctx context.Context, d *den.Den) error {
//	    r := d.Router()
//	    v1 := r.Group("/api/v1/accounting")
//	    v1.GET("/invoices", m.listInvoices)
//	    return nil
//	}
//
// Router hanya tersedia setelah Awaken() dipanggil.
// Jangan panggil Router() di luar Boot() atau handler.
func (d *Den) Router() *gin.Engine { return d.router }

// Module mencari module yang sudah terdaftar berdasarkan nama.
// Berguna untuk module yang ingin berinteraksi dengan module lain.
//
//	invMod, ok := d.Module("inventory")
func (d *Den) Module(name string) (Module, bool) {
	m, ok := d.modMap[name]
	return m, ok
}

// ═══════════════════════════════════════════════════════════════
//  Internal helpers
// ═══════════════════════════════════════════════════════════════

// validateConfig memeriksa konfigurasi kritis sebelum Awaken.
func (d *Den) validateConfig() error {
	// Validasi Fangs
	if err := d.cfg.Fangs.Validate(); err != nil {
		return err
	}

	// Sanctum hanya wajib ketat di production
	if d.cfg.App.IsProduction() {
		if err := d.cfg.Sanctum.Validate(); err != nil {
			return err
		}
	} else if d.cfg.App.IsDevelopment() {
		// Di dev, warn saja jika masih pakai default secret
		if len(d.cfg.Sanctum.AccessSecret) < 8 {
			d.logger.Warn("[Sanctum] access_secret terlalu pendek — OK untuk development, JANGAN di production")
		}
	}

	return nil
}

// setTimezone mengatur timezone process sesuai config.
func (d *Den) setTimezone() error {
	tz := d.cfg.App.Timezone
	if tz == "" {
		return nil
	}
	return os.Setenv("TZ", tz)
}

// resolveDependencies memastikan semua dependency module tersedia.
func (d *Den) resolveDependencies() error {
	for _, m := range d.modules {
		for _, dep := range m.DependsOn() {
			if _, ok := d.modMap[dep]; !ok {
				return fmt.Errorf(
					"[Den] module '%s' membutuhkan module '%s', tapi '%s' belum didaftarkan.\n"+
						"Tambahkan %s.New() ke RegisterModules() SEBELUM %s.New().",
					m.Name(), dep, dep, dep, m.Name(),
				)
			}
		}
	}
	return nil
}
