package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Cek status semua service VampiFox",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")
		fmt.Printf("[foxctl] Memeriksa health VampiFox di %s...\n\n", addr)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		url := addr + "/health"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("gagal membuat request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("  [!!] VampiFox server tidak merespons di %s\n", addr)
			fmt.Printf("       Error: %v\n", err)
			fmt.Printf("\n  Pastikan server sudah berjalan: .\\scripts\\vfx.ps1 awaken\n")
			return nil
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			fmt.Printf("  [OK] VampiFox server berjalan normal (HTTP %d)\n", resp.StatusCode)
		} else {
			fmt.Printf("  [!!] VampiFox merespons dengan HTTP %d\n", resp.StatusCode)
		}

		return nil
	},
}

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Jalankan VampiFox dalam mode development",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("[foxctl] Memulai mode development...")
		fmt.Println("  Tip: Untuk hot reload, pastikan 'air' sudah terinstall:")
		fmt.Println("       go install github.com/air-verse/air@latest")
		fmt.Println("")
		fmt.Println("  Menjalankan: go run ./cmd/vampifox")
		// TODO: spawn go run atau air jika tersedia
		return nil
	},
}

func init() {
	healthCmd.Flags().String("addr", "http://localhost:8080", "Alamat server VampiFox")
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(devCmd)
}
