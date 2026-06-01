package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

// ── health ────────────────────────────────────────────────────────

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Cek status semua service VampiFox",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverAddr, _ := cmd.Flags().GetString("addr")
		skipDB, _ := cmd.Flags().GetBool("skip-db")

		fmt.Println("[foxctl] Memeriksa status service...\n")

		// ── 1. HTTP Server ─────────────────────────────────────────
		fmt.Print("  HTTP Server  ")
		checkHTTP(serverAddr)

		if skipDB {
			return nil
		}

		// ── 2. Database & Cache ────────────────────────────────────
		cfgPath := os.Getenv("VAMPIFOX_CONFIG")
		svc, err := load(cfgPath)
		if err != nil {
			fmt.Printf("  Config       [!!] %v\n", err)
			return nil
		}
		defer svc.close()

		// Database
		fmt.Print("  Database     ")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := svc.fangs.Ping(ctx); err != nil {
			fmt.Printf("[!!] Tidak merespons — %v\n", err)
		} else {
			stats, _ := svc.fangs.Stats()
			fmt.Printf("[OK] %s — open:%d idle:%d\n",
				svc.cfg.Fangs.Driver,
				stats.OpenConnections,
				stats.Idle,
			)
		}

		// Redis / Shadow
		fmt.Print("  Redis        ")
		if svc.shadow == nil {
			fmt.Println("[!!] Tidak terhubung")
		} else {
			if err := svc.shadow.Ping(ctx); err != nil {
				fmt.Printf("[!!] Tidak merespons — %v\n", err)
			} else {
				stats := svc.shadow.Stats()
				fmt.Printf("[OK] %s — pool:%d/%d\n",
					svc.cfg.Shadow.Addr,
					stats.TotalConns,
					int32(svc.cfg.Shadow.PoolSize),
				)
			}
		}

		fmt.Println()
		return nil
	},
}

func checkHTTP(addr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr+"/health", nil)
	if err != nil {
		fmt.Printf("[!!] Gagal membuat request\n")
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[!!] Tidak merespons di %s\n", addr)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("[OK] %s (HTTP %d)\n", addr, resp.StatusCode)
	} else {
		fmt.Printf("[!!] HTTP %d\n", resp.StatusCode)
	}
}

// ── dev ───────────────────────────────────────────────────────────

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Jalankan VampiFox dalam mode development",
	Long: `Jalankan VampiFox server dalam mode development.

Jika 'air' terinstall, akan menggunakan hot reload otomatis.
Install air: go install github.com/air-verse/air@latest`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Cek apakah air tersedia (hot reload)
		if _, err := exec.LookPath("air"); err == nil {
			fmt.Println("[foxctl] Menjalankan dengan Air (hot reload)...")
			fmt.Println("  Tekan Ctrl+C untuk menghentikan.\n")

			c := exec.Command("air")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Env = append(os.Environ(),
				"VAMPIFOX_CONFIG="+os.Getenv("VAMPIFOX_CONFIG"),
			)
			return c.Run()
		}

		// Fallback: go run
		fmt.Println("[foxctl] Air tidak ditemukan, menjalankan dengan go run...")
		fmt.Println("  Tip: Install air untuk hot reload:")
		fmt.Println("       go install github.com/air-verse/air@latest")
		fmt.Println("  Tekan Ctrl+C untuk menghentikan.\n")

		cfgPath := os.Getenv("VAMPIFOX_CONFIG")
		if cfgPath == "" {
			cfgPath = "configs/vampifox.yaml"
		}

		c := exec.Command("go", "run", "./cmd/vampifox")
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Env = append(os.Environ(), "VAMPIFOX_CONFIG="+cfgPath)
		return c.Run()
	},
}

func init() {
	healthCmd.Flags().String("addr", "http://localhost:8080", "Alamat HTTP server VampiFox")
	healthCmd.Flags().Bool("skip-db", false, "Skip pemeriksaan database dan Redis")

	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(devCmd)
}
