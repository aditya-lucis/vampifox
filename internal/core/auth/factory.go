package auth

import (
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/core/user"
	"github.com/aditya-lucis/vampifox/internal/fangs"
)

// ═══════════════════════════════════════════════════════════════
//  ServiceFactory — buat auth.Service per-tenant
// ═══════════════════════════════════════════════════════════════

// ServiceFactory membuat auth.Service yang sudah di-scope
// ke tenant tertentu. Di-buat sekali saat startup, dipakai
// berkali-kali per-request.
type ServiceFactory struct {
	sanctum     *Sanctum
	tokenStore  TokenStore
	userFactory *user.ServiceFactory
	logger      *zap.Logger
}

// NewServiceFactory membuat ServiceFactory baru.
func NewServiceFactory(
	sanctum *Sanctum,
	tokenStore TokenStore,
	userFactory *user.ServiceFactory,
	logger *zap.Logger,
) *ServiceFactory {
	return &ServiceFactory{
		sanctum:     sanctum,
		tokenStore:  tokenStore,
		userFactory: userFactory,
		logger:      logger.Named("auth.factory"),
	}
}

// ForTenant membuat auth.Service yang siap dipakai untuk tenant tertentu.
//
// Dipanggil per-request dari /auth/* handlers.
// Tidak ada I/O — hanya struct construction dari pool yang sudah ada.
//
//	// Di auth handler:
//	t := middleware.GetTenant(c)
//	authSvc := authFactory.ForTenant(t)
//	result, err := authSvc.Login(ctx, t.ID, t.Slug, input)
func (f *ServiceFactory) ForTenant(t fangs.TenantScope) *Service {
	userSvc := f.userFactory.ForTenant(t)
	return NewService(userSvc, f.sanctum, f.tokenStore, f.logger)
}
