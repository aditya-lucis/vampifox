package rbac

import (
	"testing"
)

// ── Test: Permission.Parse ────────────────────────────────────────

func TestPermission_Parse(t *testing.T) {
	tests := []struct {
		raw     string
		wantMod string
		wantRes string
		wantAct string
		wantFld string
		wantErr bool
	}{
		{"*:*", "*", "*", "*", "", false},
		{"accounting:*", "accounting", "*", "*", "", false},
		{"accounting:invoice.*", "accounting", "invoice", "*", "", false},
		{"accounting:invoice.create", "accounting", "invoice", "create", "", false},
		{"accounting:invoice.read", "accounting", "invoice", "read", "", false},
		{"accounting:invoice.field.margin", "accounting", "invoice", "field", "margin", false},
		{"user:user.list", "user", "user", "list", "", false},
		// Error cases
		{"no-colon", "", "", "", "", true},
		{"", "", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			p := Permission(tt.raw)
			parsed, err := p.Parse()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse(%q) harus error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) gagal: %v", tt.raw, err)
			}
			if parsed.Module != tt.wantMod {
				t.Errorf("Module = %q, want %q", parsed.Module, tt.wantMod)
			}
			if parsed.Resource != tt.wantRes {
				t.Errorf("Resource = %q, want %q", parsed.Resource, tt.wantRes)
			}
			if parsed.Action != tt.wantAct {
				t.Errorf("Action = %q, want %q", parsed.Action, tt.wantAct)
			}
			if parsed.Field != tt.wantFld {
				t.Errorf("Field = %q, want %q", parsed.Field, tt.wantFld)
			}
		})
	}
}

// ── Test: Permission.Matches ──────────────────────────────────────

func TestPermission_Matches(t *testing.T) {
	tests := []struct {
		pattern  string
		required string
		want     bool
	}{
		// Wildcard total
		{"*:*", "accounting:invoice.create", true},
		{"*:*", "anything:any.action", true},

		// Module wildcard
		{"accounting:*", "accounting:invoice.create", true},
		{"accounting:*", "accounting:payment.read", true},
		{"accounting:*", "inventory:item.read", false},

		// Resource wildcard
		{"accounting:invoice.*", "accounting:invoice.create", true},
		{"accounting:invoice.*", "accounting:invoice.read", true},
		{"accounting:invoice.*", "accounting:payment.create", false},

		// Exact match
		{"accounting:invoice.create", "accounting:invoice.create", true},
		{"accounting:invoice.create", "accounting:invoice.read", false},
		{"accounting:invoice.create", "accounting:payment.create", false},

		// Field-level
		{"accounting:invoice.*", "accounting:invoice.field.margin", true},
		{"accounting:invoice.field.margin", "accounting:invoice.field.margin", true},
		{"accounting:invoice.field.margin", "accounting:invoice.field.discount", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"~"+tt.required, func(t *testing.T) {
			p := Permission(tt.pattern)
			got := p.Matches(Permission(tt.required))
			if got != tt.want {
				t.Errorf("Permission(%q).Matches(%q) = %v, want %v",
					tt.pattern, tt.required, got, tt.want)
			}
		})
	}
}

// ── Test: Perm builder ────────────────────────────────────────────

func TestPerm(t *testing.T) {
	tests := []struct {
		module   string
		resource string
		actions  []string
		want     string
	}{
		{"accounting", "invoice", []string{"create"}, "accounting:invoice.create"},
		{"inventory", "item", []string{"read"}, "inventory:item.read"},
		{"accounting", "invoice", []string{"field", "margin"}, "accounting:invoice.field.margin"},
		{"accounting", "invoice", nil, "accounting:invoice.*"},
	}
	for _, tt := range tests {
		got := Perm(tt.module, tt.resource, tt.actions...)
		if string(got) != tt.want {
			t.Errorf("Perm(%q, %q, %v) = %q, want %q",
				tt.module, tt.resource, tt.actions, got, tt.want)
		}
	}
}

func TestPermAll(t *testing.T) {
	got := PermAll("accounting")
	if string(got) != "accounting:*" {
		t.Errorf("PermAll('accounting') = %q, want accounting:*", got)
	}
}

func TestPermCRUD(t *testing.T) {
	perms := PermCRUD("accounting", "invoice")
	if len(perms) != 5 {
		t.Errorf("PermCRUD harus return 5 permissions, got %d", len(perms))
	}

	expected := []string{
		"accounting:invoice.create",
		"accounting:invoice.read",
		"accounting:invoice.update",
		"accounting:invoice.delete",
		"accounting:invoice.list",
	}
	for i, exp := range expected {
		if string(perms[i]) != exp {
			t.Errorf("perms[%d] = %q, want %q", i, perms[i], exp)
		}
	}
}

// ── Test: IsValidRole ─────────────────────────────────────────────

func TestIsValidRole(t *testing.T) {
	valid := []string{"overlord", "elder_vampire", "daywalker", "familiar", "spectre"}
	for _, r := range valid {
		if !IsValidRole(r) {
			t.Errorf("IsValidRole(%q) harus true", r)
		}
	}
	invalid := []string{"superadmin", "god", "", "admin", "owner"}
	for _, r := range invalid {
		if IsValidRole(r) {
			t.Errorf("IsValidRole(%q) harus false", r)
		}
	}
}

// ── Test: Covenant.Can ────────────────────────────────────────────

func TestCovenant_Can_Overlord(t *testing.T) {
	c := NewCovenant()

	// Overlord bisa melakukan apapun
	testPerms := []Permission{
		"accounting:invoice.create",
		"inventory:item.delete",
		"user:user.create",
		"apapun:apapun.apapun",
	}
	for _, p := range testPerms {
		if !c.Can([]string{"overlord"}, p) {
			t.Errorf("Overlord harus bisa %q", p)
		}
	}
}

func TestCovenant_Can_ElderVampire(t *testing.T) {
	c := NewCovenant()

	// Elder vampire bisa manage user
	if !c.Can([]string{"elder_vampire"}, PermUserCreate) {
		t.Error("ElderVampire harus bisa PermUserCreate")
	}
	if !c.Can([]string{"elder_vampire"}, PermUserDelete) {
		t.Error("ElderVampire harus bisa PermUserDelete")
	}
	if !c.Can([]string{"elder_vampire"}, PermTenantSettingsUpdate) {
		t.Error("ElderVampire harus bisa PermTenantSettingsUpdate")
	}
}

func TestCovenant_Can_Familiar_Limited(t *testing.T) {
	c := NewCovenant()

	// Familiar hanya bisa read user
	if !c.Can([]string{"familiar"}, PermUserRead) {
		t.Error("Familiar harus bisa PermUserRead")
	}

	// Familiar tidak bisa delete user
	if c.Can([]string{"familiar"}, PermUserDelete) {
		t.Error("Familiar tidak boleh bisa PermUserDelete")
	}

	// Familiar tidak bisa akses tenant settings
	if c.Can([]string{"familiar"}, PermTenantSettingsUpdate) {
		t.Error("Familiar tidak boleh akses PermTenantSettingsUpdate")
	}
}

func TestCovenant_Can_MultipleRoles(t *testing.T) {
	c := NewCovenant()

	// User dengan banyak role — cukup satu yang match
	roles := []string{"familiar", "daywalker"}

	// Daywalker bisa read report
	if !c.Can(roles, "report:view") {
		t.Error("Daywalker di multi-role harus bisa report:view")
	}
}

func TestCovenant_Can_EmptyRoles(t *testing.T) {
	c := NewCovenant()
	if c.Can([]string{}, "accounting:invoice.create") {
		t.Error("User tanpa role tidak boleh bisa apapun")
	}
}

func TestCovenant_Can_UnknownRole(t *testing.T) {
	c := NewCovenant()
	if c.Can([]string{"role-tidak-ada"}, "accounting:invoice.create") {
		t.Error("Role tidak dikenal tidak boleh memberikan permission")
	}
}

// ── Test: Covenant.CanAny & CanAll ───────────────────────────────

func TestCovenant_CanAny(t *testing.T) {
	c := NewCovenant()
	roles := []string{"familiar"}

	// Familiar bisa read user — CanAny harus true jika salah satu match
	result := c.CanAny(roles,
		PermUserDelete, // tidak bisa
		PermUserRead,   // bisa
	)
	if !result {
		t.Error("CanAny harus true jika salah satu permission match")
	}

	// Familiar tidak bisa keduanya
	result = c.CanAny(roles,
		PermUserDelete,
		PermTenantSettingsUpdate,
	)
	if result {
		t.Error("CanAny harus false jika tidak ada yang match")
	}
}

func TestCovenant_CanAll(t *testing.T) {
	c := NewCovenant()

	// Elder vampire bisa semua user operations
	result := c.CanAll([]string{"elder_vampire"},
		PermUserCreate,
		PermUserRead,
		PermUserUpdate,
		PermUserDelete,
	)
	if !result {
		t.Error("ElderVampire harus bisa semua user permissions (CanAll)")
	}

	// Familiar tidak bisa semua
	result = c.CanAll([]string{"familiar"},
		PermUserCreate,
		PermUserRead,
	)
	if result {
		t.Error("Familiar tidak seharusnya bisa CanAll user create+read")
	}
}

// ── Test: CanWithGrants ───────────────────────────────────────────

func TestCovenant_CanWithGrants_UserAllow(t *testing.T) {
	c := NewCovenant()

	// Grant khusus untuk user ini
	grants := []Grant{
		{
			TargetType: GrantTargetUser,
			TargetID:   "user-abc",
			Permission: "accounting:invoice.create",
		},
	}

	// User familiar + grant khusus → bisa create invoice
	can := c.CanWithGrants(
		[]string{"familiar"}, "user-abc",
		"accounting:invoice.create",
		grants,
	)
	if !can {
		t.Error("User dengan grant spesifik harus bisa melakukan aksi tersebut")
	}
}

func TestCovenant_CanWithGrants_UserDeny(t *testing.T) {
	c := NewCovenant()

	// Deny khusus untuk user ini
	grants := []Grant{
		{
			TargetType: GrantTargetUser,
			TargetID:   "user-restricted",
			Permission: "report:view",
			Deny:       true,
		},
	}

	// Daywalker normalnya bisa view report, tapi user ini di-deny
	can := c.CanWithGrants(
		[]string{"daywalker"}, "user-restricted",
		"report:view",
		grants,
	)
	if can {
		t.Error("Explicit deny harus menang atas Bloodline permission")
	}
}

func TestCovenant_CanWithGrants_RoleAllow(t *testing.T) {
	c := NewCovenant()

	// Grant untuk role familiar
	grants := []Grant{
		{
			TargetType: GrantTargetRole,
			TargetID:   "familiar",
			Permission: "accounting:invoice.create",
		},
	}

	can := c.CanWithGrants(
		[]string{"familiar"}, "any-user",
		"accounting:invoice.create",
		grants,
	)
	if !can {
		t.Error("Role grant harus memberikan permission ke semua user dengan role tersebut")
	}
}

func TestCovenant_CanWithGrants_NoGrants(t *testing.T) {
	c := NewCovenant()

	// Tanpa grants — fallback ke Bloodline
	can := c.CanWithGrants(
		[]string{"overlord"}, "user-id",
		"accounting:invoice.create",
		nil,
	)
	if !can {
		t.Error("Tanpa grants, Overlord harus tetap bisa berdasarkan Bloodline")
	}
}

// ── Test: RegisterModulePermissions ──────────────────────────────

func TestCovenant_RegisterModulePermissions(t *testing.T) {
	c := NewCovenant()

	// Sebelum register — familiar tidak bisa accounting:invoice.create
	if c.Can([]string{"familiar"}, "accounting:invoice.create") {
		t.Error("Sebelum RegisterModulePermissions, familiar tidak boleh bisa invoice.create")
	}

	// Register permission dari module accounting
	c.RegisterModulePermissions(map[Role][]Permission{
		RoleFamiliar: {
			Perm("accounting", "invoice", "create"),
			Perm("accounting", "invoice", "read"),
		},
	})

	// Setelah register — familiar bisa
	if !c.Can([]string{"familiar"}, "accounting:invoice.create") {
		t.Error("Setelah RegisterModulePermissions, familiar harus bisa invoice.create")
	}
	if !c.Can([]string{"familiar"}, "accounting:invoice.read") {
		t.Error("Setelah RegisterModulePermissions, familiar harus bisa invoice.read")
	}

	// Tapi masih tidak bisa delete
	if c.Can([]string{"familiar"}, "accounting:invoice.delete") {
		t.Error("Familiar tidak boleh bisa invoice.delete tanpa grant eksplisit")
	}
}

// ── Test: RolePermissions ─────────────────────────────────────────

func TestCovenant_RolePermissions(t *testing.T) {
	c := NewCovenant()

	// Overlord harus punya "*:*"
	perms := c.RolePermissions(RoleOverlord)
	found := false
	for _, p := range perms {
		if string(p) == "*:*" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Overlord harus punya permission '*:*'")
	}

	// Role tidak dikenal harus return nil/empty
	perms = c.RolePermissions("role-tidak-ada")
	if len(perms) > 0 {
		t.Errorf("Role tidak dikenal harus return empty, got %d permissions", len(perms))
	}
}
