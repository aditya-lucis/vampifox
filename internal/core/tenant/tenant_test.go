package tenant

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── Test: ValidateSlug ────────────────────────────────────────────

func TestValidateSlug(t *testing.T) {
	valid := []string{
		"pt-maju-jaya",
		"rs-sehat-123",
		"sekolah-abc",
		"abc",
		"a1b",
		"a-b-c",
	}
	for _, s := range valid {
		if err := ValidateSlug(s); err != nil {
			t.Errorf("ValidateSlug(%q) harus valid, got: %v", s, err)
		}
	}

	invalid := []struct {
		slug string
		desc string
	}{
		{"-invalid", "mulai dengan dash"},
		{"invalid-", "akhir dengan dash"},
		{"ab", "terlalu pendek (< 3)"},
		{"PT Maju", "ada spasi"},
		{"UPPERCASE", "huruf besar"},
		{"with.dot", "ada titik"},
		{"www", "reserved slug"},
		{"api", "reserved slug"},
		{"admin", "reserved slug"},
	}
	for _, tt := range invalid {
		if err := ValidateSlug(tt.slug); err == nil {
			t.Errorf("ValidateSlug(%q) harus invalid (%s)", tt.slug, tt.desc)
		}
	}
}

// ── Test: NormalizeSlug ───────────────────────────────────────────

func TestNormalizeSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"PT Maju Jaya Tbk.", "pt-maju-jaya-tbk"},
		{"RS Sehat Sentosa", "rs-sehat-sentosa"},
		{"already-slug", "already-slug"},
		{"  spasi  di  tepi  ", "spasi-di-tepi"},
		{"Special!@#Chars", "special-chars"},
		{"Angka123", "angka123"},
	}
	for _, tt := range tests {
		got := NormalizeSlug(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── Test: SchemaNameFor ───────────────────────────────────────────

func TestSchemaNameFor(t *testing.T) {
	tests := []struct {
		slug string
		want string
	}{
		{"pt-maju-jaya", "vfx_pt_maju_jaya"},
		{"rs-sehat", "vfx_rs_sehat"},
		{"abc", "vfx_abc"},
		{"a-b-c-d", "vfx_a_b_c_d"},
	}
	for _, tt := range tests {
		got := SchemaNameFor(tt.slug)
		if got != tt.want {
			t.Errorf("SchemaNameFor(%q) = %q, want %q", tt.slug, got, tt.want)
		}
	}
}

// ── Test: Tenant methods ──────────────────────────────────────────

func TestTenant_IsActive(t *testing.T) {
	active := &Tenant{Status: StatusActive}
	if !active.IsActive() {
		t.Error("StatusActive harus IsActive() = true")
	}

	suspended := &Tenant{Status: StatusSuspended}
	if suspended.IsActive() {
		t.Error("StatusSuspended harus IsActive() = false")
	}
}

func TestTenant_ValidateStatus(t *testing.T) {
	tests := []struct {
		name    string
		tenant  *Tenant
		wantErr error
	}{
		{"active", &Tenant{Status: StatusActive}, nil},
		{"suspended", &Tenant{Status: StatusSuspended}, ErrSuspended},
		{"expired status", &Tenant{Status: StatusExpired}, ErrExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tenant.ValidateStatus()
			if err != tt.wantErr {
				t.Errorf("ValidateStatus() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestTenant_Setting(t *testing.T) {
	ten := &Tenant{}

	// Setting pada tenant baru (nil map) tidak boleh panic
	if ten.Setting("key") != nil {
		t.Error("Setting pada nil Settings harus nil")
	}

	ten.SetSetting("timezone", "Asia/Jakarta")
	ten.SetSetting("fiscal_start", "01-01")

	tz, ok := ten.Setting("timezone").(string)
	if !ok || tz != "Asia/Jakarta" {
		t.Errorf("Setting('timezone') = %v, want 'Asia/Jakarta'", ten.Setting("timezone"))
	}
	if ten.Setting("tidak_ada") != nil {
		t.Error("Setting key tidak ada harus nil")
	}
}

// ── Test: Context helpers ─────────────────────────────────────────

func TestWithTenant_FromContext(t *testing.T) {
	original := &Tenant{TenantSlug: "test-tenant", Status: StatusActive}

	ctx := context.Background()
	ctx = WithTenant(ctx, original)

	retrieved, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext harus return ok=true setelah WithTenant")
	}
	if retrieved.TenantSlug != original.TenantSlug {
		t.Errorf("Slug = %q, want %q", retrieved.TenantSlug, original.TenantSlug)
	}
}

func TestFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	_, ok := FromContext(ctx)
	if ok {
		t.Error("FromContext pada context kosong harus return ok=false")
	}
}

func TestMustFromContext_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustFromContext harus panic jika tenant tidak ada di context")
		}
	}()
	MustFromContext(context.Background())
}

// ── Test: isLocalhost ─────────────────────────────────────────────

func TestIsLocalhost(t *testing.T) {
	localHosts := []string{"localhost", "127.0.0.1", "::1"}
	for _, h := range localHosts {
		if !isLocalhost(h) {
			t.Errorf("isLocalhost(%q) harus true", h)
		}
	}

	nonLocal := []string{"example.com", "vampifox.io", "pt-maju.vampifox.io"}
	for _, h := range nonLocal {
		if isLocalhost(h) {
			t.Errorf("isLocalhost(%q) harus false", h)
		}
	}
}

// ── Test: subdomain extraction logic ─────────────────────────────

func TestSubdomainExtraction(t *testing.T) {
	baseDomain := "vampifox.io"
	tests := []struct {
		host    string
		wantSub string
		isMatch bool
	}{
		{"pt-maju.vampifox.io", "pt-maju", true},
		{"rs-sehat.vampifox.io", "rs-sehat", true},
		{"vampifox.io", "", false},
		{"other.com", "", false},
		{"sub.sub.vampifox.io", "", false},
	}

	for _, tt := range tests {
		host := tt.host
		var gotSub string
		var gotMatch bool

		suffix := "." + baseDomain
		if strings.HasSuffix(host, suffix) {
			sub := strings.TrimSuffix(host, suffix)
			if sub != "" && !strings.Contains(sub, ".") {
				gotSub = sub
				gotMatch = true
			}
		}

		if gotMatch != tt.isMatch {
			t.Errorf("host=%q isMatch=%v, want %v", tt.host, gotMatch, tt.isMatch)
		}
		if gotSub != tt.wantSub {
			t.Errorf("host=%q subdomain=%q, want %q", tt.host, gotSub, tt.wantSub)
		}
	}
}

// ── Test: Scope ───────────────────────────────────────────────────

func TestNewScope(t *testing.T) {
	ten := &Tenant{TenantSlug: "pt-maju-jaya", TenantSchema: "vfx_pt_maju_jaya"}
	scope := NewScope(ten)

	if scope.Slug() != "pt-maju-jaya" {
		t.Errorf("Slug() = %q, want pt-maju-jaya", scope.Slug())
	}
	if scope.SchemaName() != "vfx_pt_maju_jaya" {
		t.Errorf("SchemaName() = %q, want vfx_pt_maju_jaya", scope.SchemaName())
	}
}

// ── Test: CreateInput.Validate ────────────────────────────────────

func TestCreateInput_Validate(t *testing.T) {
	// Valid
	input := CreateInput{Name: "PT Maju Jaya", Slug: "pt-maju-jaya"}
	if err := input.Validate(); err != nil {
		t.Errorf("valid input harus tidak error, got: %v", err)
	}
	// Plan default
	if input.Plan != PlanStarter {
		t.Errorf("Plan default harus PlanStarter, got: %q", input.Plan)
	}

	// Tanpa nama
	if err := (&CreateInput{Slug: "valid-slug"}).Validate(); err == nil {
		t.Error("input tanpa name harus error")
	}

	// Slug invalid — huruf besar dan spasi tidak diizinkan
	invalidSlug := "INVALID SLUG"
	if err := (&CreateInput{Name: "Test", Slug: invalidSlug}).Validate(); err == nil {
		t.Error("input dengan slug invalid harus error")
	}

	// Reserved slug — kata-kata yang tidak boleh dipakai
	reservedSlug := "admin"
	if err := (&CreateInput{Name: "Test", Slug: reservedSlug}).Validate(); err == nil {
		t.Error("input dengan reserved slug harus error")
	}
}

// ── Test: planDefaults ────────────────────────────────────────────

func TestPlanDefaults(t *testing.T) {
	tests := []struct {
		plan      Plan
		wantUsers int
		wantGB    int
	}{
		{PlanStarter, 10, 5},
		{PlanGrowth, 50, 20},
		{PlanEnterprise, 500, 100},
		{"unknown", 10, 5}, // fallback ke starter
	}
	for _, tt := range tests {
		users, gb := planDefaults(tt.plan)
		if users != tt.wantUsers || gb != tt.wantGB {
			t.Errorf("planDefaults(%q) = (%d, %d), want (%d, %d)",
				tt.plan, users, gb, tt.wantUsers, tt.wantGB)
		}
	}
}

// suppress unused imports
var _ = http.MethodGet
var _ = httptest.NewRequest
