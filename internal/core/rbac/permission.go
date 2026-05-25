package rbac

import (
	"fmt"
	"strings"
)

// ═══════════════════════════════════════════════════════════════
//  Permission — format dan parsing
// ═══════════════════════════════════════════════════════════════

// Permission adalah string dengan format "module:resource.action"
// atau "module:resource.action.field" untuk field-level.
//
// Format:
//
//	"accounting:invoice.create"          → buat invoice
//	"accounting:invoice.read"            → baca invoice
//	"accounting:invoice.update"          → update invoice
//	"accounting:invoice.delete"          → hapus invoice
//	"accounting:invoice.field.margin"    → akses field margin
//	"accounting:*"                       → semua akses di module accounting
//	"*:*"                                → akses penuh (superadmin)
//
// Konvensi action standar:
//   - create, read, update, delete  → CRUD dasar
//   - list                          → baca list/koleksi
//   - export                        → export data
//   - import                        → import data
//   - approve                       → workflow approval
//   - field.{nama}                  → akses field spesifik
type Permission string

// ParsedPermission adalah hasil parsing Permission string.
type ParsedPermission struct {
	Module   string // e.g. "accounting"
	Resource string // e.g. "invoice"
	Action   string // e.g. "create", "read", "field"
	Field    string // e.g. "margin" (hanya jika action == "field")
	Raw      string // string asli sebelum di-parse
}

// Parse memecah Permission string menjadi ParsedPermission.
//
// Format yang didukung:
//
//	"*:*"                            → {Module:"*", Resource:"*", Action:"*"}
//	"module:*"                       → {Module:"module", Resource:"*", Action:"*"}
//	"module:resource.*"              → {Module:"module", Resource:"resource", Action:"*"}
//	"module:resource.action"         → {Module:"module", Resource:"resource", Action:"action"}
//	"module:resource.field.fieldname"→ {Module:"module", Resource:"resource", Action:"field", Field:"fieldname"}
func (p Permission) Parse() (ParsedPermission, error) {
	raw := string(p)
	result := ParsedPermission{Raw: raw}

	// Split module:rest
	colonIdx := strings.Index(raw, ":")
	if colonIdx < 0 {
		return result, fmt.Errorf("permission '%s' tidak valid: harus mengandung ':'", raw)
	}

	result.Module = raw[:colonIdx]
	rest := raw[colonIdx+1:]

	// Wildcard total
	if result.Module == "*" && rest == "*" {
		result.Resource = "*"
		result.Action = "*"
		return result, nil
	}

	// Split resource.action
	dotIdx := strings.Index(rest, ".")
	if dotIdx < 0 {
		// Hanya module:resource tanpa action → anggap wildcard action
		result.Resource = rest
		result.Action = "*"
		return result, nil
	}

	result.Resource = rest[:dotIdx]
	actionPart := rest[dotIdx+1:]

	// Field-level: resource.field.fieldname
	if strings.HasPrefix(actionPart, "field.") {
		result.Action = "field"
		result.Field = strings.TrimPrefix(actionPart, "field.")
		return result, nil
	}

	result.Action = actionPart
	return result, nil
}

// String mengembalikan representasi string Permission.
func (p Permission) String() string { return string(p) }

// ── Permission builder helpers ────────────────────────────────────

// Perm membangun Permission string dari parts.
//
//	Perm("accounting", "invoice", "create")          → "accounting:invoice.create"
//	Perm("accounting", "invoice", "field", "margin") → "accounting:invoice.field.margin"
func Perm(module, resource string, actionParts ...string) Permission {
	base := module + ":" + resource
	if len(actionParts) == 0 {
		return Permission(base + ".*")
	}
	return Permission(base + "." + strings.Join(actionParts, "."))
}

// PermAll membuat wildcard permission untuk seluruh module.
//
//	PermAll("accounting") → "accounting:*"
func PermAll(module string) Permission {
	return Permission(module + ":*")
}

// PermCRUD mengembalikan semua permission CRUD standar untuk resource.
//
//	PermCRUD("accounting", "invoice") → [
//	    "accounting:invoice.create",
//	    "accounting:invoice.read",
//	    "accounting:invoice.update",
//	    "accounting:invoice.delete",
//	    "accounting:invoice.list",
//	]
func PermCRUD(module, resource string) []Permission {
	return []Permission{
		Perm(module, resource, "create"),
		Perm(module, resource, "read"),
		Perm(module, resource, "update"),
		Perm(module, resource, "delete"),
		Perm(module, resource, "list"),
	}
}

// ═══════════════════════════════════════════════════════════════
//  Matching
// ═══════════════════════════════════════════════════════════════

// Matches memeriksa apakah pattern permission ini cocok dengan required.
//
// Aturan matching (dari yang paling luas ke paling spesifik):
//
//	"*:*"                 cocok dengan apapun
//	"accounting:*"        cocok dengan semua permission di module accounting
//	"accounting:invoice.*"cocok dengan semua action di invoice
//	"accounting:invoice.create" hanya cocok persis
func (p Permission) Matches(required Permission) bool {
	if p == required {
		return true
	}

	ps := string(p)
	rs := string(required)

	// Wildcard total
	if ps == "*:*" {
		return true
	}

	// Cek dengan suffix wildcard: "module:*" atau "module:resource.*"
	if strings.HasSuffix(ps, ":*") {
		prefix := strings.TrimSuffix(ps, ":*")
		return strings.HasPrefix(rs, prefix+":")
	}

	if strings.HasSuffix(ps, ".*") {
		prefix := strings.TrimSuffix(ps, ".*")
		return rs == prefix || strings.HasPrefix(rs, prefix+".")
	}

	return false
}

// ═══════════════════════════════════════════════════════════════
//  Standard permissions — konstanta yang dipakai seluruh framework
// ═══════════════════════════════════════════════════════════════

// Permission-permission standar yang didefinisikan oleh VampiFox Core.
// Module-module tambahan mendefinisikan permission-nya sendiri.
const (
	// User management
	PermUserCreate = Permission("user:user.create")
	PermUserRead   = Permission("user:user.read")
	PermUserUpdate = Permission("user:user.update")
	PermUserDelete = Permission("user:user.delete")
	PermUserList   = Permission("user:user.list")
	PermUserAll    = Permission("user:*")

	// Tenant settings
	PermTenantSettingsRead   = Permission("tenant:settings.read")
	PermTenantSettingsUpdate = Permission("tenant:settings.update")
	PermTenantAll            = Permission("tenant:*")

	// Role management
	PermRoleAssign = Permission("user:role.assign")
	PermRoleRevoke = Permission("user:role.revoke")
)
