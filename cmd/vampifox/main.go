// VampiFox — "The Night Never Sleeps, The Fox Never Rests"
//
// Entry point utama aplikasi VampiFox.
// File ini sekecil mungkin — semua logika ada di package den.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aditya-lucis/vampifox/internal/den"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌  %v\n\n", err)
		os.Exit(1)
	}
}

func run() error {
	// ── Config path ───────────────────────────────────────────────
	// Prioritas: flag --config > env VAMPIFOX_CONFIG > auto-discover
	configPath := os.Getenv("VAMPIFOX_CONFIG")
	for i, arg := range os.Args[1:] {
		if arg == "--config" && i+1 < len(os.Args)-1 {
			configPath = os.Args[i+2]
			break
		}
	}

	// ── Buat Den ──────────────────────────────────────────────────
	d, err := den.NewDen(configPath)
	if err != nil {
		return err
	}

	// ── Daftarkan module ──────────────────────────────────────────
	// TODO: daftarkan module saat sudah dibuat
	// Contoh:
	//   if err := d.RegisterModules(
	//       accounting.New(),
	//       inventory.New(),
	//   ); err != nil {
	//       return err
	//   }

	// ── Context dengan graceful shutdown ──────────────────────────
	// Tangkap SIGINT (Ctrl+C) dan SIGTERM (Docker/Kubernetes stop)
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	// ── Awaken! ───────────────────────────────────────────────────
	return d.Awaken(ctx)
}
