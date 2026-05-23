// Package auth — Sistem autentikasi VampiFox.
//
// "Hanya yang diundang yang boleh masuk" — aturan klasik vampire.
//
// Auth package terdiri dari dua bagian:
//
//  1. Sanctum — JWT manager: issue, verify, refresh token
//  2. Service — orchestrator: login, logout, refresh, integrasi dengan user.Service
//
// Flow autentikasi:
//
//	Login → user.Authenticate() → Sanctum.Invite() → TokenPair
//	Request → Sanctum.Verify() → BloodClaims → inject ke context
//	Refresh → Sanctum.Renew() → TokenPair baru + blacklist token lama
//	Logout → blacklist refresh token di Shadow
//
// Refresh Token Rotation:
//
//	Setiap kali refresh, token lama langsung di-blacklist.
//	Jika token lama dipakai lagi (reuse detection), semua session
//	user tersebut dianggap compromised dan di-revoke semua.
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ═══════════════════════════════════════════════════════════════
//  Sentinel errors
// ═══════════════════════════════════════════════════════════════

var (
	ErrInvalidToken = errors.New("token tidak valid")
	ErrTokenExpired = errors.New("token sudah kadaluarsa")
	ErrTokenRevoked = errors.New("token sudah dicabut — silakan login ulang")
	ErrNotInvited   = errors.New("akses ditolak")
)

// ═══════════════════════════════════════════════════════════════
//  BloodClaims — JWT payload VampiFox
// ═══════════════════════════════════════════════════════════════

// BloodClaims adalah payload JWT yang dibawa setiap request.
// "Blood" karena JWT claims adalah "darah" identitas — mengalir
// di setiap request, membawa bukti siapa pemakainya.
type BloodClaims struct {
	UserID     uuid.UUID `json:"uid"`
	TenantID   uuid.UUID `json:"tid"`
	TenantSlug string    `json:"tslug"`
	Email      string    `json:"email"`
	Roles      []string  `json:"roles"`
	// TokenID digunakan untuk blacklist individual token
	TokenID string `json:"jti"`
	jwt.RegisteredClaims
}

// ═══════════════════════════════════════════════════════════════
//  TokenPair — hasil login / refresh
// ═══════════════════════════════════════════════════════════════

// TokenPair adalah sepasang token yang diberikan saat login berhasil.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"` // selalu "Bearer"
}

// ═══════════════════════════════════════════════════════════════
//  Sanctum — JWT manager
// ═══════════════════════════════════════════════════════════════

// Sanctum mengelola pembuatan dan validasi JWT.
// Satu instance Sanctum per aplikasi — dibuat di Den.Awaken().
//
// "Sanctum" = tempat suci yang terlindungi. Hanya Sanctum yang
// tahu secret key dan bisa memvalidasi token.
type Sanctum struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
	issuer        string
}

// SanctumConfig konfigurasi untuk membuat Sanctum.
type SanctumConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	Issuer        string
}

// NewSanctum membuat Sanctum baru dari konfigurasi.
func NewSanctum(cfg SanctumConfig) *Sanctum {
	accessTTL := cfg.AccessTTL
	if accessTTL == 0 {
		accessTTL = 15 * time.Minute
	}
	refreshTTL := cfg.RefreshTTL
	if refreshTTL == 0 {
		refreshTTL = 7 * 24 * time.Hour
	}
	issuer := cfg.Issuer
	if issuer == "" {
		issuer = "vampifox"
	}
	return &Sanctum{
		accessSecret:  []byte(cfg.AccessSecret),
		refreshSecret: []byte(cfg.RefreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
		issuer:        issuer,
	}
}

// ── Invite — issue token pair ─────────────────────────────────────

// Invite menerbitkan TokenPair untuk user yang berhasil diautentikasi.
// "Mengundang" user masuk ke sistem.
//
// tokenID digunakan untuk refresh token rotation dan revocation.
// Jika kosong, UUID baru akan di-generate.
func (s *Sanctum) Invite(
	userID, tenantID uuid.UUID,
	tenantSlug, email string,
	roles []string,
	tokenID string,
) (*TokenPair, error) {
	now := time.Now()
	expiresAt := now.Add(s.accessTTL)

	if tokenID == "" {
		tokenID = uuid.New().String()
	}

	// ── Access token ───────────────────────────────────────────
	accessClaims := BloodClaims{
		UserID:     userID,
		TenantID:   tenantID,
		TenantSlug: tenantSlug,
		Email:      email,
		Roles:      roles,
		TokenID:    tokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Issuer:    s.issuer,
			ID:        tokenID,
		},
	}

	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).
		SignedString(s.accessSecret)
	if err != nil {
		return nil, err
	}

	// ── Refresh token ──────────────────────────────────────────
	// Refresh token hanya menyimpan userID dan tokenID —
	// tidak ada info sensitif lain.
	refreshID := uuid.New().String()
	refreshClaims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshTTL)),
		Issuer:    s.issuer + "-refresh",
		ID:        refreshID,
	}

	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).
		SignedString(s.refreshSecret)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		TokenType:    "Bearer",
	}, nil
}

// ── Verify — validasi access token ───────────────────────────────

// Verify memvalidasi access token dan mengembalikan BloodClaims-nya.
//
// TIDAK memeriksa blacklist — itu dilakukan di layer middleware
// via TokenStore.IsRevoked(). Pemisahan ini membuat Sanctum
// tidak bergantung pada Shadow/Redis.
func (s *Sanctum) Verify(tokenStr string) (*BloodClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&BloodClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidToken
			}
			return s.accessSecret, nil
		},
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*BloodClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ── ParseRefresh — parse refresh token ───────────────────────────

// ParseRefresh memvalidasi refresh token dan mengembalikan claims-nya.
// Dipanggil oleh Service.Refresh() sebelum issue token baru.
func (s *Sanctum) ParseRefresh(tokenStr string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&jwt.RegisteredClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidToken
			}
			return s.refreshSecret, nil
		},
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ── Getters ───────────────────────────────────────────────────────

// AccessTTL mengembalikan durasi hidup access token.
func (s *Sanctum) AccessTTL() time.Duration { return s.accessTTL }

// RefreshTTL mengembalikan durasi hidup refresh token.
func (s *Sanctum) RefreshTTL() time.Duration { return s.refreshTTL }

// ═══════════════════════════════════════════════════════════════
//  TokenStore interface — abstraksi blacklist storage
// ═══════════════════════════════════════════════════════════════

// TokenStore adalah interface untuk menyimpan dan memeriksa
// token yang sudah di-revoke.
//
// Implementasi default menggunakan Shadow (Redis).
// Interface ini memudahkan testing tanpa Redis.
type TokenStore interface {
	// Revoke menandai tokenID sebagai tidak valid sampai ttl habis.
	Revoke(ctx context.Context, tokenID string, ttl time.Duration) error

	// IsRevoked memeriksa apakah tokenID sudah di-revoke.
	IsRevoked(ctx context.Context, tokenID string) (bool, error)

	// RevokeAllForUser mencabut semua token milik userID.
	// Dipanggil saat reuse detection atau paksa logout semua device.
	RevokeAllForUser(ctx context.Context, userID string) error
}
