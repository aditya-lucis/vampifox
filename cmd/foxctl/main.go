// foxctl — CLI developer tool VampiFox.
//
// Usage:
//
//	foxctl migrate up                  — jalankan migrasi baru
//	foxctl migrate down                — rollback migrasi terakhir
//	foxctl migrate status              — status semua migrasi
//	foxctl migrate create <name>       — buat file migrasi baru
//	foxctl tenant create --name --slug — buat tenant baru
//	foxctl tenant list                 — list semua tenant
//	foxctl tenant suspend <slug>       — suspend tenant
//	foxctl tenant unsuspend <slug>     — aktifkan kembali tenant
//	foxctl health                      — cek koneksi semua service
//	foxctl dev                         — jalankan server development
//	foxctl version                     — tampilkan versi
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version di-inject saat build via ldflags.
var Version = "0.1.0-nightfall"

var rootCmd = &cobra.Command{
	Use:   "foxctl",
	Short: "foxctl — CLI tool untuk VampiFox developer",
	Long: `
  [VampiFox] foxctl — The Fox's Toolkit
  ----------------------------------------
  Semua yang kamu butuhkan untuk membangun kerajaan VampiFox.

  Config: set VAMPIFOX_CONFIG atau taruh vampifox.yaml di ./configs/`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Tampilkan versi foxctl",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("foxctl %s\n", Version)
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
