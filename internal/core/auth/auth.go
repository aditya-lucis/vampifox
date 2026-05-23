// Package auth — Sistem autentikasi VampiFox.
// "Hanya yang diundang yang boleh masuk" — aturan klasik vampire.
// VampiFox menggunakan JWT dengan refresh token strategy.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenPair hasil autentikasi sukses.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"` // "Bearer"
}

// BloodClaims — klaim JWT VampiFox.
// "Blood" karena JWT claims adalah "darah" identitas user.
type BloodClaims struct {
	UserID     uuid.UUID `json:"uid"`
	TenantID   uuid.UUID `json:"tid"`
	TenantSlug string    `json:"tslug"`
	Email      string    `json:"email"`
	Roles      []string  `json:"roles"`
	jwt.RegisteredClaims
}

var (
	ErrInvalidToken = errors.New("token tidak valid atau sudah kadaluarsa")
	ErrTokenExpired = errors.New("token sudah kadaluarsa")
	ErrNotInvited   = errors.New("akses ditolak — kamu tidak diundang")
)

// Sanctum mengelola pembuatan dan validasi token.
// "Sanctum" = tempat suci/terlindungi dalam mitologi vampire.
type Sanctum struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration // default: 15 menit
	refreshTTL    time.Duration // default: 7 hari
}

// NewSanctum membuat Sanctum baru.
func NewSanctum(accessSecret, refreshSecret string) *Sanctum {
	return &Sanctum{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     15 * time.Minute,
		refreshTTL:    7 * 24 * time.Hour,
	}
}

// Invite membuat token pair untuk user yang berhasil login.
// "Mengundang" user masuk ke sistem.
func (s *Sanctum) Invite(
	userID,
	tenantID uuid.UUID,
	tenantSlug,
	email string,
	roles []string,
) (*TokenPair, error) {
	now := time.Now()
	expiresAt := now.Add(s.accessTTL)

	claims := BloodClaims{
		UserID:     userID,
		TenantID:   tenantID,
		TenantSlug: tenantSlug,
		Email:      email,
		Roles:      roles,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Issuer:    "vampifox",
		},
	}

	accessToken, err := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	).SignedString(s.accessSecret)
	if err != nil {
		return nil, err
	}

	refreshClaims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshTTL)),
		Issuer:    "vampifox-refresh",
	}

	refreshToken, err := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		refreshClaims,
	).SignedString(s.refreshSecret)
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

// Verify memvalidasi access token dan mengembalikan claims-nya.
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
