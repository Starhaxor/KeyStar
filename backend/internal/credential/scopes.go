package credential

import "slices"

// Client scopes allowed on publishable (client-side) keys. A publishable key
// is not a secret: it may never administer, read other applications or call
// server endpoints.
var PublishableScopes = []string{
	"auth.login",
	"auth.register",
	"auth.refresh",
	"auth.logout",
	"license.activate",
	"license.me",
	"device.verify",
	"device.me",
	"variables.read_public",
	"me.read",
}

// Server scopes available to secret (server-to-server) keys. Every server
// endpoint declares the exact scope it requires instead of one broad admin
// scope.
var ServerScopes = []string{
	"users.read",
	"users.write",
	"licenses.read",
	"licenses.write",
	"devices.read",
	"devices.write",
	"sessions.read",
	"sessions.revoke",
	"variables.read",
	"variables.write",
	"webhooks.read",
	"webhooks.write",
	"analytics.read",
}

// ValidScopes returns true when every scope is known for the credential type.
func ValidScopes(credentialType string, scopes []string) bool {
	allowed := PublishableScopes
	if credentialType == "secret" {
		allowed = ServerScopes
	}
	for _, scope := range scopes {
		if !slices.Contains(allowed, scope) {
			return false
		}
	}
	return true
}

// Requires reports whether the granted scope set contains the required scope.
func Requires(granted []string, required string) bool {
	return slices.Contains(granted, required)
}
