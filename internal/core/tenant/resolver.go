package tenant

import (
	"context"
	"net/http"
	"strings"
)

// ═══════════════════════════════════════════════════════════════
//  Resolver — identifikasi tenant dari HTTP request
// ═══════════════════════════════════════════════════════════════

// Resolver mengidentifikasi tenant dari sebuah HTTP request.
//
// VampiFox mendukung tiga strategi resolusi, dicoba berurutan:
//
//  1. Header X-VampiFox-Tenant: pt-maju-jaya
//     → Cocok untuk API calls dari frontend/mobile
//
//  2. Subdomain: pt-maju-jaya.vampifox.io
//     → Cocok untuk web app dengan custom subdomain
//
//  3. Custom domain: erp.pt-maju.com
//     → Cocok untuk tenant yang punya domain sendiri
//
// Jika ketiga strategi gagal, request ditolak.
type Resolver struct {
	svc        *Service
	baseDomain string // e.g. "vampifox.io" — untuk deteksi subdomain
}

// NewResolver membuat Resolver baru.
//
// baseDomain adalah domain utama VampiFox, e.g. "vampifox.io".
// Digunakan untuk membedakan subdomain tenant dari domain lain.
func NewResolver(svc *Service, baseDomain string) *Resolver {
	return &Resolver{
		svc:        svc,
		baseDomain: strings.ToLower(strings.TrimPrefix(baseDomain, ".")),
	}
}

// ── ResolveResult ─────────────────────────────────────────────────

// ResolveResult adalah hasil resolusi tenant dari request.
type ResolveResult struct {
	Tenant   *Tenant
	Strategy string // "header" | "subdomain" | "domain" — untuk logging/debugging
}

// ── Resolve ───────────────────────────────────────────────────────

// Resolve mengidentifikasi dan memuat tenant dari HTTP request.
//
// Mengembalikan ErrNotFound jika tidak ada identifikasi tenant,
// ErrSuspended/ErrExpired jika tenant tidak aktif.
func (r *Resolver) Resolve(ctx context.Context, req *http.Request) (*ResolveResult, error) {
	// ── Strategi 1: Header eksplisit ─────────────────────────────
	if slug := req.Header.Get("X-VampiFox-Tenant"); slug != "" {
		slug = strings.ToLower(strings.TrimSpace(slug))
		t, err := r.svc.FindBySlug(ctx, slug)
		if err != nil {
			return nil, err
		}
		return &ResolveResult{Tenant: t, Strategy: "header"}, nil
	}

	host := strings.ToLower(req.Host)
	// Hilangkan port jika ada (e.g. "localhost:8080" → "localhost")
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// ── Strategi 2: Subdomain ─────────────────────────────────────
	if r.baseDomain != "" && strings.HasSuffix(host, "."+r.baseDomain) {
		subdomain := strings.TrimSuffix(host, "."+r.baseDomain)
		// Pastikan ini benar-benar subdomain langsung (bukan sub-sub)
		if subdomain != "" && !strings.Contains(subdomain, ".") {
			t, err := r.svc.FindBySlug(ctx, subdomain)
			if err != nil {
				return nil, err
			}
			return &ResolveResult{Tenant: t, Strategy: "subdomain"}, nil
		}
	}

	// ── Strategi 3: Custom domain ─────────────────────────────────
	// Jika host bukan subdomain dari baseDomain kita, coba sebagai custom domain
	if host != "" && host != r.baseDomain && !isLocalhost(host) {
		t, err := r.svc.FindByDomain(ctx, host)
		if err != nil {
			if isNotFoundErr(err) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		return &ResolveResult{Tenant: t, Strategy: "domain"}, nil
	}

	return nil, ErrNotFound
}

// ResolveFromSlug adalah shortcut untuk resolve tenant langsung dari slug.
// Berguna di foxctl dan testing tanpa perlu membuat *http.Request.
func (r *Resolver) ResolveFromSlug(ctx context.Context, slug string) (*Tenant, error) {
	return r.svc.FindBySlug(ctx, slug)
}

// ── Helpers ───────────────────────────────────────────────────────

// isLocalhost memeriksa apakah host adalah localhost/127.0.0.1.
func isLocalhost(host string) bool {
	return host == "localhost" ||
		host == "127.0.0.1" ||
		host == "::1" ||
		strings.HasPrefix(host, "localhost.")
}

// isNotFoundErr memeriksa apakah error adalah ErrNotFound.
func isNotFoundErr(err error) bool {
	return err == ErrNotFound
}

// ═══════════════════════════════════════════════════════════════
//  TenantScope adapter — agar *Tenant bisa dipakai di fangs.For()
// ═══════════════════════════════════════════════════════════════

// Scope adalah adapter yang mengimplementasikan fangs.TenantScope.
// Memungkinkan *Tenant digunakan langsung di fangs.For(scope).
type Scope struct {
	slug       string
	schemaName string
}

// NewScope membuat Scope dari *Tenant.
func NewScope(t *Tenant) *Scope {
	return &Scope{
		slug:       t.Slug,
		schemaName: t.SchemaName,
	}
}

// TenantSlug memenuhi fangs.TenantScope.
func (s *Scope) TenantSlug() string { return s.slug }

// SchemaName memenuhi fangs.TenantScope.
func (s *Scope) SchemaName() string { return s.schemaName }

// ScopeFromContext adalah shortcut — ambil Tenant dari context
// dan langsung konversi ke Scope untuk fangs.For().
//
//	db := fangs.For(tenant.ScopeFromContext(ctx))
func ScopeFromContext(ctx context.Context) *Scope {
	t := MustFromContext(ctx)
	return NewScope(t)
}
