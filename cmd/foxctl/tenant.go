package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var tenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Kelola tenant VampiFox",
}

var tenantCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Provision tenant baru",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		slug, _ := cmd.Flags().GetString("slug")
		plan, _ := cmd.Flags().GetString("plan")

		if name == "" || slug == "" {
			return fmt.Errorf("--name dan --slug wajib diisi")
		}

		fmt.Printf("[foxctl] Provisioning tenant...\n")
		fmt.Printf("  Nama  : %s\n", name)
		fmt.Printf("  Slug  : %s\n", slug)
		fmt.Printf("  Plan  : %s\n", plan)
		fmt.Printf("  Schema: vfx_%s\n", slug)
		// TODO: load config → init services → tenant.Service.Provision()
		fmt.Println("[foxctl] (TODO: implementasi penuh)")
		return nil
	},
}

var tenantListCmd = &cobra.Command{
	Use:   "list",
	Short: "Tampilkan semua tenant aktif",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("[foxctl] Daftar tenant:")
		fmt.Println("  (TODO: implementasi penuh)")
		return nil
	},
}

var tenantSuspendCmd = &cobra.Command{
	Use:   "suspend [slug]",
	Short: "Suspend tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("[foxctl] Menyuspend tenant: %s\n", args[0])
		// TODO: implementasi
		return nil
	},
}

func init() {
	tenantCreateCmd.Flags().String("name", "", "Nama tenant (wajib)")
	tenantCreateCmd.Flags().String("slug", "", "Slug tenant, e.g. pt-maju-jaya (wajib)")
	tenantCreateCmd.Flags().String("plan", "starter", "Plan: starter|growth|enterprise")

	tenantCmd.AddCommand(tenantCreateCmd)
	tenantCmd.AddCommand(tenantListCmd)
	tenantCmd.AddCommand(tenantSuspendCmd)
	rootCmd.AddCommand(tenantCmd)
}
