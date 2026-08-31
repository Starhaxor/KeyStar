package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"io"
	"log"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/metrics"
)

type RouterConfig struct {
	Login               LoginService
	DeviceVerification  DeviceVerificationService
	SessionVerifier     BearerVerifier
	Profile             ProfileRepository
	HealthCheck         func(context.Context) error
	HealthCheckTimeout  time.Duration
	LoginTimeout        time.Duration
	DeviceVerifyTimeout time.Duration
	TrustedProxies      []netip.Prefix
	Logger              *log.Logger
	RateLimitMaxKeys    int
	RateLimits          RateLimitStore
	// CredentialRateLimit caps requests per credential per minute across the
	// client and server namespaces. Zero selects the default (120).
	CredentialRateLimit int
	Now                 func() time.Time
	Admin               AdminConfig
	// DefaultApplicationID is the tenant boundary applied to client requests
	// that do not carry an explicit application context. It is resolved once
	// at startup from the default StarLoader application.
	DefaultApplicationID string
	// Applications resolves the X-KeyStar-App header to a live application.
	Applications ApplicationResolver
	// Credentials validates publishable/secret keys presented by clients.
	Credentials CredentialVerifier
	// DisableLegacyApplication rejects client requests that carry no
	// credential. Kept off during migration (phase A) so existing clients
	// keep working with the default application context.
	DisableLegacyApplication bool
	// Server enables the /v1/server namespace. When empty, the namespace
	// returns 503.
	Server ServerConfig
	// ServerStore backs the /v1/server namespace (application-scoped).
	ServerStore ServerStore
	// RefreshService manages refresh token issuance, rotation and reuse detection.
	RefreshService RefreshService
	// ApplicationSigner is staged for the later application-scoped token
	// profile migration. Existing token issuance and verification do not use it.
	ApplicationSigner ApplicationSigner
	// Metrics, when set, enables request instrumentation and the /metrics
	// endpoint. Nil keeps both disabled (default).
	Metrics      *metrics.Registry
	MetricsToken string
}

// Router is the root HTTP handler. It serves the public client API directly
// and dispatches the /v1/admin and /v1/server namespaces to the handlers
// mounted with MountAdmin and MountServer (built by the adminapi and serverapi
// subpackages).
type Router struct {
	login                    LoginService
	deviceVerification       DeviceVerificationService
	profile                  ProfileRepository
	healthCheck              func(context.Context) error
	healthCheckTimeout       time.Duration
	loginTimeout             time.Duration
	deviceVerifyTimeout      time.Duration
	trustedProxies           []netip.Prefix
	loginLimiter             *ipRateLimiter
	sessionLimiter           *ipRateLimiter
	credentialLimiter        *ipRateLimiter
	Admin                    AdminConfig
	adminLimiter             *ipRateLimiter
	now                      func() time.Time
	defaultApplicationID     string
	applications             ApplicationResolver
	credentials              CredentialVerifier
	disableLegacyApplication bool
	Server                   ServerConfig
	ServerStore              ServerStore
	refreshService           *refreshServiceAdapter
	applicationSigner        ApplicationSigner
	adminHandler             http.Handler
	serverHandler            http.Handler
	loginHandler             http.Handler
	deviceVerifyHandler      http.Handler
	refreshHandler           http.Handler
	logoutHandler            http.Handler
	meHandler                http.Handler
	handler                  http.Handler
	metrics                  *metrics.Registry
	metricsToken             string
	rateLimits               RateLimitStore
}

// Now returns the router clock (injectable in tests).
func (router *Router) Now() time.Time {
	return router.now()
}

// LoginTimeout is the per-login context timeout.
func (router *Router) LoginTimeout() time.Duration {
	return router.loginTimeout
}

// TrustedProxies returns the configured proxy prefixes.
func (router *Router) TrustedProxies() []netip.Prefix {
	return router.trustedProxies
}

// DefaultApplicationID returns the application boundary for requests without
// an explicit X-KeyStar-App header.
func (router *Router) DefaultApplicationID() string {
	return router.defaultApplicationID
}

// AllowAdminRate gates dashboard login/MFA endpoints with the admin rate
// limiter.
func (router *Router) AllowAdminRate(ctx context.Context, key string) bool {
	allowed, _ := router.allowRate(ctx, "admin", key, 10, time.Minute, router.adminLimiter)
	return allowed
}

// credentialRateLimit normalizes the configured per-credential limit.
func credentialRateLimit(configured int) int {
	if configured <= 0 {
		return 120
	}
	return configured
}

// AllowCredentialRate gates client/server endpoints with the per-credential
// limiter. retryAfter is meaningful only when allowed is false.
func (router *Router) AllowCredentialRate(key string) (allowed bool, retryAfter int) {
	return router.AllowCredentialRateContext(context.Background(), key)
}

func (router *Router) AllowCredentialRateContext(ctx context.Context, key string) (bool, int) {
	return router.allowRate(ctx, "credential", key, router.credentialLimiter.limit, router.credentialLimiter.window, router.credentialLimiter)
}

func (router *Router) allowRate(ctx context.Context, namespace, key string, limit int, window time.Duration, fallback *ipRateLimiter) (bool, int) {
	if router.rateLimits == nil {
		return fallback.allowWithRetry(key)
	}
	digest := sha256.Sum256([]byte(namespace + "\x00" + key))
	allowed, retry, err := router.rateLimits.AllowRateLimit(ctx, digest[:], limit, window, router.now().UTC())
	if err != nil {
		return false, 1
	}
	return allowed, retry
}

// MountAdmin attaches the /v1/admin namespace handler.
func (router *Router) MountAdmin(handler http.Handler) {
	router.adminHandler = handler
}

// MountServer attaches the /v1/server namespace handler.
func (router *Router) MountServer(handler http.Handler) {
	router.serverHandler = handler
}

func NewRouter(config RouterConfig) *Router {
	logger := config.Logger
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	healthCheckTimeout := config.HealthCheckTimeout
	if healthCheckTimeout <= 0 {
		healthCheckTimeout = 2 * time.Second
	}
	loginTimeout := config.LoginTimeout
	if loginTimeout <= 0 {
		loginTimeout = 10 * time.Second
	}
	deviceVerifyTimeout := config.DeviceVerifyTimeout
	if deviceVerifyTimeout <= 0 {
		deviceVerifyTimeout = 10 * time.Second
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	router := &Router{
		login:                    config.Login,
		deviceVerification:       config.DeviceVerification,
		profile:                  config.Profile,
		healthCheck:              config.HealthCheck,
		healthCheckTimeout:       healthCheckTimeout,
		loginTimeout:             loginTimeout,
		deviceVerifyTimeout:      deviceVerifyTimeout,
		trustedProxies:           append([]netip.Prefix(nil), config.TrustedProxies...),
		loginLimiter:             newIPRateLimiter(5, time.Minute, config.RateLimitMaxKeys, config.Now),
		sessionLimiter:           newIPRateLimiter(10, time.Minute, config.RateLimitMaxKeys, config.Now),
		Admin:                    config.Admin,
		adminLimiter:             newIPRateLimiter(10, time.Minute, config.RateLimitMaxKeys, config.Now),
		credentialLimiter:        newIPRateLimiter(credentialRateLimit(config.CredentialRateLimit), time.Minute, config.RateLimitMaxKeys, config.Now),
		now:                      now,
		defaultApplicationID:     config.DefaultApplicationID,
		applications:             config.Applications,
		credentials:              config.Credentials,
		disableLegacyApplication: config.DisableLegacyApplication,
		Server:                   config.Server,
		ServerStore:              config.ServerStore,
		refreshService:           wrapRefreshService(config.RefreshService),
		applicationSigner:        config.ApplicationSigner,
		metricsToken:             config.MetricsToken,
		rateLimits:               config.RateLimits,
	}
	router.loginHandler = router.RequireCredential(domain.CredentialPublishable, "auth.login")(http.HandlerFunc(router.handleLogin))
	router.deviceVerifyHandler = router.RequireCredential(domain.CredentialPublishable, "device.verify")(http.HandlerFunc(router.handleDeviceVerify))
	router.refreshHandler = router.RequireCredential(domain.CredentialPublishable, "auth.refresh")(http.HandlerFunc(router.handleRefresh))
	router.logoutHandler = router.RequireCredential(domain.CredentialPublishable, "auth.logout")(http.HandlerFunc(router.handleLogout))
	router.meHandler = RequireSession(config.SessionVerifier, http.HandlerFunc(router.handleMe))
	router.metrics = config.Metrics
	var core http.Handler = http.HandlerFunc(router.route)
	if config.Metrics != nil {
		core = RequestObserver(config.Metrics, core)
	}
	router.handler = requestIDMiddleware(recoveryMiddleware(logger, core))
	return router
}

func (router *Router) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	router.handler.ServeHTTP(writer, request)
}

func (router *Router) route(writer http.ResponseWriter, request *http.Request) {
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/auth/login":
		router.loginHandler.ServeHTTP(writer, request)
	case request.Method == http.MethodPost && request.URL.Path == "/v1/device/verify":
		router.deviceVerifyHandler.ServeHTTP(writer, request)
	case request.Method == http.MethodPost && request.URL.Path == "/v1/auth/refresh":
		router.refreshHandler.ServeHTTP(writer, request)
	case request.Method == http.MethodPost && request.URL.Path == "/v1/auth/logout":
		router.logoutHandler.ServeHTTP(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/healthz":
		router.handleHealth(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/readyz":
		router.handleReadyz(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/status":
		router.handleStatus(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/metrics" && router.metrics != nil:
		if !router.authorizeMetrics(request) {
			writer.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			WriteError(writer, request, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
			return
		}
		router.metrics.Handler().ServeHTTP(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/v1/me":
		router.meHandler.ServeHTTP(writer, request)
	case strings.HasPrefix(request.URL.Path, AdminPathPrefix):
		router.serveMounted(writer, request, router.adminHandler, "admin console unavailable")
	case strings.HasPrefix(request.URL.Path, ServerPathPrefix):
		router.serveMounted(writer, request, router.serverHandler, "server api unavailable")
	case request.URL.Path == "/v1/auth/login" || request.URL.Path == "/v1/auth/refresh" || request.URL.Path == "/v1/auth/logout" || request.URL.Path == "/v1/device/verify" || request.URL.Path == "/healthz" || request.URL.Path == "/readyz" || request.URL.Path == "/status" || request.URL.Path == "/v1/me":
		WriteError(writer, request, http.StatusMethodNotAllowed, "INVALID_REQUEST", "method not allowed")
	default:
		WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func (router *Router) authorizeMetrics(request *http.Request) bool {
	token, ok := bearerKey(strings.TrimSpace(request.Header.Get("Authorization")))
	if !ok || router.metricsToken == "" || len(token) != len(router.metricsToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(router.metricsToken)) == 1
}

// serveMounted dispatches to a mounted namespace handler, answering 503 when
// the namespace was never mounted or its dependencies are missing.
func (router *Router) serveMounted(writer http.ResponseWriter, request *http.Request, handler http.Handler, unavailableMessage string) {
	if handler == nil {
		WriteError(writer, request, http.StatusServiceUnavailable, "SERVER_ERROR", unavailableMessage)
		return
	}
	handler.ServeHTTP(writer, request)
}

func (router *Router) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if router.healthCheck != nil {
		ctx, cancel := context.WithTimeout(request.Context(), router.healthCheckTimeout)
		defer cancel()
		if err := router.healthCheck(ctx); err != nil {
			WriteError(writer, request, http.StatusServiceUnavailable, "SERVER_ERROR", "service unavailable")
			return
		}
	}
	WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// handleReadyz reports whether the service is ready to accept traffic.
// Returns 203 when the database is reachable, 503 otherwise.
func (router *Router) handleReadyz(writer http.ResponseWriter, request *http.Request) {
	if router.healthCheck != nil {
		ctx, cancel := context.WithTimeout(request.Context(), router.healthCheckTimeout)
		defer cancel()
		if err := router.healthCheck(ctx); err != nil {
			WriteJSON(writer, http.StatusServiceUnavailable, struct {
				OK      bool   `json:"ok"`
				Status  string `json:"status"`
				Message string `json:"message"`
			}{OK: false, Status: "not_ready", Message: "database unreachable"})
			return
		}
	}
	WriteJSON(writer, http.StatusOK, struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}{OK: true, Status: "ready"})
}

// handleStatus returns a lightweight service status overview without
// requiring database connectivity. Useful for load balancer probes.
func (router *Router) handleStatus(writer http.ResponseWriter, request *http.Request) {
	WriteJSON(writer, http.StatusOK, struct {
		OK      bool   `json:"ok"`
		Service string `json:"service"`
		Version string `json:"version"`
		Status  string `json:"status"`
	}{OK: true, Service: "keystar", Version: "0.1.0", Status: "running"})
}
