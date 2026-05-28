package den

import (
	"net/http"

	"github.com/aditya-lucis/vampifox/internal/api/rest"
)

// buildRouter membuat HTTP handler dari services yang sudah di-wire.
func (d *Den) buildRouter() http.Handler {
	svc := d.services
	return rest.NewRouter(rest.RouterDeps{
		TokenValidator: svc.TokenValidator,
		AuthFactory:    svc.AuthFactory,
		UserFactory:    svc.UserFactory,
		TenantResolver: svc.TenantResolver,
		Covenant:       svc.Covenant,
		Logger:         d.logger,
	})
}
