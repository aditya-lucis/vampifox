package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aditya-lucis/vampifox/internal/shadow"
)

// ═══════════════════════════════════════════════════════════════
//  ShadowTokenStore — implementasi TokenStore menggunakan Redis
// ═══════════════════════════════════════════════════════════════

// ShadowTokenStore menyimpan revoked token di Redis via Shadow.
//
// Key strategy:
//   - Token individual : "revoked:token:{tokenID}"
//   - User tokens      : "revoked:user:{userID}"  → set of tokenIDs
//
// TTL setiap entry mengikuti sisa TTL token yang di-revoke,
// sehingga Redis tidak menyimpan data yang sudah tidak relevan.
type ShadowTokenStore struct {
	shadow *shadow.TenantShadow
}

// NewShadowTokenStore membuat ShadowTokenStore baru.
// ts harus menggunakan namespace sistem (bukan tenant),
// karena token revocation adalah operasi lintas tenant.
func NewShadowTokenStore(sh *shadow.Shadow) *ShadowTokenStore {
	return &ShadowTokenStore{
		// Namespace "_auth" untuk memisahkan dari cache data bisnis
		shadow: sh.ForTenant("_auth"),
	}
}

// ── Revoke ────────────────────────────────────────────────────────

// Revoke menandai satu token sebagai tidak valid.
// ttl sebaiknya = sisa waktu token itu valid, supaya entry
// otomatis terhapus saat token sudah expired anyway.
func (s *ShadowTokenStore) Revoke(ctx context.Context, tokenID string, ttl time.Duration) error {
	key := revokedTokenKey(tokenID)
	if err := s.shadow.Haunt(ctx, key, true, ttl); err != nil {
		return fmt.Errorf("[TokenStore] gagal revoke token %s: %w", tokenID, err)
	}
	return nil
}

// IsRevoked memeriksa apakah tokenID ada di blacklist.
func (s *ShadowTokenStore) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	exists, err := s.shadow.Exists(ctx, revokedTokenKey(tokenID))
	if err != nil {
		return false, fmt.Errorf("[TokenStore] gagal cek revoked token %s: %w", tokenID, err)
	}
	return exists, nil
}

// RevokeAllForUser mencabut semua session milik satu user.
// Dipanggil saat:
//   - Refresh token reuse terdeteksi (possible token theft)
//   - User memilih "logout dari semua device"
//   - Admin force-logout user
func (s *ShadowTokenStore) RevokeAllForUser(ctx context.Context, userID string) error {
	// Tandai userID sebagai "all revoked" — lebih efisien dari
	// melacak setiap tokenID individual
	key := revokedUserKey(userID)
	// TTL 30 hari — cukup panjang untuk menutup semua token aktif
	if err := s.shadow.Haunt(ctx, key, true, 30*24*time.Hour); err != nil {
		return fmt.Errorf("[TokenStore] gagal revoke all tokens user %s: %w", userID, err)
	}
	return nil
}

// IsUserRevoked memeriksa apakah SEMUA token user di-revoke.
// Dipanggil bersamaan dengan IsRevoked di middleware.
func (s *ShadowTokenStore) IsUserRevoked(ctx context.Context, userID string) (bool, error) {
	exists, err := s.shadow.Exists(ctx, revokedUserKey(userID))
	if err != nil {
		return false, fmt.Errorf("[TokenStore] gagal cek revoked user %s: %w", userID, err)
	}
	return exists, nil
}

// ── Key builders ──────────────────────────────────────────────────

func revokedTokenKey(tokenID string) string {
	return shadow.BuildKey("revoked", "token", tokenID)
}

func revokedUserKey(userID string) string {
	return shadow.BuildKey("revoked", "user", userID)
}
