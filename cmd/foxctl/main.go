// foxctl — CLI developer tool VampiFox.
//
// "The fox's toolkit" — semua yang developer butuhkan
// untuk membangun, mengelola, dan men-debug aplikasi VampiFox.
//
// Usage:
//
//	foxctl dev              — jalankan server development dengan hot reload
//	foxctl migrate up       — jalankan semua migration yang belum diaplikasikan
//	foxctl migrate down     — rollback migration terakhir
//	foxctl migrate status   — tampilkan status semua migration
//	foxctl tenant create    — buat tenant baru
//	foxctl tenant list      — tampilkan semua tenant
//	foxctl module scaffold  — generate boilerplate module baru
//	foxctl health           — cek status semua service
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "foxctl",
	Short: "foxctl — CLI tool untuk VampiFox developer",
	Long: `
  ███████╗ ██████╗ ██╗  ██╗ ██████╗████████╗██╗
  ██╔════╝██╔═══██╗╚██╗██╔╝██╔════╝╚══██╔══╝██║
  █████╗  ██║   ██║ ╚███╔╝ ██║        ██║   ██║
  ██╔══╝  ██║   ██║ ██╔██╗ ██║        ██║   ██║
  ██║     ╚██████╔╝██╔╝ ██╗╚██████╗   ██║   ███████╗
  ╚═╝      ╚═════╝ ╚═╝  ╚═╝ ╚═════╝   ╚═╝   ╚══════╝

  VampiFox Developer CLI Tool
  "The fox's toolkit — everything you need to build the kingdom"`,
	SilenceUsage: true,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
