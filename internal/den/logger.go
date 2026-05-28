package den

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/aditya-lucis/vampifox/internal/config"
)

// ═══════════════════════════════════════════════════════════════
//  NightVision Logger — Zap logger factory untuk VampiFox.
//  Logger ini dipakai di seluruh aplikasi via Den.
// ═══════════════════════════════════════════════════════════════

// buildLogger membuat zap.Logger sesuai konfigurasi LogConfig.
// Dipanggil sekali saat Den.Awaken().
func buildLogger(cfg config.LogConfig) (*zap.Logger, error) {
	// ── Level ─────────────────────────────────────────────────────
	level, err := parseLogLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	// ── Encoder ───────────────────────────────────────────────────
	var encoderCfg zapcore.EncoderConfig
	if cfg.Format == "json" {
		encoderCfg = zap.NewProductionEncoderConfig()
	} else {
		// Console: human-readable, berwarna di terminal
		encoderCfg = zap.NewDevelopmentEncoderConfig()
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoderCfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	}

	var encoder zapcore.Encoder
	if cfg.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	}

	// ── Output ────────────────────────────────────────────────────
	var cores []zapcore.Core

	if cfg.Output == "stdout" || cfg.Output == "both" {
		stdoutCore := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)
		cores = append(cores, stdoutCore)
	}

	if cfg.Output == "file" || cfg.Output == "both" {
		fileWriter, err := openLogFile(cfg.FilePath)
		if err != nil {
			return nil, fmt.Errorf("[NightVision] gagal membuka file log: %w", err)
		}
		// File selalu JSON untuk kemudahan parsing
		fileEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
		fileCore := zapcore.NewCore(fileEncoder, zapcore.AddSync(fileWriter), level)
		cores = append(cores, fileCore)
	}

	if len(cores) == 0 {
		// Fallback ke stdout
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level))
	}

	// ── Combine & Build ───────────────────────────────────────────
	core := zapcore.NewTee(cores...)
	logger := zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)

	return logger, nil
}

// parseLogLevel mengkonversi string level ke zapcore.Level.
func parseLogLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info", "":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf(
			"[NightVision] log level '%s' tidak dikenal. Pilihan: debug, info, warn, error",
			level,
		)
	}
}

// openLogFile membuka atau membuat file log.
// Jika direktori tidak ada, otomatis dibuat.
func openLogFile(path string) (*os.File, error) {
	// Buat direktori jika belum ada
	dir := path[:strings.LastIndex(path, "/")]
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("gagal membuat direktori log '%s': %w", dir, err)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("gagal membuka file log '%s': %w", path, err)
	}
	return f, nil
}
