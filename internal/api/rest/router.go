package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/api/middleware"
	v1 "github.com/aditya-lucis/vampifox/internal/api/rest/v1"
	"github.com/aditya-lucis/vampifox/internal/core/auth"
	"github.com/aditya-lucis/vampifox/internal/core/rbac"
	"github.com/aditya-lucis/vampifox/internal/core/tenant"
	"github.com/aditya-lucis/vampifox/internal/core/user"
)

// ═══════════════════════════════════════════════════════════════
//  RouterDeps — dependency yang dibutuhkan router
// ═══════════════════════════════════════════════════════════════

// RouterDeps semua dependency yang dibutuhkan router.
// Semua field adalah singleton — dibuat sekali saat startup.
type RouterDeps struct {
	// TokenValidator untuk Bloodgate middleware — tidak butuh DB
	TokenValidator *auth.TokenValidator

	// AuthFactory untuk /auth/* handlers — membuat Service per-request
	AuthFactory *auth.ServiceFactory

	// UserFactory untuk /users/* handlers — membuat Service per-request
	UserFactory *user.ServiceFactory

	// TenantResolver untuk Territory middleware
	TenantResolver *tenant.Resolver

	// Covenant untuk authorization
	Covenant *rbac.Covenant

	Logger *zap.Logger
}

// ═══════════════════════════════════════════════════════════════
//  NewRouter
// ═══════════════════════════════════════════════════════════════

// NewRouter membuat dan mengkonfigurasi Gin engine dengan semua route.
func NewRouter(deps RouterDeps) *gin.Engine {
	r := gin.New()

	// ── Global middleware (urutan penting) ────────────────────────
	r.Use(
		middleware.RequestID(),
		middleware.Recovery(deps.Logger),
		middleware.Logger(deps.Logger),
	)

	// ── Health & root — tidak butuh tenant/auth ───────────────────
	r.GET("/health", handleHealth)
	r.GET("/", handleRoot)

	// ── API v1 ────────────────────────────────────────────────────
	api := r.Group("/api/v1")

	// Territory: resolve tenant dari semua request API
	api.Use(middleware.Territory(deps.TenantResolver, deps.Logger))

	// ── Auth routes — tidak butuh JWT, tapi butuh tenant ─────────
	authGroup := api.Group("/auth")
	v1.RegisterAuthRoutes(authGroup, deps.AuthFactory, deps.Logger)

	// ── Protected routes — butuh JWT ─────────────────────────────
	protected := api.Group("")
	protected.Use(middleware.Bloodgate(deps.TokenValidator, deps.Logger))

	// User routes
	v1.RegisterUserRoutes(
		protected.Group("/users"),
		deps.UserFactory,
		deps.Covenant,
		deps.Logger,
	)

	return r
}

// ── Handlers ──────────────────────────────────────────────────────

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "alive",
		"service": "vampifox",
		"version": "0.1.0-nightfall",
	})
}

func handleRoot(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":    "VampiFox Application Framework",
		"version": "0.1.0-nightfall",
		"docs":    "/api/v1",
	})
}
