// VampiFox ERP — "The Night Never Sleeps, The Fox Never Rests"
// Entry point utama aplikasi VampiFox
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"vampifox/internal/den"
)

func main() {
	// Den = sarang VampiFox, tempat semua layanan diinisialisasi
	app, err := den.NewDen(den.Config{
		ConfigPath: os.Getenv("VAMPIFOX_CONFIG"),
	})
	if err != nil {
		log.Fatalf("[VampiFox] Gagal membuka Den: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Awaken(ctx); err != nil {
		log.Fatalf("[VampiFox] Den tidak bisa terbangun: %v", err)
	}
}
