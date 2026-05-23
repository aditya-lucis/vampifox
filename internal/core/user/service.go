package user

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// ═══════════════════════════════════════════════════════════════
//  Service — business logic User
// ═══════════════════════════════════════════════════════════════

// Service mengorkestrasi operasi user.
type Service struct {
	repo       *Repository
	bcryptCost int
	logger     *zap.Logger
}

// NewService membuat Service baru.
// bcryptCost: cost factor bcrypt, recommended 12 untuk production.
func NewService(repo *Repository, bcryptCost int, logger *zap.Logger) *Service {
	if bcryptCost < 10 {
		bcryptCost = 12 // minimum aman
	}
	return &Service{
		repo:       repo,
		bcryptCost: bcryptCost,
		logger:     logger.Named("user.svc"),
	}
}

// ── Register ──────────────────────────────────────────────────────

// Register mendaftarkan user baru di tenant.
//
// Alur:
//  1. Validasi input
//  2. Cek email tidak duplikat
//  3. Hash password
//  4. Simpan ke DB
func (s *Service) Register(ctx context.Context, input RegisterInput, roles []string) (*User, error) {
	// Validasi
	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Cek duplikat email
	exists, err := s.repo.EmailExists(ctx, input.Email)
	if err != nil {
		return nil, fmt.Errorf("gagal cek email: %w", err)
	}
	if exists {
		return nil, ErrEmailTaken
	}

	// Buat user
	u := &User{
		Email:    input.Email,
		FullName: input.FullName,
		Status:   StatusActive,
		Roles:    roles,
	}

	// Hash password
	if err := u.SetPassword(input.Password, s.bcryptCost); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("gagal menyimpan user: %w", err)
	}

	s.logger.Info("User terdaftar",
		zap.String("id", u.ID.String()),
		zap.String("email", u.Email),
	)

	return u, nil
}

// ── Auth helpers ──────────────────────────────────────────────────

// Authenticate memverifikasi email + password dan mengembalikan User.
// Mencatat LastLoginAt jika berhasil.
//
// Mengembalikan ErrNotFound jika email tidak ada,
// ErrWrongPassword jika password salah,
// ErrInactive jika akun tidak aktif.
func (s *Service) Authenticate(ctx context.Context, email, password string) (*User, error) {
	u, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		// Jangan reveal apakah email terdaftar atau tidak
		// — kembalikan pesan generik untuk keamanan
		if err == ErrNotFound {
			return nil, ErrWrongPassword
		}
		return nil, err
	}

	if !u.IsActive() {
		return nil, ErrInactive
	}

	if err := u.CheckPassword(password); err != nil {
		return nil, err
	}

	// Catat waktu login (best-effort — jangan gagalkan login jika ini error)
	if err := s.repo.UpdateLastLogin(ctx, u); err != nil {
		s.logger.Warn("Gagal update last_login_at",
			zap.String("user_id", u.ID.String()),
			zap.Error(err),
		)
	}

	return u, nil
}

// ── Profile ───────────────────────────────────────────────────────

// FindByID mencari user berdasarkan ID.
func (s *Service) FindByID(ctx context.Context, id string) (*User, error) {
	return s.repo.FindByID(ctx, id)
}

// UpdateProfile memperbarui nama dan avatar user.
func (s *Service) UpdateProfile(ctx context.Context, userID string, input UpdateProfileInput) (*User, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if input.FullName != "" {
		u.FullName = input.FullName
	}
	if input.Avatar != "" {
		u.Avatar = input.Avatar
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, fmt.Errorf("gagal update profil: %w", err)
	}

	return u, nil
}

// ChangePassword mengganti password user setelah verifikasi password lama.
func (s *Service) ChangePassword(ctx context.Context, userID string, input ChangePasswordInput) error {
	if err := input.Validate(); err != nil {
		return err
	}

	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verifikasi password lama
	if err := u.CheckPassword(input.OldPassword); err != nil {
		return err
	}

	// Set password baru
	if err := u.SetPassword(input.NewPassword, s.bcryptCost); err != nil {
		return err
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return fmt.Errorf("gagal menyimpan password baru: %w", err)
	}

	s.logger.Info("Password berhasil diganti",
		zap.String("user_id", userID),
	)

	return nil
}

// ── Status management ─────────────────────────────────────────────

// Deactivate menonaktifkan akun user.
func (s *Service) Deactivate(ctx context.Context, userID string) error {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.UpdateStatus(ctx, u, StatusInactive)
}

// Ban memblokir akun user secara permanen.
func (s *Service) Ban(ctx context.Context, userID string) error {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.UpdateStatus(ctx, u, StatusBanned)
}

// ── Role management ───────────────────────────────────────────────

// AssignRole menambahkan role ke user.
func (s *Service) AssignRole(ctx context.Context, userID, role string) error {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	u.AddRole(role)
	return s.repo.Update(ctx, u)
}

// RevokeRole mencabut role dari user.
func (s *Service) RevokeRole(ctx context.Context, userID, role string) error {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if len(u.Roles) == 1 && u.HasRole(role) {
		return fmt.Errorf("user harus memiliki minimal satu role")
	}
	u.RemoveRole(role)
	return s.repo.Update(ctx, u)
}
