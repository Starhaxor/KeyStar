package httpapi

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/service"
)

type LoginService interface {
	Login(context.Context, service.LoginInput) (service.PendingChallenge, error)
}

type DeviceVerificationService interface {
	Verify(context.Context, service.VerifyInput) (service.VerifiedSession, error)
}

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
}

type Router struct {
	login               LoginService
	deviceVerification  DeviceVerificationService
	profile             ProfileRepository
	healthCheck         func(context.Context) error
	healthCheckTimeout  time.Duration
	loginTimeout        time.Duration
	deviceVerifyTimeout time.Duration
	trustedProxies      []netip.Prefix
	loginLimiter        *ipRateLimiter
	sessionLimiter      *ipRateLimiter
	admin               AdminConfig
	adminLimiter        *ipRateLimiter
	now                   func() time.Time
	defaultApplicationID string
	applications         ApplicationResolver
	credentials          CredentialVerifier
	disableLegacyApplication bool
	loginHandler         http.Handler
	deviceVerifyHandler  http.Handler
	meHandler            http.Handler
	handler              http.Handler
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
		login:               config.Login,
		deviceVerification:  config.DeviceVerification,
		profile:             config.Profile,
		healthCheck:         config.HealthCheck,
		healthCheckTimeout:  healthCheckTimeout,
		loginTimeout:        loginTimeout,
		deviceVerifyTimeout: deviceVerifyTimeout,
		trustedProxies:      append([]netip.Prefix(nil), config.TrustedProxies...),
		loginLimiter:        newIPRateLimiter(5, time.Minute, config.RateLimitMaxKeys, config.Now),
		sessionLimiter:      newIPRateLimiter(10, time.Minute, config.RateLimitMaxKeys, config.Now),
		admin:                config.Admin,
		adminLimiter:         newIPRateLimiter(10, time.Minute, config.RateLimitMaxKeys, config.Now),
		now:                     now,
		defaultApplicationID:    config.DefaultApplicationID,
		applications:            config.Applications,
		credentials:             config.Credentials,
		disableLegacyApplication: config.DisableLegacyApplication,
	}
	router.loginHandler = router.requireCredential(domain.CredentialPublishable, "auth.login")(http.HandlerFunc(router.handleLogin))
	router.deviceVerifyHandler = router.requireCredential(domain.CredentialPublishable, "device.verify")(http.HandlerFunc(router.handleDeviceVerify))
	router.meHandler = RequireSession(config.SessionVerifier, http.HandlerFunc(router.handleMe))
	router.handler = requestIDMiddleware(recoveryMiddleware(logger, http.HandlerFunc(router.route)))
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
	case request.Method == http.MethodGet && request.URL.Path == "/healthz":
		router.handleHealth(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/v1/me":
		router.meHandler.ServeHTTP(writer, request)
	case strings.HasPrefix(request.URL.Path, adminPathPrefix):
		router.serveAdmin(writer, request)
	case request.URL.Path == "/v1/auth/login" || request.URL.Path == "/v1/device/verify" || request.URL.Path == "/healthz" || request.URL.Path == "/v1/me":
		writeError(writer, request, http.StatusMethodNotAllowed, "INVALID_REQUEST", "method not allowed")
	default:
		writeError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func (router *Router) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if router.healthCheck != nil {
		ctx, cancel := context.WithTimeout(request.Context(), router.healthCheckTimeout)
		defer cancel()
		if err := router.healthCheck(ctx); err != nil {
			writeError(writer, request, http.StatusServiceUnavailable, "SERVER_ERROR", "service unavailable")
			return
		}
	}
	writeJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}
