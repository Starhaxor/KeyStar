package domain

import "testing"

func TestAllPermissionsIncludesPlatformManagementPermissions(t *testing.T) {
	want := []string{
		"applications.read", "applications.write",
		"catalog.read", "catalog.write",
		"webhooks.read", "webhooks.write",
	}
	available := make(map[string]bool, len(AllPermissions))
	for _, permission := range AllPermissions {
		available[permission] = true
	}
	for _, permission := range want {
		if !available[permission] {
			t.Fatalf("missing permission %q", permission)
		}
	}
}
