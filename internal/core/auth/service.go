package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/core/user"
)

// ═══════════════════════════════════════════════════════════════
//  Service — orchestrator autentikasi
// ═══════════════════════════════════════════════════════════════

// Service mengorkestrasikan seluruh flow autentikasi:
// login, logout, refresh token, dan validasi session.
//
// Service bergantung pada:
//   - user.Service   → verifikasi kredensial
//   - Sanctum        → issue dan verify JWT
//   - TokenStore     → blacklist token yang di-revoke
type Service struct {
	userSvc    *user.Service
	sanctum    *Sanctum
	tokenStore TokenStore
	logger     *zap.Logger
}

// NewService membuat Service baru.
func NewService(
	userSvc *user.Service,
	sanctum *Sanctum,
	tokenStore TokenStore,
	logger *zap.Logger,
) *Service {
	return &Service{
		userSvc:    userSvc,
		sanctum:    sanctum,
		tokenStore: tokenStore,
		logger:     logger.Named("auth.svc"),
	}
}

// ── Login ─────────────────────────────────────────────────────────

// LoginInput data yang dibutuhkan untuk login.
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResult hasil login yang berhasil.
type LoginResult struct {
	Tokens *TokenPair
	User   *user.User
}

// Login memverifikasi kredensial dan menerbitkan TokenPair.
//
// Mengembalikan error jika:
//   - Email tidak ditemukan (ErrWrongPassword — tidak reveal)
//   - Password salah (ErrWrongPassword)
//   - Akun tidak aktif (user.ErrInactive)
func (s *Service) Login(
	ctx context.Context,
	tenantID uuid.UUID,
	tenantSlug string,
	input LoginInput,
) (*LoginResult, error) {
	// Autentikasi via user service
	u, err := s.userSvc.Authenticate(ctx, input.Email, input.Password)
	if err != nil {
		return nil, err
	}

	// Issue token pair
	tokens, err := s.sanctum.Invite(
		u.ID, tenantID, tenantSlug,
		u.Email, u.Roles,
		"", // tokenID di-generate otomatis
	)
	if err != nil {
		return nil, fmt.Errorf("[Auth] gagal issue token: %w", err)
	}

	s.logger.Info("Login berhasil",
		zap.String("user_id", u.ID.String()),
		zap.String("email", u.Email),
		zap.String("tenant", tenantSlug),
	)

	return &LoginResult{Tokens: tokens, User: u}, nil
}

// ── Refresh ───────────────────────────────────────────────────────

// Refresh menukar refresh token lama dengan TokenPair baru.
//
// Refresh Token Rotation:
//  1. Parse dan validasi refresh token lama
//  2. Cek apakah token sudah di-revoke (reuse detection)
//  3. Jika di-revoke → token theft terdeteksi → revoke SEMUA session user
//  4. Load data user terbaru dari DB
//  5. Revoke refresh token lama
//  6. Issue TokenPair baru
func (s *Service) Refresh(
	ctx context.Context,
	refreshTokenStr string,
	tenantID uuid.UUID,
	tenantSlug string,
) (*TokenPair, error) {
	// Parse refresh token
	claims, err := s.sanctum.ParseRefresh(refreshTokenStr)
	if err != nil {
		return nil, err
	}

	tokenID := claims.ID
	userID := claims.Subject

	// Cek apakah token sudah di-revoke
	revoked, err := s.tokenStore.IsRevoked(ctx, tokenID)
	if err != nil {
		return nil, fmt.Errorf("[Auth] gagal cek revoked token: %w", err)
	}

	if revoked {
		// Token reuse terdeteksi — kemungkinan token theft!
		// Revoke SEMUA session milik user ini
		s.logger.Warn("Refresh token reuse terdeteksi — kemungkinan token theft!",
			zap.String("user_id", userID),
			zap.String("token_id", tokenID),
			zap.String("tenant", tenantSlug),
		)
		_ = s.tokenStore.RevokeAllForUser(ctx, userID)
		return nil, ErrTokenRevoked
	}

	// Load user terbaru
	u, err := s.userSvc.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("[Auth] user tidak ditemukan saat refresh: %w", err)
	}
	if !u.IsActive() {
		return nil, user.ErrInactive
	}

	// Revoke refresh token lama
	remainingTTL := time.Until(claims.ExpiresAt.Time)
	if remainingTTL < 0 {
		remainingTTL = 0
	}
	if err := s.tokenStore.Revoke(ctx, tokenID, remainingTTL+time.Minute); err != nil {
		// Log tapi lanjut — jangan gagalkan refresh karena ini
		s.logger.Warn("Gagal revoke refresh token lama",
			zap.String("token_id", tokenID),
			zap.Error(err),
		)
	}

	// Issue token baru
	tokens, err := s.sanctum.Invite(
		u.ID, tenantID, tenantSlug,
		u.Email, u.Roles,
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("[Auth] gagal issue token baru: %w", err)
	}

	s.logger.Info("Token berhasil di-refresh",
		zap.String("user_id", userID),
		zap.String("tenant", tenantSlug),
	)

	return tokens, nil
}

// ── Logout ────────────────────────────────────────────────────────

// Logout mencabut refresh token sehingga tidak bisa dipakai lagi.
// Access token tetap valid sampai expired (tidak ada cara revoke
// access token individual — TTL-nya pendek, 15 menit).
func (s *Service) Logout(ctx context.Context, refreshTokenStr string) error {
	claims, err := s.sanctum.ParseRefresh(refreshTokenStr)
	if err != nil {
		// Token invalid — anggap sudah logout
		return nil
	}

	remainingTTL := time.Until(claims.ExpiresAt.Time)
	if remainingTTL <= 0 {
		return nil // sudah expired, tidak perlu diapa-apakan
	}

	if err := s.tokenStore.Revoke(ctx, claims.ID, remainingTTL+time.Minute); err != nil {
		return fmt.Errorf("[Auth] gagal revoke token saat logout: %w", err)
	}

	s.logger.Info("Logout berhasil",
		zap.String("user_id", claims.Subject),
		zap.String("token_id", claims.ID),
	)

	return nil
}

// LogoutAllDevices mencabut semua session milik user.
// Dipanggil saat user minta "keluar dari semua perangkat".
func (s *Service) LogoutAllDevices(ctx context.Context, userID string) error {
	if err := s.tokenStore.RevokeAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("[Auth] gagal revoke all sessions: %w", err)
	}

	s.logger.Info("Semua session di-revoke",
		zap.String("user_id", userID),
	)

	return nil
}

// ── Validate session ──────────────────────────────────────────────

// ValidateAccessToken memvalidasi access token secara penuh:
//  1. Verify signature dan expiry via Sanctum
//  2. Cek blacklist token individual
//  3. Cek apakah semua token user di-revoke
//
// Dipanggil oleh Bloodgate middleware di setiap request.
func (s *Service) ValidateAccessToken(ctx context.Context, tokenStr string) (*BloodClaims, error) {
	// Verify JWT
	claims, err := s.sanctum.Verify(tokenStr)
	if err != nil {
		return nil, err
	}

	// Cek blacklist token individual
	if claims.TokenID != "" {
		revoked, err := s.tokenStore.IsRevoked(ctx, claims.TokenID)
		if err != nil {
			// Redis down — fail open (allow) dengan log warning
			// Kebijakan ini bisa diubah ke fail closed sesuai kebutuhan
			s.logger.Warn("Gagal cek token blacklist — fail open",
				zap.String("token_id", claims.TokenID),
				zap.Error(err),
			)
		} else if revoked {
			return nil, ErrTokenRevoked
		}
	}

	// Cek apakah semua token user di-revoke (LogoutAllDevices)
	if store, ok := s.tokenStore.(*ShadowTokenStore); ok {
		userRevoked, err := store.IsUserRevoked(ctx, claims.UserID.String())
		if err == nil && userRevoked {
			return nil, ErrTokenRevoked
		}
	}

	return claims, nil
}
