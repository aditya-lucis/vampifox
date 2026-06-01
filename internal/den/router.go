package den

import (
	"github.com/gin-gonic/gin"

	"github.com/aditya-lucis/vampifox/internal/api/rest"
)

// buildRouter membuat dan mengkonfigurasi Gin engine dari services yang sudah di-wire.
// Dipanggil dari Awaken() setelah wire() selesai.
func (d *Den) buildRouter() *gin.Engine {
	svc := d.svc
	return rest.NewRouter(rest.RouterDeps{
		TokenValidator: svc.TokenValidator,
		AuthFactory:    svc.AuthFactory,
		UserFactory:    svc.UserFactory,
		TenantResolver: svc.TenantResolver,
		Covenant:       svc.Covenant,
		Logger:         d.logger,
	})
}
