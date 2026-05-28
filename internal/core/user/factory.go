package user

import (
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/fangs"
	"github.com/aditya-lucis/vampifox/internal/shadow"
)

// ═══════════════════════════════════════════════════════════════
//  ServiceFactory — buat user.Service per-tenant
// ═══════════════════════════════════════════════════════════════

// ServiceFactory membuat user.Service yang sudah di-scope
// ke schema database tenant yang benar.
//
// Di-buat sekali saat startup (singleton factory),
// tapi setiap ForTenant() menghasilkan Service baru dengan
// koneksi DB yang di-scope ke tenant yang tepat.
type ServiceFactory struct {
	fangs      *fangs.Fangs
	shadow     *shadow.Shadow
	bcryptCost int
	logger     *zap.Logger
}

// NewServiceFactory membuat ServiceFactory baru.
func NewServiceFactory(
	f *fangs.Fangs,
	sh *shadow.Shadow,
	bcryptCost int,
	logger *zap.Logger,
) *ServiceFactory {
	return &ServiceFactory{
		fangs:      f,
		shadow:     sh,
		bcryptCost: bcryptCost,
		logger:     logger,
	}
}

// ForTenant membuat user.Service yang sudah di-scope ke tenant tertentu.
//
// Dipanggil per-request dari handler — ringan karena hanya
// membuat struct baru, tidak ada I/O atau koneksi baru.
// Koneksi database diambil dari pool yang sudah ada di Fangs.
//
//	// Di handler:
//	t := middleware.GetTenant(c)
//	userSvc := userFactory.ForTenant(t)
//	user, err := userSvc.Register(ctx, input, roles)
func (f *ServiceFactory) ForTenant(t fangs.TenantScope) *Service {
	// Scope DB ke schema tenant
	tenantDB := f.fangs.For(t)

	// Scope cache ke namespace tenant
	tenantShadow := f.shadow.ForTenant(t.Slug())

	// Buat repository dengan DB + cache yang sudah di-scope
	repo := NewRepository(tenantDB, tenantShadow, f.logger)

	return NewService(repo, f.bcryptCost, f.logger)
}

// ForTenant menggunakan fangs.TenantScope — interface yang sama
// dipakai oleh fangs.For() sehingga *tenant.Tenant bisa langsung dipakai.
