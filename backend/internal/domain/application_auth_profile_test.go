package domain

import "testing"

func TestValidateApplicationAuthProfileAllowsOnlyDefinedProfiles(t *testing.T) {
	for _, profile := range []ApplicationAuthProfile{ApplicationAuthLegacy, ApplicationAuthProofBound} {
		if err := ValidateApplicationAuthProfile(profile); err != nil {
			t.Fatalf("ValidateApplicationAuthProfile(%q) error = %v", profile, err)
		}
	}
	for _, profile := range []ApplicationAuthProfile{"", "unknown", "LEGACY"} {
		if err := ValidateApplicationAuthProfile(profile); err == nil {
			t.Fatalf("ValidateApplicationAuthProfile(%q) unexpectedly succeeded", profile)
		}
	}
}
