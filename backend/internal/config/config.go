package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultLoginTimeout        = 10 * time.Second
	defaultAdminSessionTTL     = 12 * time.Hour
	defaultAdminAllowedOrigins = "http://localhost:3000,http://127.0.0.1:3000"
)

var requiredEnvironmentVariables = [...]string{
	"DATABASE_URL",
	"LICENSE_HMAC_KEY",
	"HARDWARE_HMAC_KEY",
	"ED25519_PRIVATE_KEY",
	"LICENSE_ISSUER",
	"LICENSE_AUDIENCE",
	"PRODUCT",
	"ADMIN_SESSION_SECRET",
	"ADMIN_MFA_ENCRYPTION_KEY",
	"ADMIN_BOOTSTRAP_TOKEN",
}

// Config contains the values required to operate the license service. Secrets
// are read only from the environment and must never be logged.
type Config struct {
	DatabaseURL           string
	LicenseHMACKey        string
	HardwareHMACKey       string
	Ed25519PrivateKey     string
	LicenseIssuer         string
	LicenseAudience       string
	Product               string
	LoginTimeout          time.Duration
	AdminConsoleEnabled   bool
	AdminSessionSecret    string
	AdminMFAEncryptionKey string
	AdminBootstrapToken   string
	AdminAllowedOrigins   []string
	AdminSessionTTL       time.Duration
	AdminCookieSecure     bool
	// ClientCredentialsRequired keeps the public client API in strict mode:
	// login and device verification require a publishable API key.
	ClientCredentialsRequired bool
	MetricsEnabled            bool
	MetricsToken              string
}

// Load reads the complete configuration, refusing to start when any required
// setting is missing or blank.
func Load() (Config, error) {
	values := make(map[string]string, len(requiredEnvironmentVariables))
	for _, name := range requiredEnvironmentVariables {
		value, ok := os.LookupEnv(name)
		if !ok || strings.TrimSpace(value) == "" {
			return Config{}, fmt.Errorf("required environment variable %s is not set", name)
		}
		values[name] = value
	}
	mfaKey, err := decodeMFAEncryptionKey(values["ADMIN_MFA_ENCRYPTION_KEY"])
	if err != nil {
		return Config{}, err
	}
	values["ADMIN_MFA_ENCRYPTION_KEY"] = string(mfaKey)
	secretNames := []string{"LICENSE_HMAC_KEY", "HARDWARE_HMAC_KEY", "ADMIN_SESSION_SECRET", "ADMIN_MFA_ENCRYPTION_KEY", "ADMIN_BOOTSTRAP_TOKEN"}
	seenSecrets := make(map[string]string, len(secretNames))
	for _, name := range secretNames {
		if len([]byte(values[name])) < 32 {
			return Config{}, fmt.Errorf("%s must be at least 32 bytes", name)
		}
		if previous, exists := seenSecrets[values[name]]; exists {
			return Config{}, fmt.Errorf("%s and %s must differ", previous, name)
		}
		seenSecrets[values[name]] = name
	}
	loginTimeout := defaultLoginTimeout
	if configuredTimeout := strings.TrimSpace(os.Getenv("LOGIN_TIMEOUT")); configuredTimeout != "" {
		parsedTimeout, err := time.ParseDuration(configuredTimeout)
		if err != nil || parsedTimeout <= 0 {
			return Config{}, fmt.Errorf("LOGIN_TIMEOUT must be a positive duration")
		}
		loginTimeout = parsedTimeout
	}

	configuredOrigins := strings.TrimSpace(os.Getenv("ADMIN_ALLOWED_ORIGIN"))
	if configuredOrigins == "" {
		configuredOrigins = defaultAdminAllowedOrigins
	}
	adminAllowedOrigins := make([]string, 0, len(strings.Split(configuredOrigins, ",")))
	for _, candidate := range strings.Split(configuredOrigins, ",") {
		candidate = strings.TrimRight(strings.TrimSpace(candidate), "/")
		parsed, err := url.Parse(candidate)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return Config{}, fmt.Errorf("ADMIN_ALLOWED_ORIGIN contains an invalid origin")
		}
		if candidate != "" {
			adminAllowedOrigins = append(adminAllowedOrigins, candidate)
		}
	}
	if len(adminAllowedOrigins) == 0 {
		return Config{}, fmt.Errorf("ADMIN_ALLOWED_ORIGIN must contain at least one origin")
	}
	adminSessionTTL := defaultAdminSessionTTL
	if configuredTTL := strings.TrimSpace(os.Getenv("ADMIN_SESSION_TTL")); configuredTTL != "" {
		parsedTTL, err := time.ParseDuration(configuredTTL)
		if err != nil || parsedTTL <= 0 {
			return Config{}, fmt.Errorf("ADMIN_SESSION_TTL must be a positive duration")
		}
		adminSessionTTL = parsedTTL
	}
	adminCookieSecure := true
	switch configuredSecure := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_COOKIE_SECURE"))); configuredSecure {
	case "false", "0":
		adminCookieSecure = false
	case "", "true", "1":
		adminCookieSecure = true
	default:
		return Config{}, fmt.Errorf("ADMIN_COOKIE_SECURE must be true or false")
	}
	adminConsoleEnabled := true
	switch configuredConsole := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_CONSOLE_ENABLED"))); configuredConsole {
	case "", "true", "1":
		adminConsoleEnabled = true
	case "false", "0":
		adminConsoleEnabled = false
	default:
		return Config{}, fmt.Errorf("ADMIN_CONSOLE_ENABLED must be true or false")
	}
	metricsEnabled := false
	switch configuredMetrics := strings.ToLower(strings.TrimSpace(os.Getenv("ENABLE_METRICS"))); configuredMetrics {
	case "", "false", "0":
	case "true", "1":
		metricsEnabled = true
	default:
		return Config{}, fmt.Errorf("ENABLE_METRICS must be true or false")
	}
	metricsToken := strings.TrimSpace(os.Getenv("METRICS_TOKEN"))
	if metricsEnabled && len([]byte(metricsToken)) < 32 {
		return Config{}, fmt.Errorf("METRICS_TOKEN must be at least 32 bytes when metrics are enabled")
	}

	return Config{
		DatabaseURL:               values["DATABASE_URL"],
		LicenseHMACKey:            values["LICENSE_HMAC_KEY"],
		HardwareHMACKey:           values["HARDWARE_HMAC_KEY"],
		Ed25519PrivateKey:         values["ED25519_PRIVATE_KEY"],
		LicenseIssuer:             values["LICENSE_ISSUER"],
		LicenseAudience:           values["LICENSE_AUDIENCE"],
		Product:                   values["PRODUCT"],
		LoginTimeout:              loginTimeout,
		AdminConsoleEnabled:       adminConsoleEnabled,
		AdminSessionSecret:        values["ADMIN_SESSION_SECRET"],
		AdminMFAEncryptionKey:     values["ADMIN_MFA_ENCRYPTION_KEY"],
		AdminBootstrapToken:       values["ADMIN_BOOTSTRAP_TOKEN"],
		AdminAllowedOrigins:       adminAllowedOrigins,
		AdminSessionTTL:           adminSessionTTL,
		AdminCookieSecure:         adminCookieSecure,
		ClientCredentialsRequired: true,
		MetricsEnabled:            metricsEnabled,
		MetricsToken:              metricsToken,
	}, nil
}

func decodeMFAEncryptionKey(value string) ([]byte, error) {
	if raw := []byte(value); len(raw) == 32 {
		return raw, nil
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		decoded, err := encoding.DecodeString(value)
		if err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("ADMIN_MFA_ENCRYPTION_KEY must be 32 raw bytes or base64-encoded 32 bytes")
}
