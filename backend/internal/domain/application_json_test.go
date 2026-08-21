package domain

import (
	"encoding/json"
	"testing"
)

func TestOrganizationJSONUsesConsoleFieldNames(t *testing.T) {
	raw, err := json.Marshal(Organization{ID: "org-123", Name: "StarLoader"})
	if err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["id"] != "org-123" || payload["name"] != "StarLoader" {
		t.Fatalf("organization payload = %s, want lowercase console fields", raw)
	}
	if _, exists := payload["ID"]; exists {
		t.Fatalf("organization payload = %s, must not expose Go field names", raw)
	}
}
