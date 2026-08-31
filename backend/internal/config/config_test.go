package config

import (
	"bytes"
	"encoding/base64"
	"testing"
	"time"
)

func TestLoadRequiresEverySecuritySetting(t *testing.T) {
	for _, name := range []string{
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
		"APPLICATION_KEY_ENCRYPTION_KEYS",
		"APPLICATION_KEY_ACTIVE_VERSION",
	} {
		t.Run(name, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv(name, "")
			if _, err := Load(); err == nil {
				t.Fatalf("Load() accepted missing %s", name)
			}
		})
	}
}

func TestLoadParsesApplicationKeyEncryptionKeys(t *testing.T) {
	setRequiredEnvironment(t)
	first := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	second := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32))
	t.Setenv("APPLICATION_KEY_ENCRYPTION_KEYS", "1="+first+",2="+second)
	t.Setenv("APPLICATION_KEY_ACTIVE_VERSION", "2")

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.ApplicationKeyActiveVersion != 2 || !bytes.Equal(configuration.ApplicationKeyEncryptionKeys[2], bytes.Repeat([]byte{0x22}, 32)) {
		t.Fatal("application signing-key configuration was not parsed correctly")
	}
}

func TestLoadRejectsInvalidApplicationKeyEncryptionConfiguration(t *testing.T) {
	validKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	shortKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 31))
	for _, change := range []struct {
		name, keys, activeVersion string
	}{
		{"duplicate versions", "1=" + validKey + ",1=" + validKey, "1"},
		{"zero version", "0=" + validKey, "1"},
		{"negative version", "-1=" + validKey, "1"},
		{"malformed base64", "1=not-base64!", "1"},
		{"wrong decoded length", "1=" + shortKey, "1"},
		{"missing active version", "1=" + validKey, "2"},
	} {
		t.Run(change.name, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv("APPLICATION_KEY_ENCRYPTION_KEYS", change.keys)
			t.Setenv("APPLICATION_KEY_ACTIVE_VERSION", change.activeVersion)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() accepted invalid application key configuration %q", change.name)
			}
		})
	}
}

func TestLoadRejectsReusedApplicationKeyEncryptionKey(t *testing.T) {
	setRequiredEnvironment(t)
	reusedKey := "0123456789abcdef0123456789abcdef"
	t.Setenv("APPLICATION_KEY_ENCRYPTION_KEYS", "1="+base64.StdEncoding.EncodeToString([]byte(reusedKey)))
	t.Setenv("APPLICATION_KEY_ACTIVE_VERSION", "1")

	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted an application encryption key reused as another secret")
	}
}

func TestLoadReturnsConfiguredValues(t *testing.T) {
	setRequiredEnvironment(t)
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.DatabaseURL != "postgres://user:pass@localhost:5432/starloader" || config.Product != "StarLoader" {
		t.Fatalf("Load() = %#v", config)
	}
}

func TestLoadDecodesBase64MFAEncryptionKey(t *testing.T) {
	setRequiredEnvironment(t)
	raw := []byte("abcdef0123456789abcdef0123456789")
	t.Setenv("ADMIN_MFA_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(raw))

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.AdminMFAEncryptionKey != string(raw) {
		t.Fatal("base64 MFA encryption key was not normalized to 32 key bytes")
	}
}

func TestLoadRequiresClientCredentialsByDefault(t *testing.T) {
	setRequiredEnvironment(t)

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !configuration.ClientCredentialsRequired {
		t.Fatal("ClientCredentialsRequired should default to true")
	}
}

func TestLoadRejectsReusedHMACKey(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("HARDWARE_HMAC_KEY", "license-key-0123456789abcdef0123456789")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted identical license and hardware HMAC keys")
	}
}

func TestLoadDefaultsLoginTimeoutToTenSeconds(t *testing.T) {
	setRequiredEnvironment(t)

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.LoginTimeout != 10*time.Second {
		t.Fatalf("LoginTimeout = %s, want 10s", configuration.LoginTimeout)
	}
}

func TestLoadParsesConfiguredLoginTimeout(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("LOGIN_TIMEOUT", " 750ms ")

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.LoginTimeout != 750*time.Millisecond {
		t.Fatalf("LoginTimeout = %s, want 750ms", configuration.LoginTimeout)
	}
}

func TestLoadRejectsNonPositiveOrInvalidLoginTimeout(t *testing.T) {
	for _, value := range []string{"0", "0s", "-1s", "ten seconds"} {
		t.Run(value, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv("LOGIN_TIMEOUT", value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() accepted LOGIN_TIMEOUT=%q", value)
			}
		})
	}
}

func TestLoadDefaultsAdminConsoleSettings(t *testing.T) {
	setRequiredEnvironment(t)

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	if len(configuration.AdminAllowedOrigins) != len(want) {
		t.Fatalf("AdminAllowedOrigins = %q, want %q", configuration.AdminAllowedOrigins, want)
	}
	for i := range want {
		if configuration.AdminAllowedOrigins[i] != want[i] {
			t.Fatalf("AdminAllowedOrigins = %q, want %q", configuration.AdminAllowedOrigins, want)
		}
	}
	if configuration.AdminSessionTTL != 12*time.Hour {
		t.Fatalf("AdminSessionTTL = %s, want 12h", configuration.AdminSessionTTL)
	}
	if !configuration.AdminCookieSecure {
		t.Fatal("AdminCookieSecure should default to true")
	}
}

func TestLoadRejectsWeakOrReusedSecrets(t *testing.T) {
	for _, change := range []struct{ name, value string }{
		{"LICENSE_HMAC_KEY", "too-short"},
		{"HARDWARE_HMAC_KEY", "too-short"},
		{"ADMIN_SESSION_SECRET", "too-short"},
		{"ADMIN_MFA_ENCRYPTION_KEY", "too-short"},
		{"ADMIN_BOOTSTRAP_TOKEN", "too-short"},
		{"ADMIN_SESSION_SECRET", "0123456789abcdef0123456789abcdef"},
	} {
		t.Run(change.name+"="+change.value, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv(change.name, change.value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted weak/reused %s", change.name)
			}
		})
	}
}

func TestLoadRequiresMetricsTokenWhenEnabled(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("ENABLE_METRICS", "1")
	t.Setenv("METRICS_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("metrics enabled without a token")
	}
	t.Setenv("METRICS_TOKEN", "metrics-token-0123456789abcdef0123456789")
	configuration, err := Load()
	if err != nil || !configuration.MetricsEnabled {
		t.Fatalf("Load()=(%#v,%v)", configuration, err)
	}
}

func TestLoadParsesAdminConsoleSettings(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("ADMIN_ALLOWED_ORIGIN", "https://admin.example.com/")
	t.Setenv("ADMIN_SESSION_TTL", "2h")
	t.Setenv("ADMIN_COOKIE_SECURE", "true")

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(configuration.AdminAllowedOrigins) != 1 || configuration.AdminAllowedOrigins[0] != "https://admin.example.com" || configuration.AdminSessionTTL != 2*time.Hour || !configuration.AdminCookieSecure {
		t.Fatalf("Load() = %#v", configuration)
	}
}

func TestLoadRejectsInvalidAdminConsoleSettings(t *testing.T) {
	for _, setting := range []struct{ name, value string }{
		{"ADMIN_SESSION_TTL", "0s"},
		{"ADMIN_SESSION_TTL", "weekly"},
		{"ADMIN_COOKIE_SECURE", "maybe"},
	} {
		t.Run(setting.name+"="+setting.value, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv(setting.name, setting.value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() accepted %s=%q", setting.name, setting.value)
			}
		})
	}
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("LOGIN_TIMEOUT", "")
	t.Setenv("ADMIN_ALLOWED_ORIGIN", "")
	t.Setenv("ADMIN_SESSION_TTL", "")
	t.Setenv("ADMIN_COOKIE_SECURE", "")
	t.Setenv("ENABLE_METRICS", "")
	t.Setenv("METRICS_TOKEN", "")
	for _, setting := range []struct{ name, value string }{
		{"DATABASE_URL", "postgres://user:pass@localhost:5432/starloader"},
		{"LICENSE_HMAC_KEY", "license-key-0123456789abcdef0123456789"},
		{"HARDWARE_HMAC_KEY", "hardware-key-0123456789abcdef01234567"},
		{"ED25519_PRIVATE_KEY", "ed25519-private-key"},
		{"LICENSE_ISSUER", "starloader"},
		{"LICENSE_AUDIENCE", "starloader-client"},
		{"PRODUCT", "StarLoader"},
		{"ADMIN_SESSION_SECRET", "session-key-0123456789abcdef0123456789"},
		{"ADMIN_MFA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef"},
		{"ADMIN_BOOTSTRAP_TOKEN", "bootstrap-token-0123456789abcdef012345"},
		{"APPLICATION_KEY_ENCRYPTION_KEYS", "1=" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x33}, 32))},
		{"APPLICATION_KEY_ACTIVE_VERSION", "1"},
	} {
		t.Setenv(setting.name, setting.value)
	}
}
