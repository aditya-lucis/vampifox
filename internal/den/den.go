// Package den — Sarang utama VampiFox.
// Den adalah dependency injection container dan lifecycle manager.
// Layaknya sarang vampire, semua "makhluk" (service) lahir dan hidup di sini.
package den

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// Den adalah jantung VampiFox. Ia mengelola semua service dan lifecycle-nya.
type Den struct {
	cfg    Config
	logger *zap.Logger
	// services diisi oleh Awaken()
}

// Config konfigurasi bootstrap Den.
type Config struct {
	ConfigPath string
}

// NewDen membuat instance Den baru — "membuka peti mati".
func NewDen(cfg Config) (*Den, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("logger gagal diinisialisasi: %w", err)
	}

	return &Den{
		cfg:    cfg,
		logger: logger,
	}, nil
}

// Awaken membangunkan semua service VampiFox.
// Dipanggil saat matahari terbenam — server mulai berjalan.
func (d *Den) Awaken(ctx context.Context) error {
	d.logger.Info("🦊🧛 VampiFox sedang terbangun...",
		zap.String("version", "0.1.0-nightfall"),
	)

	// TODO: inisialisasi Database, Cache, Queue, Router, Modules
	// Urutan: Fangs (DB) → Shadow (Cache) → Modules → HTTP Server

	<-ctx.Done()

	d.logger.Info("🌅 Fajar tiba — VampiFox kembali ke Den.")
	return d.Slumber()
}

// Slumber graceful shutdown — "kembali ke peti mati saat fajar".
func (d *Den) Slumber() error {
	d.logger.Info("VampiFox masuk mode Slumber, menutup semua koneksi...")
	// TODO: tutup DB, cache, queue, flush logs
	return nil
}
