package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ── Test helpers ──────────────────────────────────────────────────

func newTestSanctum() *Sanctum {
	return NewSanctum(SanctumConfig{
		AccessSecret:  "test-access-secret-32-characters!",
		RefreshSecret: "test-refresh-secret-32-characters",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    7 * 24 * time.Hour,
		Issuer:        "vampifox-test",
	})
}

func newTestIDs() (userID, tenantID uuid.UUID) {
	return uuid.New(), uuid.New()
}

// ── Test: NewSanctum defaults ─────────────────────────────────────

func TestNewSanctum_Defaults(t *testing.T) {
	s := NewSanctum(SanctumConfig{
		AccessSecret:  "access-secret-32-characters-long",
		RefreshSecret: "refresh-secret-32-characters-lon",
	})
	// Harus pakai default TTL
	if s.AccessTTL() != 15*time.Minute {
		t.Errorf("AccessTTL default = %v, want 15m", s.AccessTTL())
	}
	if s.RefreshTTL() != 7*24*time.Hour {
		t.Errorf("RefreshTTL default = %v, want 168h", s.RefreshTTL())
	}
}

// ── Test: Invite & Verify ─────────────────────────────────────────

func TestSanctum_Invite_Verify(t *testing.T) {
	s := newTestSanctum()
	userID, tenantID := newTestIDs()

	pair, err := s.Invite(
		userID, tenantID, "pt-maju", "user@example.com",
		[]string{"familiar"}, "",
	)
	if err != nil {
		t.Fatalf("Invite() gagal: %v", err)
	}

	if pair.AccessToken == "" {
		t.Error("AccessToken tidak boleh kosong")
	}
	if pair.RefreshToken == "" {
		t.Error("RefreshToken tidak boleh kosong")
	}
	if pair.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want Bearer", pair.TokenType)
	}
	if pair.ExpiresAt.IsZero() {
		t.Error("ExpiresAt tidak boleh zero")
	}

	// Verify access token
	claims, err := s.Verify(pair.AccessToken)
	if err != nil {
		t.Fatalf("Verify() gagal: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("UserID = %v, want %v", claims.UserID, userID)
	}
	if claims.TenantID != tenantID {
		t.Errorf("TenantID = %v, want %v", claims.TenantID, tenantID)
	}
	if claims.TenantSlug != "pt-maju" {
		t.Errorf("TenantSlug = %q, want pt-maju", claims.TenantSlug)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("Email = %q, want user@example.com", claims.Email)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "familiar" {
		t.Errorf("Roles = %v, want [familiar]", claims.Roles)
	}
	if claims.TokenID == "" {
		t.Error("TokenID tidak boleh kosong")
	}
}

func TestSanctum_Verify_InvalidToken(t *testing.T) {
	s := newTestSanctum()

	_, err := s.Verify("ini.bukan.token.valid")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Verify token invalid harus ErrInvalidToken, got: %v", err)
	}
}

func TestSanctum_Verify_WrongSecret(t *testing.T) {
	s1 := newTestSanctum()
	s2 := NewSanctum(SanctumConfig{
		AccessSecret:  "different-secret-32-characters!!!",
		RefreshSecret: "different-refresh-32-characters!!",
	})

	userID, tenantID := newTestIDs()
	pair, _ := s1.Invite(userID, tenantID, "test", "e@e.com", nil, "")

	// s2 tidak bisa verify token yang dibuat s1
	_, err := s2.Verify(pair.AccessToken)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Verify dengan secret berbeda harus ErrInvalidToken, got: %v", err)
	}
}

func TestSanctum_Verify_ExpiredToken(t *testing.T) {
	// Sanctum dengan TTL sangat pendek
	s := NewSanctum(SanctumConfig{
		AccessSecret:  "test-access-secret-32-characters!",
		RefreshSecret: "test-refresh-secret-32-characters",
		AccessTTL:     1 * time.Millisecond,
		RefreshTTL:    1 * time.Millisecond,
	})

	userID, tenantID := newTestIDs()
	pair, _ := s.Invite(userID, tenantID, "test", "e@e.com", nil, "")

	// Tunggu token expired
	time.Sleep(10 * time.Millisecond)

	_, err := s.Verify(pair.AccessToken)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("Verify token expired harus ErrTokenExpired, got: %v", err)
	}
}

// ── Test: ParseRefresh ────────────────────────────────────────────

func TestSanctum_ParseRefresh(t *testing.T) {
	s := newTestSanctum()
	userID, tenantID := newTestIDs()

	pair, _ := s.Invite(userID, tenantID, "test", "e@e.com", nil, "")

	claims, err := s.ParseRefresh(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ParseRefresh() gagal: %v", err)
	}
	if claims.Subject != userID.String() {
		t.Errorf("Subject = %q, want %q", claims.Subject, userID.String())
	}
	if claims.ID == "" {
		t.Error("Refresh token ID tidak boleh kosong")
	}
}

func TestSanctum_ParseRefresh_WithAccessToken(t *testing.T) {
	// Access token tidak boleh bisa di-parse sebagai refresh token
	s := newTestSanctum()
	userID, tenantID := newTestIDs()
	pair, _ := s.Invite(userID, tenantID, "test", "e@e.com", nil, "")

	_, err := s.ParseRefresh(pair.AccessToken)
	if err == nil {
		t.Error("ParseRefresh dengan access token harus error")
	}
}

// ── Test: TokenID digunakan untuk blacklist ───────────────────────

func TestSanctum_Invite_TokenIDInjected(t *testing.T) {
	s := newTestSanctum()
	userID, tenantID := newTestIDs()

	customTokenID := "custom-token-id-123"
	pair, _ := s.Invite(userID, tenantID, "test", "e@e.com", nil, customTokenID)

	claims, _ := s.Verify(pair.AccessToken)
	if claims.TokenID != customTokenID {
		t.Errorf("TokenID = %q, want %q", claims.TokenID, customTokenID)
	}
}

func TestSanctum_Invite_AutoGenerateTokenID(t *testing.T) {
	s := newTestSanctum()
	userID, tenantID := newTestIDs()

	// TokenID kosong → harus di-generate otomatis
	pair, _ := s.Invite(userID, tenantID, "test", "e@e.com", nil, "")
	claims, _ := s.Verify(pair.AccessToken)
	if claims.TokenID == "" {
		t.Error("TokenID harus di-generate otomatis jika tidak diisi")
	}

	// Dua Invite berbeda harus menghasilkan TokenID berbeda
	pair2, _ := s.Invite(userID, tenantID, "test", "e@e.com", nil, "")
	claims2, _ := s.Verify(pair2.AccessToken)
	if claims.TokenID == claims2.TokenID {
		t.Error("TokenID harus unik di setiap Invite()")
	}
}

// ── Test: MockTokenStore ──────────────────────────────────────────

// mockTokenStore adalah in-memory implementation untuk testing.
type mockTokenStore struct {
	revoked     map[string]bool
	revokedUser map[string]bool
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{
		revoked:     make(map[string]bool),
		revokedUser: make(map[string]bool),
	}
}

func (m *mockTokenStore) Revoke(_ context.Context, tokenID string, _ time.Duration) error {
	m.revoked[tokenID] = true
	return nil
}

func (m *mockTokenStore) IsRevoked(_ context.Context, tokenID string) (bool, error) {
	return m.revoked[tokenID], nil
}

func (m *mockTokenStore) RevokeAllForUser(_ context.Context, userID string) error {
	m.revokedUser[userID] = true
	return nil
}

func TestMockTokenStore(t *testing.T) {
	store := newMockTokenStore()
	ctx := context.Background()

	// Revoke
	if err := store.Revoke(ctx, "token-abc", time.Minute); err != nil {
		t.Fatalf("Revoke() gagal: %v", err)
	}

	// IsRevoked — harus true
	revoked, err := store.IsRevoked(ctx, "token-abc")
	if err != nil || !revoked {
		t.Errorf("IsRevoked('token-abc') = %v, %v; want true, nil", revoked, err)
	}

	// IsRevoked — belum di-revoke
	revoked, _ = store.IsRevoked(ctx, "token-xyz")
	if revoked {
		t.Error("IsRevoked('token-xyz') harus false")
	}

	// RevokeAllForUser
	if err := store.RevokeAllForUser(ctx, "user-123"); err != nil {
		t.Fatalf("RevokeAllForUser() gagal: %v", err)
	}
	if !store.revokedUser["user-123"] {
		t.Error("user-123 harus masuk revokedUser")
	}
}

// ── Test: ShadowTokenStore key builders ──────────────────────────

func TestRevokedTokenKey(t *testing.T) {
	key := revokedTokenKey("abc123")
	if key != "revoked:token:abc123" {
		t.Errorf("revokedTokenKey = %q, want revoked:token:abc123", key)
	}
}

func TestRevokedUserKey(t *testing.T) {
	key := revokedUserKey("user-uuid")
	if key != "revoked:user:user-uuid" {
		t.Errorf("revokedUserKey = %q, want revoked:user:user-uuid", key)
	}
}
