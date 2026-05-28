package den

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/core/auth"
	"github.com/aditya-lucis/vampifox/internal/core/rbac"
	"github.com/aditya-lucis/vampifox/internal/core/tenant"
	"github.com/aditya-lucis/vampifox/internal/core/user"
	"github.com/aditya-lucis/vampifox/internal/fangs"
	"github.com/aditya-lucis/vampifox/internal/shadow"
)

// Services menyimpan semua singleton service dan factory.
type Services struct {
	Fangs          *fangs.Fangs
	Shadow         *shadow.Shadow
	TenantRepo     *tenant.Repository
	TenantService  *tenant.Service
	TenantResolver *tenant.Resolver
	TokenValidator *auth.TokenValidator
	AuthFactory    *auth.ServiceFactory
	UserFactory    *user.ServiceFactory
	Covenant       *rbac.Covenant
}

// wire menginisialisasi semua service dalam urutan dependency yang benar.
func (d *Den) wire() (*Services, error) {
	cfg    := d.cfg
	logger := d.logger

	// 1. Fangs (database)
	logger.Info("Wiring Fangs...", zap.String("driver", string(cfg.Fangs.Driver)))
	f, err := fangs.New(cfg.Fangs, logger)
	if err != nil {
		return nil, fmt.Errorf("Fangs gagal: %w", err)
	}

	// 2. Shadow (cache)
	logger.Info("Wiring Shadow...", zap.String("addr", cfg.Shadow.Addr))
	sh, err := shadow.New(cfg.Shadow, logger)
	if err != nil {
		return nil, fmt.Errorf("Shadow gagal: %w", err)
	}

	// 3. Tenant
	logger.Info("Wiring Tenant...")
	tenantRepo := tenant.NewRepository(f.DB(), sh, logger)
	tenantSvc  := tenant.NewService(tenantRepo, f, logger)
	resolver   := tenant.NewResolver(tenantSvc, cfg.App.BaseDomain)

	// 4. Sanctum + TokenStore
	logger.Info("Wiring Sanctum...")
	sanctum := auth.NewSanctum(auth.SanctumConfig{
		AccessSecret:  cfg.Sanctum.AccessSecret,
		RefreshSecret: cfg.Sanctum.RefreshSecret,
		AccessTTL:     cfg.Sanctum.AccessTTL,
		RefreshTTL:    cfg.Sanctum.RefreshTTL,
		Issuer:        cfg.Sanctum.Issuer,
	})
	tokenStore := auth.NewShadowTokenStore(sh)

	// 5. TokenValidator — singleton untuk Bloodgate, tidak butuh DB
	tokenValidator := auth.NewTokenValidator(sanctum, tokenStore)

	// 6. User factory
	userFactory := user.NewServiceFactory(f, sh, cfg.Sanctum.BcryptCost, logger)

	// 7. Auth factory
	authFactory := auth.NewServiceFactory(sanctum, tokenStore, userFactory, logger)

	// 8. Covenant (RBAC)
	logger.Info("Wiring Covenant...")
	covenant := rbac.NewCovenant()

	logger.Info("Semua service core berhasil di-wire",
		zap.String("driver", string(cfg.Fangs.Driver)),
		zap.String("cache", cfg.Shadow.Addr),
	)

	return &Services{
		Fangs:          f,
		Shadow:         sh,
		TenantRepo:     tenantRepo,
		TenantService:  tenantSvc,
		TenantResolver: resolver,
		TokenValidator: tokenValidator,
		AuthFactory:    authFactory,
		UserFactory:    userFactory,
		Covenant:       covenant,
	}, nil
}
