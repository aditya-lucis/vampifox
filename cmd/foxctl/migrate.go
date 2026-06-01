package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/aditya-lucis/vampifox/internal/fangs"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Kelola migrasi database",
}

// ── migrate up ────────────────────────────────────────────────────

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Jalankan semua migrasi yang belum diaplikasikan",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := os.Getenv("VAMPIFOX_CONFIG")
		svc, err := load(cfgPath)
		if err != nil {
			return err
		}
		defer svc.close()

		dir, _ := cmd.Flags().GetString("dir")

		fmt.Printf("[foxctl] Menjalankan migrasi dari: %s\n", dir)
		fmt.Printf("  Driver : %s\n", svc.cfg.Fangs.Driver)
		fmt.Printf("  Host   : %s:%d\n\n", svc.cfg.Fangs.Host, svc.cfg.Fangs.Port)

		runner := fangs.NewMigrationRunner(svc.fangs.DB(), svc.logger)

		// Jalankan migration system dulu
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Baca dari OS filesystem
		fsys := os.DirFS(dir)
		count, err := runner.Up(ctx, fsys, ".")
		if err != nil {
			return fmt.Errorf("migrasi gagal: %w", err)
		}

		if count == 0 {
			fmt.Println("[OK] Tidak ada migrasi baru — database sudah up to date.")
		} else {
			fmt.Printf("[OK] %d migrasi berhasil dijalankan.\n", count)
		}
		return nil
	},
}

// ── migrate down ──────────────────────────────────────────────────

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Rollback migrasi terakhir",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Migration down (rollback) butuh down scripts terpisah
		// Ini fitur yang lebih kompleks — untuk sekarang tampilkan instruksi
		fmt.Println("[foxctl] migrate down belum diimplementasikan.")
		fmt.Println("")
		fmt.Println("  Untuk rollback manual:")
		fmt.Println("  1. Lihat tabel _vfx_migrations di database")
		fmt.Println("  2. Jalankan SQL rollback yang sesuai")
		fmt.Println("  3. Hapus baris dari tabel _vfx_migrations")
		fmt.Println("")
		fmt.Println("  Tip: Buat file SQL rollback di migrations/rollback/")
		return nil
	},
}

// ── migrate status ────────────────────────────────────────────────

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Tampilkan status semua migrasi",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := os.Getenv("VAMPIFOX_CONFIG")
		svc, err := load(cfgPath)
		if err != nil {
			return err
		}
		defer svc.close()

		dir, _ := cmd.Flags().GetString("dir")

		runner := fangs.NewMigrationRunner(svc.fangs.DB(), svc.logger)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		fsys := os.DirFS(dir)
		statuses, err := runner.Status(ctx, fsys, ".")
		if err != nil {
			return fmt.Errorf("gagal mengambil status migrasi: %w", err)
		}

		if len(statuses) == 0 {
			fmt.Printf("  Tidak ada file migrasi ditemukan di: %s\n", dir)
			return nil
		}

		fmt.Printf("[foxctl] Status migrasi (%s):\n\n", dir)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "  STATUS\tNAMA FILE\tDIJALANKAN\tDURASI")
		fmt.Fprintln(w, "  ------\t----------\t----------\t-------")

		for _, s := range statuses {
			status := "[PENDING]"
			applied := "-"
			duration := "-"

			if s.Applied {
				status = "[OK]    "
				applied = s.AppliedAt.Format("2006-01-02 15:04:05")
				duration = fmt.Sprintf("%dms", s.DurationMs)
			}

			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", status, s.Name, applied, duration)
		}
		w.Flush()
		fmt.Println()
		return nil
	},
}

// ── migrate create ────────────────────────────────────────────────

var migrateCreateCmd = &cobra.Command{
	Use:   "create <nama>",
	Short: "Buat file migrasi SQL baru dengan timestamp prefix",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		dir, _ := cmd.Flags().GetString("dir")

		// Bersihkan nama: spasi dan karakter aneh jadi underscore
		safeName := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '_' {
				return r
			}
			return '_'
		}, name)
		safeName = strings.ToLower(safeName)

		// Format: 20060102_150405_nama.sql
		timestamp := time.Now().Format("20060102_150405")
		filename := fmt.Sprintf("%s_%s.sql", timestamp, safeName)
		fullPath := filepath.Join(dir, filename)

		// Pastikan direktori ada
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("gagal membuat direktori '%s': %w", dir, err)
		}

		// Buat file SQL dengan template
		content := fmt.Sprintf(`-- Migration: %s
-- Dibuat: %s
-- Deskripsi: %s
--
-- Tulis SQL migration di bawah ini.
-- File ini hanya dijalankan SEKALI dan tidak bisa di-rollback otomatis.

`,
			filename,
			time.Now().Format("2006-01-02 15:04:05"),
			strings.ReplaceAll(safeName, "_", " "),
		)

		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("gagal membuat file '%s': %w", fullPath, err)
		}

		fmt.Printf("[OK] File migrasi dibuat:\n  %s\n\n", fullPath)
		fmt.Println("  Buka file tersebut dan tulis SQL migration kamu.")
		return nil
	},
}

func init() {
	// Flag --dir untuk semua subcommand migrate
	defaultMigrationDir := filepath.Join("migrations", "init")

	migrateUpCmd.Flags().String("dir", defaultMigrationDir, "Direktori file SQL migration")
	migrateStatusCmd.Flags().String("dir", defaultMigrationDir, "Direktori file SQL migration")
	migrateCreateCmd.Flags().String("dir", defaultMigrationDir, "Direktori untuk menyimpan file migration baru")

	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateCreateCmd)
	rootCmd.AddCommand(migrateCmd)
}
