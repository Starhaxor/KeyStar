package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starloader/backend/internal/admin"
	"github.com/starloader/backend/internal/config"
	"github.com/starloader/backend/internal/credential"
	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/httpapi/adminapi"
	"github.com/starloader/backend/internal/httpapi/serverapi"
	"github.com/starloader/backend/internal/metrics"
	"github.com/starloader/backend/internal/security"
	"github.com/starloader/backend/internal/service"
	"github.com/starloader/backend/internal/service/adminauth"
	"github.com/starloader/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	mode, args, err := parseCommand(os.Args[1:])
	if err != nil {
		return err
	}
	switch mode {
	case commandServe:
		return runServer()
	case commandMigrate:
		return runMigration(args[0])
	case commandAdmin:
		return runAdmin(args)
	case commandKeygen:
		return generateSigningKeys(os.Stdout, cryptorand.Reader)
	default:
		return errors.New("unsupported command")
	}
}

type commandMode string

const (
	commandServe   commandMode = "serve"
	commandMigrate commandMode = "migrate"
	commandAdmin   commandMode = "admin"
	commandKeygen  commandMode = "keygen"
)

func parseCommand(args []string) (commandMode, []string, error) {
	if len(args) == 0 {
		return commandServe, nil, nil
	}
	switch args[0] {
	case "serve":
		if len(args) != 1 {
			return "", nil, errors.New("serve does not accept arguments")
		}
		return commandServe, nil, nil
	case "migrate":
		if len(args) != 2 || (args[1] != "up" && args[1] != "down") {
			return "", nil, errors.New("usage: server migrate up|down")
		}
		return commandMigrate, args[1:], nil
	case "admin":
		if len(args) < 2 {
			return "", nil, errors.New("usage: server admin create-user|create-license|create-admin [options]")
		}
		return commandAdmin, args[1:], nil
	case "keygen":
		if len(args) != 1 {
			return "", nil, errors.New("keygen does not accept arguments")
		}
		return commandKeygen, nil, nil
	default:
		return "", nil, errors.New("usage: server [serve|migrate up|migrate down|admin ...|keygen]")
	}
}

func generateSigningKeys(output io.Writer, random io.Reader) error {
	if output == nil || random == nil {
		return errors.New("key generation dependencies are required")
	}
	seed := make([]byte, ed25519.SeedSize)
	if _, err := io.ReadFull(random, seed); err != nil {
		return fmt.Errorf("generate signing key: %w", err)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if _, err := fmt.Fprintf(output, "ED25519_PRIVATE_KEY=%s\nSTARLOADER_ED25519_PUBLIC_KEY=%s\n",
		base64.StdEncoding.EncodeToString(seed), base64.StdEncoding.EncodeToString(publicKey)); err != nil {
		return fmt.Errorf("write signing keys: %w", err)
	}
	return nil
}

func runServer() error {
	configuration, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	trustedProxies := make([]netip.Prefix, 0)
	for _, configuredProxy := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		configuredProxy = strings.TrimSpace(configuredProxy)
		if configuredProxy == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(configuredProxy)
		if err != nil {
			return errors.New("configuration error: TRUSTED_PROXIES contains an invalid network")
		}
		trustedProxies = append(trustedProxies, prefix)
	}

	applicationCtx, cancelApplication := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelApplication()
	pool, err := pgxpool.New(applicationCtx, configuration.DatabaseURL)
	if err != nil {
		return errors.New("database configuration failed")
	}
	defer pool.Close()
	startupCtx, cancelStartup := context.WithTimeout(applicationCtx, 10*time.Second)
	defer cancelStartup()
	if err := pool.Ping(startupCtx); err != nil {
		return errors.New("database connection failed")
	}

	repository := store.New(pool)
	defaultApplication, err := repository.FindDefaultApplication(startupCtx)
	if err != nil {
		return fmt.Errorf("resolve default application: %w", err)
	}
	loginService := service.NewLoginService(repository, configuration.Product)
	privateKey, err := security.ParseEd25519PrivateKey(configuration.Ed25519PrivateKey)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}
	tokenIssuer, err := security.NewTokenIssuer(privateKey, configuration.LicenseIssuer, configuration.LicenseAudience, configuration.Product)
	if err != nil {
		return errors.New("configuration error: invalid token issuer configuration")
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return errors.New("configuration error: invalid token verifier key")
	}
	tokenVerifier, err := security.NewTokenVerifier(publicKey, configuration.LicenseIssuer, configuration.LicenseAudience, configuration.Product)
	if err != nil {
		return errors.New("configuration error: invalid token verifier configuration")
	}
	refreshService := service.NewRefreshService(service.RefreshServiceConfig{
		Repository: repository, Profile: repository,
		HMACKey: []byte(configuration.LicenseHMACKey), TokenIssuer: tokenIssuer,
		Issuer: configuration.LicenseIssuer, Audience: configuration.LicenseAudience, Product: configuration.Product,
	})
	deviceService := service.NewDeviceService(service.NewStoreDeviceRepository(repository), service.DeviceServiceConfig{
		HardwareHMACKey: []byte(configuration.HardwareHMACKey),
		TokenIssuer:     tokenIssuer,
		Issuer:          configuration.LicenseIssuer,
		Audience:        configuration.LicenseAudience,
		Product:         configuration.Product,
		RefreshService:  refreshService,
	})
	adminConfig := httpapi.AdminConfig{}
	if configuration.AdminConsoleEnabled {
		adminAuthService := adminauth.New(repository, adminauth.Config{
			SessionTTL:    configuration.AdminSessionTTL,
			Random:        cryptorand.Reader,
			Now:           time.Now,
			EncryptionKey: []byte(configuration.AdminMFAEncryptionKey),
		})
		adminConfig = httpapi.AdminConfig{
			Auth:           adminAuthService,
			Console:        repository,
			BootstrapToken: configuration.AdminBootstrapToken,
			LicenseHMACKey: []byte(configuration.LicenseHMACKey),
			Product:        configuration.Product,
			MFAIssuer:      "KeyStar Admin",
			AllowedOrigins: configuration.AdminAllowedOrigins,
			CSRFSecret:     []byte(configuration.AdminSessionSecret),
			CookieSecure:   configuration.AdminCookieSecure,
			SessionTTL:     configuration.AdminSessionTTL,
		}
	} else {
		// Keep the allowed origin so disabled-console responses still carry
		// CORS headers and browsers can render the 503 message.
		adminConfig = httpapi.AdminConfig{AllowedOrigins: configuration.AdminAllowedOrigins}
		log.Printf("admin console disabled: /v1/admin endpoints will return 503")
	}
	credentialVerifier := credential.NewVerifier(repository)
	// Optional Prometheus instrumentation, gated so metrics (including
	// route-level traffic) are never exposed unintentionally.
	var registry *metrics.Registry
	if configuration.MetricsEnabled {
		registry = metrics.NewRegistry()
		registry.DeclareCounter("keystar_http_requests_total", "HTTP requests processed.")
		registry.DeclareHistogram("keystar_http_request_duration_seconds", "HTTP request latency in seconds.")
		registry.DeclareGauge("keystar_webhook_backlog", "Webhook deliveries waiting to be sent.")
	}

	router := httpapi.NewRouter(httpapi.RouterConfig{
		Login:                    loginService,
		DeviceVerification:       deviceService,
		SessionVerifier:          tokenVerifier,
		Profile:                  repository,
		LoginTimeout:             configuration.LoginTimeout,
		TrustedProxies:           trustedProxies,
		Logger:                   log.Default(),
		RateLimitMaxKeys:         envInt("RATE_LIMIT_MAX_KEYS", 0),
		RateLimits:               repository,
		CredentialRateLimit:      envInt("CREDENTIAL_RATE_LIMIT", 0),
		HealthCheck:              pool.Ping,
		Metrics:                  registry,
		MetricsToken:             configuration.MetricsToken,
		Admin:                    adminConfig,
		DefaultApplicationID:     defaultApplication.ID,
		Applications:             repository,
		Credentials:              credentialVerifier,
		DisableLegacyApplication: configuration.ClientCredentialsRequired,
		Server: httpapi.ServerConfig{
			LicenseHMACKey: []byte(configuration.LicenseHMACKey),
			Product:        configuration.Product,
		},
		ServerStore:    repository,
		RefreshService: refreshService,
	})
	// The admin and server namespaces are mounted as separate handlers so each
	// package owns its routes while sharing the core router middleware.
	router.MountAdmin(adminapi.New(router))
	router.MountServer(serverapi.New(router))
	address := strings.TrimSpace(os.Getenv("SERVER_ADDR"))
	if address == "" {
		address = ":8080"
	}
	server := newHTTPServer(address, router, applicationCtx)

	// Start the webhook delivery worker. It drains the outbox table that
	// console mutations enqueue events into; without it webhooks never fire.
	webhookDispatcher := service.NewWebhookDispatcher(service.WebhookDispatcherConfig{
		WebhookRepo: repository,
	})
	go runWebhookWorker(applicationCtx, webhookDispatcher,
		func() (int64, error) { return repository.CountOutboxBacklog(applicationCtx) },
		registry, log.Default())

	// Start periodic cleanup of expired refresh sessions plus audit and
	// security-event retention. Retention is disabled (keep forever) unless a
	// day count is configured through the environment.
	auditRetentionDays := envInt("AUDIT_RETENTION_DAYS", 0)
	securityRetentionDays := envInt("SECURITY_EVENT_RETENTION_DAYS", 0)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-applicationCtx.Done():
				return
			case <-ticker.C:
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				deleted, err := repository.DeleteExpiredRefreshSessions(cleanupCtx)
				if err != nil {
					log.Printf("refresh session cleanup error: %v", err)
				} else if deleted > 0 {
					log.Printf("refresh session cleanup: deleted %d expired sessions", deleted)
				}
				if auditRetentionDays > 0 {
					cutoff := time.Now().UTC().AddDate(0, 0, -auditRetentionDays)
					if deleted, err := repository.DeleteAuditLogsBefore(cleanupCtx, cutoff); err != nil {
						log.Printf("audit log retention error: %v", err)
					} else if deleted > 0 {
						log.Printf("audit log retention: deleted %d entries older than %dd", deleted, auditRetentionDays)
					}
				}
				if securityRetentionDays > 0 {
					cutoff := time.Now().UTC().AddDate(0, 0, -securityRetentionDays)
					if deleted, err := repository.DeleteSecurityEventsBefore(cleanupCtx, cutoff); err != nil {
						log.Printf("security event retention error: %v", err)
					} else if deleted > 0 {
						log.Printf("security event retention: deleted %d events older than %dd", deleted, securityRetentionDays)
					}
				}
				cancel()
			}
		}
	}()

	log.Printf("license service listening on %s", address)
	if err := serveUntilStopped(applicationCtx, cancelApplication, server, 10*time.Second); err != nil {
		return fmt.Errorf("server stopped: %w", err)
	}
	return nil
}

// envInt reads an integer environment variable, falling back to the default
// when unset or malformed.
func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func runMigration(action string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	if action == "up" {
		err = store.MigrateUp(ctx, pool)
	} else {
		err = store.MigrateDown(ctx, pool)
	}
	if err != nil {
		return fmt.Errorf("migration %s failed: %w", action, err)
	}
	log.Printf("migration %s completed", action)
	return nil
}

func runAdmin(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	repository := store.New(pool)
	licenseHMACKey := strings.TrimSpace(os.Getenv("LICENSE_HMAC_KEY"))
	if args[0] == "create-license" && licenseHMACKey == "" {
		return errors.New("configuration error: LICENSE_HMAC_KEY is not set")
	}
	passwordReader := admin.PasswordReader(admin.ReadPasswordFromTerminal)
	if hasArgument(args, "--password-stdin") {
		scanner := bufio.NewScanner(os.Stdin)
		passwordReader = func() (string, error) {
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return "", err
				}
				return "", errors.New("password input ended unexpectedly")
			}
			return scanner.Text(), nil
		}
	}
	return admin.Run(
		ctx,
		args,
		os.Stdout,
		adminUserRepository{store: repository},
		adminLicenseRepository{store: repository, hmacKey: []byte(licenseHMACKey)},
		adminAccountRepository{store: repository},
		passwordReader,
		cryptorand.Reader,
		time.Now,
	)
}

func hasArgument(args []string, wanted string) bool {
	for _, argument := range args {
		if argument == wanted {
			return true
		}
	}
	return false
}

func openDatabase(ctx context.Context) (*pgxpool.Pool, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return nil, errors.New("configuration error: DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, errors.New("database configuration failed")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("database connection failed")
	}
	return pool, nil
}

type adminUserRepository struct {
	store *store.Store
}

func (repository adminUserRepository) CreateUser(ctx context.Context, email, passwordHash string) error {
	applicationID, err := defaultApplicationID(ctx, repository.store)
	if err != nil {
		return err
	}
	_, err = repository.store.CreateUser(ctx, applicationID, domain.NewUser{Email: email, PasswordHash: passwordHash})
	return err
}

type adminLicenseRepository struct {
	store   *store.Store
	hmacKey []byte
}

func (repository adminLicenseRepository) CreateLicense(ctx context.Context, normalized, userEmail, product string, expiresAt time.Time, maxDevices int) error {
	applicationID, err := defaultApplicationID(ctx, repository.store)
	if err != nil {
		return err
	}
	user, err := repository.store.FindUserByEmail(ctx, applicationID, userEmail)
	if err != nil {
		return err
	}
	// The CLI product name is resolved into the application's product catalog
	// and its default plan; the license is bound to both.
	productID, planID, err := repository.store.ResolveProductPlan(ctx, applicationID, product)
	if err != nil {
		return err
	}
	_, err = repository.store.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: security.HMACHex(repository.hmacKey, normalized),
		UserID:      user.ID,
		ProductID:   productID,
		PlanID:      planID,
		MaxDevices:  maxDevices,
		ExpiresAt:   expiresAt,
	})
	return err
}

// defaultApplicationID resolves the default StarLoader application for legacy
// flows that have no explicit application context yet (CLI commands, console).
func defaultApplicationID(ctx context.Context, repository *store.Store) (string, error) {
	application, err := repository.FindDefaultApplication(ctx)
	if err != nil {
		return "", err
	}
	return application.ID, nil
}

type adminAccountRepository struct {
	store *store.Store
}

func (repository adminAccountRepository) CreateAdminAccount(ctx context.Context, email, passwordHash, roleName string) error {
	_, err := repository.store.CreateAdminAccount(ctx, domain.NewAdminAccount{Email: email, PasswordHash: passwordHash, RoleName: roleName})
	return err
}

func newHTTPServer(address string, handler http.Handler, applicationCtx context.Context) *http.Server {
	return &http.Server{
		Addr:    address,
		Handler: handler,
		BaseContext: func(net.Listener) context.Context {
			return applicationCtx
		},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

type managedHTTPServer interface {
	Shutdown(context.Context) error
	Close() error
}

type runningHTTPServer interface {
	managedHTTPServer
	ListenAndServe() error
}

func serveUntilStopped(applicationCtx context.Context, cancelApplication context.CancelFunc, server runningHTTPServer, gracePeriod time.Duration) error {
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-applicationCtx.Done():
		return shutdownServer(server, cancelApplication, gracePeriod)
	case err := <-serverErrors:
		cancelApplication()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func shutdownServer(server managedHTTPServer, cancelApplication context.CancelFunc, gracePeriod time.Duration) error {
	cancelApplication()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), gracePeriod)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		if closeErr := server.Close(); err != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}
	return nil
}
