package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	coretenant "github.com/aditya-lucis/vampifox/internal/core/tenant"
)

var tenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Kelola tenant VampiFox",
}

// ── tenant create ─────────────────────────────────────────────────

var tenantCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Provision tenant baru beserta schema database-nya",
	Example: `  foxctl tenant create --name "PT Maju Jaya" --slug pt-maju-jaya
  foxctl tenant create --name "RS Sehat" --slug rs-sehat --plan growth`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		slug, _ := cmd.Flags().GetString("slug")
		plan, _ := cmd.Flags().GetString("plan")
		domain, _ := cmd.Flags().GetString("domain")

		if name == "" || slug == "" {
			return fmt.Errorf("--name dan --slug wajib diisi\n\nContoh:\n  foxctl tenant create --name \"PT Maju Jaya\" --slug pt-maju-jaya")
		}

		// Validasi slug sebelum load services
		if err := coretenant.ValidateSlug(slug); err != nil {
			return fmt.Errorf("slug tidak valid: %w", err)
		}

		cfgPath := os.Getenv("VAMPIFOX_CONFIG")
		svc, err := load(cfgPath)
		if err != nil {
			return err
		}
		defer svc.close()

		if svc.tenantSvc == nil {
			return fmt.Errorf("tenant service tidak tersedia — pastikan Redis berjalan")
		}

		fmt.Printf("[foxctl] Provisioning tenant baru...\n")
		fmt.Printf("  Nama   : %s\n", name)
		fmt.Printf("  Slug   : %s\n", slug)
		fmt.Printf("  Plan   : %s\n", plan)
		fmt.Printf("  Schema : %s\n", coretenant.SchemaNameFor(slug))
		if domain != "" {
			fmt.Printf("  Domain : %s\n", domain)
		}
		fmt.Println()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		input := coretenant.CreateInput{
			Name:   name,
			Slug:   slug,
			Plan:   coretenant.Plan(plan),
			Domain: domain,
		}

		tenant, err := svc.tenantSvc.Provision(ctx, input)
		if err != nil {
			switch err {
			case coretenant.ErrSlugTaken:
				return fmt.Errorf("slug '%s' sudah digunakan tenant lain", slug)
			case coretenant.ErrInvalidSlug:
				return fmt.Errorf("format slug tidak valid: hanya huruf kecil, angka, dan tanda hubung")
			default:
				return fmt.Errorf("gagal membuat tenant: %w", err)
			}
		}

		fmt.Println("[OK] Tenant berhasil dibuat!")
		fmt.Printf("  ID     : %s\n", tenant.ID.String())
		fmt.Printf("  Slug   : %s\n", tenant.Slug)
		fmt.Printf("  Schema : %s\n", tenant.SchemaName)
		fmt.Printf("  Status : %s\n", tenant.Status)
		fmt.Println()
		fmt.Println("  Langkah selanjutnya:")
		fmt.Printf("  1. Jalankan migrasi: foxctl migrate up\n")
		fmt.Printf("  2. Buat user pertama via API: POST /api/v1/auth/register\n")
		return nil
	},
}

// ── tenant list ───────────────────────────────────────────────────

var tenantListCmd = &cobra.Command{
	Use:   "list",
	Short: "Tampilkan semua tenant",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := os.Getenv("VAMPIFOX_CONFIG")
		svc, err := load(cfgPath)
		if err != nil {
			return err
		}
		defer svc.close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Query langsung ke database
		var tenants []coretenant.Tenant
		result := svc.fangs.DB().WithContext(ctx).
			Order("created_at DESC").
			Find(&tenants)

		if result.Error != nil {
			return fmt.Errorf("gagal mengambil data tenant: %w", result.Error)
		}

		if len(tenants) == 0 {
			fmt.Println("[foxctl] Belum ada tenant yang terdaftar.")
			fmt.Println("  Buat tenant pertama: foxctl tenant create --name \"Nama\" --slug slug-nya")
			return nil
		}

		fmt.Printf("[foxctl] Daftar tenant (%d):\n\n", len(tenants))

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "  SLUG\tNAMA\tPLAN\tSTATUS\tSCHEMA")
		fmt.Fprintln(w, "  ----\t----\t----\t------\t------")

		for _, t := range tenants {
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n",
				t.Slug, t.Name, t.Plan, t.Status, t.SchemaName)
		}
		w.Flush()
		fmt.Println()
		return nil
	},
}

// ── tenant suspend ────────────────────────────────────────────────

var tenantSuspendCmd = &cobra.Command{
	Use:   "suspend <slug>",
	Short: "Suspend tenant — tenant tidak bisa login",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]

		cfgPath := os.Getenv("VAMPIFOX_CONFIG")
		svc, err := load(cfgPath)
		if err != nil {
			return err
		}
		defer svc.close()

		if svc.tenantSvc == nil {
			return fmt.Errorf("tenant service tidak tersedia")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := svc.tenantSvc.Suspend(ctx, slug); err != nil {
			if err == coretenant.ErrNotFound {
				return fmt.Errorf("tenant '%s' tidak ditemukan", slug)
			}
			return fmt.Errorf("gagal suspend tenant: %w", err)
		}

		fmt.Printf("[OK] Tenant '%s' berhasil disuspend.\n", slug)
		fmt.Println("  Untuk mengaktifkan kembali: foxctl tenant unsuspend " + slug)
		return nil
	},
}

// ── tenant unsuspend ──────────────────────────────────────────────

var tenantUnsuspendCmd = &cobra.Command{
	Use:   "unsuspend <slug>",
	Short: "Aktifkan kembali tenant yang tersuspend",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]

		cfgPath := os.Getenv("VAMPIFOX_CONFIG")
		svc, err := load(cfgPath)
		if err != nil {
			return err
		}
		defer svc.close()

		if svc.tenantSvc == nil {
			return fmt.Errorf("tenant service tidak tersedia")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := svc.tenantSvc.Unsuspend(ctx, slug); err != nil {
			if err == coretenant.ErrNotFound {
				return fmt.Errorf("tenant '%s' tidak ditemukan", slug)
			}
			return fmt.Errorf("gagal unsuspend tenant: %w", err)
		}

		fmt.Printf("[OK] Tenant '%s' berhasil diaktifkan kembali.\n", slug)
		return nil
	},
}

func init() {
	tenantCreateCmd.Flags().String("name", "", "Nama tenant (wajib)")
	tenantCreateCmd.Flags().String("slug", "", "Slug unik tenant, e.g. pt-maju-jaya (wajib)")
	tenantCreateCmd.Flags().String("plan", "starter", "Plan: starter | growth | enterprise")
	tenantCreateCmd.Flags().String("domain", "", "Custom domain opsional, e.g. erp.pt-maju.com")

	tenantCmd.AddCommand(tenantCreateCmd)
	tenantCmd.AddCommand(tenantListCmd)
	tenantCmd.AddCommand(tenantSuspendCmd)
	tenantCmd.AddCommand(tenantUnsuspendCmd)
	rootCmd.AddCommand(tenantCmd)
}
