package tenant

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/fangs"
)

// ═══════════════════════════════════════════════════════════════
//  Service — business logic untuk Tenant
// ═══════════════════════════════════════════════════════════════

// Service mengorkestrasi operasi tenant yang melibatkan
// lebih dari satu layer (repository + fangs + notifikasi).
type Service struct {
	repo   *Repository
	fangs  *fangs.Fangs
	logger *zap.Logger
}

// NewService membuat Service baru.
func NewService(repo *Repository, f *fangs.Fangs, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		fangs:  f,
		logger: logger.Named("tenant.svc"),
	}
}

// ── Provisioning ──────────────────────────────────────────────────

// Provision membuat tenant baru beserta schema database-nya.
//
// Alur:
//  1. Validasi input
//  2. Cek slug tidak duplikat
//  3. Buat record Tenant di DB sistem
//  4. Buat schema database untuk tenant
//  5. Return tenant yang sudah dibuat
//
// Operasi ini idempotent di sisi schema — jika schema sudah ada
// (karena provision sebelumnya gagal di tengah), tidak akan error.
func (s *Service) Provision(ctx context.Context, input CreateInput) (*Tenant, error) {
	// ── Validasi input ────────────────────────────────────────────
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("input tidak valid: %w", err)
	}

	// ── Cek slug duplikat ─────────────────────────────────────────
	exists, err := s.repo.SlugExists(ctx, input.Slug)
	if err != nil {
		return nil, fmt.Errorf("gagal cek slug: %w", err)
	}
	if exists {
		return nil, ErrSlugTaken
	}

	// ── Cek domain duplikat ───────────────────────────────────────
	if input.Domain != "" {
		domainExists, err := s.repo.DomainExists(ctx, input.Domain)
		if err != nil {
			return nil, fmt.Errorf("gagal cek domain: %w", err)
		}
		if domainExists {
			return nil, fmt.Errorf("domain '%s' sudah digunakan tenant lain", input.Domain)
		}
	}

	// ── Set defaults berdasarkan plan ─────────────────────────────
	maxUsers, storageGB := planDefaults(input.Plan)
	if input.MaxUsers > 0 {
		maxUsers = input.MaxUsers
	}
	if input.StorageGB > 0 {
		storageGB = input.StorageGB
	}

	// ── Buat Tenant record ────────────────────────────────────────
	tenant := &Tenant{
		Slug: input.Slug,
		Name:       input.Name,
		Domain:     input.Domain,
		Plan:       input.Plan,
		Status:     StatusActive,
		SchemaName: SchemaNameFor(input.Slug),
		MaxUsers:   maxUsers,
		StorageGB:  storageGB,
		Settings:   make(Settings),
	}

	if err := s.repo.Create(ctx, tenant); err != nil {
		return nil, fmt.Errorf("gagal menyimpan tenant: %w", err)
	}

	// ── Buat schema database ──────────────────────────────────────
	if err := s.fangs.CreateTenantSchema(ctx, tenant.SchemaName); err != nil {
		// Schema gagal dibuat — log sebagai warning tapi jangan rollback tenant record.
		// Admin bisa retry provisioning schema via foxctl.
		s.logger.Error("Gagal membuat schema tenant — tenant tersimpan tapi schema belum ada",
			zap.String("slug", tenant.Slug),
			zap.String("schema", tenant.SchemaName),
			zap.Error(err),
		)
		return tenant, fmt.Errorf("tenant dibuat tapi schema gagal: %w", err)
	}

	s.logger.Info("Tenant berhasil di-provision",
		zap.String("slug", tenant.Slug),
		zap.String("schema", tenant.SchemaName),
		zap.String("plan", string(tenant.Plan)),
	)

	return tenant, nil
}

// ── Lookups ───────────────────────────────────────────────────────

// FindBySlug mencari dan memvalidasi tenant berdasarkan slug.
// Mengembalikan error jika tidak ditemukan atau status tidak aktif.
func (s *Service) FindBySlug(ctx context.Context, slug string) (*Tenant, error) {
	t, err := s.repo.FindBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if err := t.ValidateStatus(); err != nil {
		return nil, err
	}
	return t, nil
}

// FindByDomain mencari dan memvalidasi tenant berdasarkan custom domain.
func (s *Service) FindByDomain(ctx context.Context, domain string) (*Tenant, error) {
	t, err := s.repo.FindByDomain(ctx, domain)
	if err != nil {
		return nil, err
	}
	if err := t.ValidateStatus(); err != nil {
		return nil, err
	}
	return t, nil
}

// ── Status management ─────────────────────────────────────────────

// Suspend menonaktifkan tenant sementara.
func (s *Service) Suspend(ctx context.Context, slug string) error {
	t, err := s.repo.FindBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if t.Status == StatusSuspended {
		return nil // sudah suspended, idempotent
	}
	return s.repo.UpdateStatus(ctx, t, StatusSuspended)
}

// Unsuspend mengaktifkan kembali tenant yang tersuspend.
func (s *Service) Unsuspend(ctx context.Context, slug string) error {
	t, err := s.repo.FindBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if t.Status != StatusSuspended {
		return fmt.Errorf("tenant '%s' tidak dalam status suspended", slug)
	}
	return s.repo.UpdateStatus(ctx, t, StatusActive)
}

// UpdateSettings memperbarui settings tenant.
func (s *Service) UpdateSettings(ctx context.Context, slug string, settings Settings) error {
	t, err := s.repo.FindBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if t.Settings == nil {
		t.Settings = make(Settings)
	}
	// Merge settings — tidak replace keseluruhan
	for k, v := range settings {
		t.Settings[k] = v
	}
	return s.repo.UpdateSettings(ctx, t)
}

// ── Helpers ───────────────────────────────────────────────────────

// planDefaults mengembalikan nilai default maxUsers dan storageGB
// berdasarkan plan yang dipilih.
func planDefaults(plan Plan) (maxUsers, storageGB int) {
	switch plan {
	case PlanGrowth:
		return 50, 20
	case PlanEnterprise:
		return 500, 100
	default: // PlanStarter
		return 10, 5
	}
}
