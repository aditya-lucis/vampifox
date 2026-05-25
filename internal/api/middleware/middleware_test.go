package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/core/auth"
	"github.com/aditya-lucis/vampifox/internal/core/rbac"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ── Helpers ───────────────────────────────────────────────────────

func newTestLogger() *zap.Logger { return zap.NewNop() }

func newMockClaims(roles []string) auth.BloodClaims {
	return auth.BloodClaims{
		UserID:     uuid.New(),
		TenantID:   uuid.New(),
		TenantSlug: "test-tenant",
		Email:      "test@example.com",
		Roles:      roles,
	}
}

// injectClaims adalah middleware mock pengganti Bloodgate untuk testing.
func injectClaims(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := newMockClaims(roles)
		c.Set(KeyBloodClaims, &claims)
		c.Next()
	}
}

// ── Test: RequestID ───────────────────────────────────────────────

func TestRequestID_GeneratesID(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())

	var capturedID string
	r.GET("/test", func(c *gin.Context) {
		capturedID = GetRequestID(c)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if capturedID == "" {
		t.Error("RequestID tidak boleh kosong")
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID harus ada di response header")
	}
}

func TestRequestID_UseClientID(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())

	clientID := "550e8400-e29b-41d4-a716-446655440000"
	var capturedID string
	r.GET("/test", func(c *gin.Context) {
		capturedID = GetRequestID(c)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", clientID)
	r.ServeHTTP(w, req)

	if capturedID != clientID {
		t.Errorf("RequestID = %q, want %q", capturedID, clientID)
	}
}

func TestRequestID_InvalidClientID(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())

	var capturedID string
	r.GET("/test", func(c *gin.Context) {
		capturedID = GetRequestID(c)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "bukan-uuid-valid")
	r.ServeHTTP(w, req)

	if len(capturedID) < 4 || capturedID[:4] != "vfx-" {
		t.Errorf("ID invalid harus generate baru dengan prefix 'vfx-', got: %q", capturedID)
	}
}

// ── Test: Recovery ────────────────────────────────────────────────

func TestRecovery_CatchesPanic(t *testing.T) {
	r := gin.New()
	r.Use(RequestID(), Recovery(newTestLogger()))
	r.GET("/panic", func(c *gin.Context) { panic("test panic!") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Recovery harus return 500, got %d", w.Code)
	}
}

func TestRecovery_NormalRequest(t *testing.T) {
	r := gin.New()
	r.Use(RequestID(), Recovery(newTestLogger()))
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))

	if w.Code != http.StatusOK {
		t.Errorf("Request normal harus 200, got %d", w.Code)
	}
}

// ── Test: extractBearerToken ──────────────────────────────────────

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		header  string
		want    string
		wantErr bool
	}{
		{"Bearer mytoken123", "mytoken123", false},
		{"Bearer  spaced  ", "spaced", false},
		{"", "", true},
		{"Basic credentials", "", true},
		{"Bearer", "", true},
		{"Bearer ", "", true},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if tt.header != "" {
			req.Header.Set("Authorization", tt.header)
		}

		got, err := extractBearerToken(req)
		if (err != nil) != tt.wantErr {
			t.Errorf("header=%q: wantErr=%v got err=%v", tt.header, tt.wantErr, err)
		}
		if got != tt.want {
			t.Errorf("header=%q: token=%q, want=%q", tt.header, got, tt.want)
		}
	}
}

// ── Test: context accessors ───────────────────────────────────────

func TestGetBloodClaims_Nil(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if GetBloodClaims(c) != nil {
		t.Error("GetBloodClaims tanpa Bloodgate harus nil")
	}
}

func TestGetUserID_Empty(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if GetUserID(c) != "" {
		t.Error("GetUserID tanpa Bloodgate harus kosong")
	}
}

func TestGetRoles_Nil(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if GetRoles(c) != nil {
		t.Error("GetRoles tanpa Bloodgate harus nil")
	}
}

// ── Test: CovenantRole ────────────────────────────────────────────

func TestCovenantRole_Allowed(t *testing.T) {
	r := gin.New()
	r.GET("/admin",
		injectClaims("elder_vampire"),
		CovenantRole(rbac.RoleElderVampire, rbac.RoleOverlord),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if w.Code != http.StatusOK {
		t.Errorf("CovenantRole allowed harus 200, got %d", w.Code)
	}
}

func TestCovenantRole_Denied(t *testing.T) {
	r := gin.New()
	r.GET("/admin",
		injectClaims("familiar"),
		CovenantRole(rbac.RoleElderVampire, rbac.RoleOverlord),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if w.Code != http.StatusForbidden {
		t.Errorf("CovenantRole denied harus 403, got %d", w.Code)
	}
}

func TestCovenantRole_NoClaims(t *testing.T) {
	r := gin.New()
	r.GET("/admin",
		// Tidak ada injectClaims — simulasi tanpa Bloodgate
		CovenantRole(rbac.RoleElderVampire),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("CovenantRole tanpa claims harus 401, got %d", w.Code)
	}
}

// ── Test: isValidUUID ─────────────────────────────────────────────

func TestIsValidUUID(t *testing.T) {
	valid := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
	}
	for _, u := range valid {
		if !isValidUUID(u) {
			t.Errorf("isValidUUID(%q) harus true", u)
		}
	}

	invalid := []string{"", "bukan-uuid", "123", "vfx-abc"}
	for _, u := range invalid {
		if isValidUUID(u) {
			t.Errorf("isValidUUID(%q) harus false", u)
		}
	}
}

// ── Test: newErrorResponse ────────────────────────────────────────

func TestNewErrorResponse(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	resp := newErrorResponse(c, "TEST_CODE", "Pesan test")

	if resp.Success {
		t.Error("ErrorResponse.Success harus false")
	}
	if resp.Error.Code != "TEST_CODE" {
		t.Errorf("Code = %q, want TEST_CODE", resp.Error.Code)
	}
	if resp.Error.Message != "Pesan test" {
		t.Errorf("Message = %q, want 'Pesan test'", resp.Error.Message)
	}
}
