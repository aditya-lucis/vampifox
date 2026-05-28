package auth

import (
	"context"
)

// ═══════════════════════════════════════════════════════════════
//  TokenValidator — validasi token TANPA butuh database
// ═══════════════════════════════════════════════════════════════

// TokenValidator memvalidasi access token menggunakan hanya
// Sanctum (JWT) dan TokenStore (Redis blacklist).
// Aman sebagai singleton — tidak ada state per-tenant.
type TokenValidator struct {
	sanctum    *Sanctum
	tokenStore TokenStore
}

// NewTokenValidator membuat TokenValidator baru.
func NewTokenValidator(sanctum *Sanctum, tokenStore TokenStore) *TokenValidator {
	return &TokenValidator{
		sanctum:    sanctum,
		tokenStore: tokenStore,
	}
}

// ValidateAccessToken memvalidasi access token secara penuh:
//  1. Verify signature dan expiry (Sanctum) — pure crypto, no I/O
//  2. Cek blacklist token individual (TokenStore → Redis)
//  3. Cek apakah semua token user di-revoke (LogoutAllDevices)
func (v *TokenValidator) ValidateAccessToken(ctx context.Context, tokenStr string) (*BloodClaims, error) {
	// 1. Verify JWT signature dan expiry
	claims, err := v.sanctum.Verify(tokenStr)
	if err != nil {
		return nil, err
	}

	// 2. Cek blacklist token individual
	if claims.TokenID != "" {
		revoked, err := v.tokenStore.IsRevoked(ctx, claims.TokenID)
		if err != nil {
			// Redis error — fail open (tetap izinkan request)
			// Konsekuensi: token yang sudah di-revoke bisa lewat sementara
			// Ini trade-off availability vs security — ubah ke fail closed jika perlu
		} else if revoked {
			return nil, ErrTokenRevoked
		}
	}

	// 3. Cek apakah semua session user di-revoke (LogoutAllDevices)
	if store, ok := v.tokenStore.(*ShadowTokenStore); ok {
		userRevoked, err := store.IsUserRevoked(ctx, claims.UserID.String())
		if err == nil && userRevoked {
			return nil, ErrTokenRevoked
		}
	}

	return claims, nil
}
