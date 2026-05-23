// Package rbac — Role-Based Access Control VampiFox.
// "Hierarki vampire": setiap makhluk punya kekuatan yang berbeda.
// VampiFox menggunakan model RBAC dengan permission yang granular.
package rbac

// Role hierarki peran dalam VampiFox.
type Role string

const (
	// RoleOverlord = pemilik tenant, kekuatan penuh
	RoleOverlord Role = "overlord"

	// RoleElderVampire = admin, bisa manage user & modul
	RoleElderVampire Role = "elder_vampire"

	// RoleDaywalker = manager, akses read-write modul tertentu
	RoleDaywalker Role = "daywalker"

	// RoleFamiliar = staff biasa, akses terbatas sesuai modul
	RoleFamiliar Role = "familiar"

	// RoleSpectre = auditor/viewer, hanya bisa read
	RoleSpectre Role = "spectre"
)

// Permission format: "module:action"
// Contoh: "accounting:invoice.create", "inventory:item.delete"
type Permission string

// BloodlineMap mapping role → set of permissions.
// Layaknya "darah" yang diturunkan — setiap role mewarisi permissions.
var BloodlineMap = map[Role][]Permission{
	RoleOverlord: {
		"*:*", // akses total
	},

	RoleElderVampire: {
		"user:*",
		"tenant:settings.*",
		"accounting:*",
		"inventory:*",
		"purchasing:*",
		"sales:*",
		"hrm:*",
		"crm:*",
		"project:*",
		"report:*",
	},

	RoleDaywalker: {
		"accounting:invoice.*",
		"accounting:payment.*",
		"inventory:item.*",
		"inventory:movement.*",
		"sales:order.*",
		"sales:customer.*",
		"purchasing:po.*",
		"report:view",
	},

	RoleFamiliar: {
		"accounting:invoice.create",
		"accounting:invoice.read",
		"inventory:item.read",
		"sales:order.read",
	},

	RoleSpectre: {
		"*:*.read",
		"report:view",
	},
}

// Covenant adalah RBAC engine.
// "Covenant" — perjanjian antara vampire dan bawahannya.
type Covenant struct {
	// userPerms cache: userID → set of permissions
	// Diisi saat user login, disimpan di Shadow (Redis)
}

// CanEnter memeriksa apakah roles user mengizinkan permission tertentu.
func CanEnter(
	roles []string,
	required Permission,
) bool {
	for _, r := range roles {
		perms, ok := BloodlineMap[Role(r)]
		if !ok {
			continue
		}

		for _, p := range perms {
			if p == "*:*" ||
				p == required ||
				matchWildcard(string(p), string(required)) {
				return true
			}
		}
	}

	return false
}

// matchWildcard cek apakah pattern wildcard cocok.
// "accounting:*" → cocok dengan "accounting:invoice.create"
// "accounting:invoice.*" → cocok dengan "accounting:invoice.read"
func matchWildcard(pattern, target string) bool {
	if len(pattern) == 0 {
		return len(target) == 0
	}

	if pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(target) >= len(prefix) &&
			target[:len(prefix)] == prefix
	}

	return pattern == target
}
