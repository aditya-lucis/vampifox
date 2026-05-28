package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Kelola migrasi database",
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Jalankan semua migrasi yang belum diaplikasikan",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("[foxctl] Menjalankan migrasi...")
		// TODO: load config → init Fangs → run MigrationRunner.Up()
		fmt.Println("[foxctl] Tidak ada migrasi baru. (TODO: implementasi penuh)")
		return nil
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Rollback migrasi terakhir",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("[foxctl] Rollback migrasi terakhir...")
		// TODO: implementasi
		return nil
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Tampilkan status semua migrasi",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("[foxctl] Status migrasi:")
		fmt.Println("  (TODO: implementasi penuh)")
		return nil
	},
}

var migrateCreateCmd = &cobra.Command{
	Use:   "create [nama]",
	Short: "Buat file migrasi baru",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		fmt.Printf("[foxctl] Membuat migrasi: %s\n", name)
		// TODO: generate file dengan timestamp prefix
		return nil
	},
}

func init() {
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateCreateCmd)
	rootCmd.AddCommand(migrateCmd)
}
