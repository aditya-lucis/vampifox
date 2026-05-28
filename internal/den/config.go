package den

import (
	"github.com/aditya-lucis/vampifox/internal/config"
)

// VampConfig adalah alias ke config.VampConfig untuk backward compatibility.
// Den menggunakan config package untuk semua struct konfigurasi.
type VampConfig = config.VampConfig

// LoadConfig membaca konfigurasi dari file dan env vars.
func LoadConfig(configPath string) (*VampConfig, error) {
	return config.Load(configPath)
}
