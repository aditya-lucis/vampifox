// Package fangs — Koneksi database VampiFox.
// "Fangs" adalah cara vampire menyerap sumber daya.
// Di sini, VampiFox menyerap data dari PostgreSQL.
package fangs

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// FangConfig konfigurasi koneksi database per-tenant.
type FangConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
	MaxOpen  int
	MaxIdle  int
}

// Bite membuka koneksi ke database — "vampire menggigit".
// Setiap tenant memiliki koneksi database tersendiri (isolated schema strategy).
func Bite(cfg FangConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Jakarta",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		// PrepareStmt: cache prepared statements untuk performa
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("fangs gagal menggigit DB: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	sqlDB.SetMaxIdleConns(cfg.MaxIdle)

	return db, nil
}

// Retract menutup koneksi database — "vampire menarik taringnya".
func Retract(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
