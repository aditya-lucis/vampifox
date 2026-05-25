// Package rbac — Role-Based Access Control VampiFox.
//
// "Hierarki vampire" — setiap makhluk dalam kerajaan VampiFox
// memiliki kekuatan yang berbeda, dan kekuatan itu diwariskan
// melalui Bloodline (hierarki role).
//
// Konsep utama:
//
//  1. Role      — jabatan user: overlord, elder_vampire, daywalker, familiar, spectre
//  2. Permission — aksi spesifik: "accounting:invoice.create"
//  3. Bloodline  — mapping role → permissions (built-in)
//  4. Grant      — permission tambahan yang diberikan ke user/role tertentu per-tenant
//  5. Covenant   — engine yang menggabungkan Bloodline + Grant untuk evaluasi akses
//
// Dua layer permission:
//
//	Layer 1 — Bloodline (static, compile-time):
//	  Setiap role punya set permission default yang berlaku untuk semua tenant.
//	  Didefinisikan oleh framework dan module.
//
//	Layer 2 — Grant (dynamic, per-tenant):
//	  Tenant bisa memberikan permission tambahan atau mencabut permission
//	  untuk role atau user tertentu. Disimpan di database tenant.
package rbac

// ═══════════════════════════════════════════════════════════════
//  Role hierarchy
// ═══════════════════════════════════════════════════════════════

// Role adalah jabatan user dalam sebuah tenant.
type Role string

const (
	// RoleOverlord adalah pemilik tenant — kekuatan absolut.
	// Hanya satu per tenant, tidak bisa di-revoke.
	RoleOverlord Role = "overlord"

	// RoleElderVampire adalah administrator tenant.
	// Bisa manage semua user, module, dan konfigurasi.
	RoleElderVampire Role = "elder_vampire"

	// RoleDaywalker adalah manager operasional.
	// Bisa read-write di sebagian besar modul bisnis.
	// "Daywalker" karena bisa bergerak di dua dunia — operasional dan laporan.
	RoleDaywalker Role = "daywalker"

	// RoleFamiliar adalah staff operasional.
	// Akses terbatas hanya ke fungsi yang dibutuhkan sehari-hari.
	// "Familiar" karena seperti asisten vampire — membantu tapi tidak berkuasa.
	RoleFamiliar Role = "familiar"

	// RoleSpectre adalah auditor / view-only user.
	// Hanya bisa membaca data, tidak bisa mengubah apapun.
	// "Spectre" karena seperti hantu — ada tapi tidak bisa menyentuh.
	RoleSpectre Role = "spectre"
)

// AllRoles mengembalikan semua role yang valid.
func AllRoles() []Role {
	return []Role{RoleOverlord, RoleElderVampire, RoleDaywalker, RoleFamiliar, RoleSpectre}
}

// IsValidRole memeriksa apakah string adalah role yang valid.
func IsValidRole(r string) bool {
	for _, valid := range AllRoles() {
		if string(valid) == r {
			return true
		}
	}
	return false
}

// ═══════════════════════════════════════════════════════════════
//  Bloodline — mapping role → permissions (built-in)
// ═══════════════════════════════════════════════════════════════

// Bloodline adalah set permission default untuk setiap role.
// Ini adalah Layer 1 — berlaku untuk semua tenant tanpa konfigurasi.
//
// Module-module (accounting, inventory, dll) akan menambahkan
// permission mereka sendiri saat RegisterModule() dipanggil via
// Covenant.RegisterModulePermissions().
type Bloodline struct {
	permissions map[Role][]Permission
}

// defaultBloodline adalah built-in Bloodline VampiFox.
// Hanya berisi permission Core — permission module didaftarkan saat module di-boot.
var defaultBloodline = &Bloodline{
	permissions: map[Role][]Permission{
		RoleOverlord: {
			"*:*", // akses total
		},
		RoleElderVampire: {
			PermUserAll,
			PermTenantAll,
			"report:*",
		},
		RoleDaywalker: {
			PermUserRead,
			PermUserList,
			PermTenantSettingsRead,
			"report:view",
		},
		RoleFamiliar: {
			PermUserRead,
		},
		RoleSpectre: {
			// Spectre hanya bisa membaca — permission read per-module
			// akan ditambahkan oleh masing-masing module
			"report:view",
		},
	},
}

// ═══════════════════════════════════════════════════════════════
//  Grant — permission tambahan per-tenant (dynamic)
// ═══════════════════════════════════════════════════════════════

// Grant adalah permission tambahan yang diberikan di luar Bloodline.
// Disimpan di database tenant (tabel rbac_grants).
type Grant struct {
	// TargetType menentukan apakah grant ini untuk role atau user spesifik
	TargetType GrantTargetType `gorm:"not null"`
	// TargetID adalah role name atau user UUID
	TargetID   string     `gorm:"not null;index"`
	Permission Permission `gorm:"not null"`
	// Deny jika true, permission ini justru dicabut (explicit deny)
	Deny bool `gorm:"default:false"`
}

// GrantTargetType tipe target grant.
type GrantTargetType string

const (
	GrantTargetRole GrantTargetType = "role"
	GrantTargetUser GrantTargetType = "user"
)

// ═══════════════════════════════════════════════════════════════
//  Covenant — RBAC engine
// ═══════════════════════════════════════════════════════════════

// Covenant adalah engine RBAC VampiFox.
// "Covenant" — perjanjian yang mengatur kekuatan setiap makhluk.
//
// Covenant menggabungkan dua layer permission:
//  1. Bloodline (static) — permission default per role
//  2. Grants (dynamic)   — permission tambahan/deny per tenant
//
// Satu instance Covenant dipakai untuk seluruh aplikasi.
// Grant di-load per-request dari cache atau database.
type Covenant struct {
	bloodline *Bloodline
}

// NewCovenant membuat Covenant baru dengan Bloodline default.
func NewCovenant() *Covenant {
	return &Covenant{
		bloodline: defaultBloodline,
	}
}

// ── Module permission registration ───────────────────────────────

// RegisterModulePermissions mendaftarkan permission tambahan dari sebuah module
// ke Bloodline. Dipanggil saat module di-boot via Module.Boot().
//
// Contoh dari module accounting:
//
//	covenant.RegisterModulePermissions(map[Role][]Permission{
//	    RoleElderVampire: PermCRUD("accounting", "invoice"),
//	    RoleDaywalker:    PermCRUD("accounting", "invoice"),
//	    RoleFamiliar:     {Perm("accounting", "invoice", "create"), Perm("accounting", "invoice", "read")},
//	    RoleSpectre:      {Perm("accounting", "invoice", "read"), Perm("accounting", "invoice", "list")},
//	})
func (c *Covenant) RegisterModulePermissions(perms map[Role][]Permission) {
	for role, ps := range perms {
		c.bloodline.permissions[role] = append(c.bloodline.permissions[role], ps...)
	}
}

// ── Permission checking ───────────────────────────────────────────

// Can memeriksa apakah user dengan roles tertentu bisa melakukan aksi.
// Hanya menggunakan Bloodline (Layer 1) — tidak memerlukan database.
//
// Gunakan ini untuk pemeriksaan cepat tanpa perlu load grants dari DB.
func (c *Covenant) Can(roles []string, required Permission) bool {
	for _, r := range roles {
		if c.canRole(Role(r), required) {
			return true
		}
	}
	return false
}

// CanWithGrants seperti Can tapi juga mempertimbangkan Grants (Layer 2).
// Digunakan oleh Bloodgate middleware yang sudah load grants dari cache/DB.
//
// Urutan evaluasi:
//  1. Explicit deny grant untuk user → false (deny menang)
//  2. Explicit allow grant untuk user → true
//  3. Explicit deny grant untuk role → false
//  4. Explicit allow grant untuk role → true
//  5. Bloodline permission → true/false
func (c *Covenant) CanWithGrants(roles []string, userID string, required Permission, grants []Grant) bool {
	// 1 & 2: User-specific grants (prioritas tertinggi)
	for _, g := range grants {
		if g.TargetType == GrantTargetUser && g.TargetID == userID {
			if g.Permission.Matches(required) {
				if g.Deny {
					return false // explicit deny
				}
				return true // explicit allow
			}
		}
	}

	// 3 & 4: Role-specific grants
	for _, role := range roles {
		for _, g := range grants {
			if g.TargetType == GrantTargetRole && g.TargetID == role {
				if g.Permission.Matches(required) {
					if g.Deny {
						return false
					}
					return true
				}
			}
		}
	}

	// 5: Bloodline (default)
	return c.Can(roles, required)
}

// CanAny memeriksa apakah user memiliki minimal satu dari permission yang diberikan.
// Berguna untuk cek akses ke halaman yang butuh salah satu dari beberapa permission.
func (c *Covenant) CanAny(roles []string, required ...Permission) bool {
	for _, p := range required {
		if c.Can(roles, p) {
			return true
		}
	}
	return false
}

// CanAll memeriksa apakah user memiliki SEMUA permission yang diberikan.
func (c *Covenant) CanAll(roles []string, required ...Permission) bool {
	for _, p := range required {
		if !c.Can(roles, p) {
			return false
		}
	}
	return true
}

// RolePermissions mengembalikan semua permission yang dimiliki sebuah role
// dari Bloodline. Berguna untuk dokumentasi dan UI role management.
func (c *Covenant) RolePermissions(role Role) []Permission {
	return c.bloodline.permissions[role]
}

// ── Internal helpers ──────────────────────────────────────────────

// canRole memeriksa apakah satu role memiliki permission tertentu
// berdasarkan Bloodline saja.
func (c *Covenant) canRole(role Role, required Permission) bool {
	perms, ok := c.bloodline.permissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p.Matches(required) {
			return true
		}
	}
	return false
}
