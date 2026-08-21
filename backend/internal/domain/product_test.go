package domain

import (
	"encoding/json"
	"testing"
)

func TestCatalogModelsUseConsoleJSONFieldNames(t *testing.T) {
	payload, err := json.Marshal(struct {
		Product Product `json:"product"`
		Plan    Plan    `json:"plan"`
	}{
		Product: Product{ID: "product-1", ApplicationID: "application-1", Name: "StarLoader"},
		Plan:    Plan{ID: "plan-1", ProductID: "product-1", Name: "Pro", Code: "pro"},
	})
	if err != nil {
		t.Fatalf("marshal catalog models: %v", err)
	}

	var decoded map[string]map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal catalog payload: %v", err)
	}
	if got := decoded["product"]["id"]; got != "product-1" {
		t.Fatalf("product id = %v, want product-1", got)
	}
	if got := decoded["product"]["application_id"]; got != "application-1" {
		t.Fatalf("product application_id = %v, want application-1", got)
	}
	if got := decoded["plan"]["id"]; got != "plan-1" {
		t.Fatalf("plan id = %v, want plan-1", got)
	}
	if got := decoded["plan"]["product_id"]; got != "product-1" {
		t.Fatalf("plan product_id = %v, want product-1", got)
	}
}
