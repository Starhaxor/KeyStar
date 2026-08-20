package domain

import "testing"

func TestDefaultDevicePolicy(t *testing.T) {
	policy := DefaultDevicePolicy("app-123")
	if policy.ApplicationID != "app-123" {
		t.Fatalf("ApplicationID = %q, want %q", policy.ApplicationID, "app-123")
	}
	if policy.TPMPolicy != TPMOptional {
		t.Fatalf("TPMPolicy = %q, want %q", policy.TPMPolicy, TPMOptional)
	}
	if policy.MinMatchScore != 70 {
		t.Fatalf("MinMatchScore = %d, want 70", policy.MinMatchScore)
	}
	if policy.StepUpScore != 40 {
		t.Fatalf("StepUpScore = %d, want 40", policy.StepUpScore)
	}
	if !policy.AllowAutoRebind {
		t.Fatal("AllowAutoRebind should default to true")
	}
	if policy.RebindCooldownSeconds != 86400 {
		t.Fatalf("RebindCooldownSeconds = %d, want 86400", policy.RebindCooldownSeconds)
	}
	if policy.MaxDeviceChangesPer30d != 5 {
		t.Fatalf("MaxDeviceChangesPer30d = %d, want 5", policy.MaxDeviceChangesPer30d)
	}
}

func TestNewDevicePolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  NewDevicePolicy
		wantErr error
	}{
		{
			name:    "valid optional policy",
			policy:  NewDevicePolicy{TPMPolicy: TPMOptional, MinMatchScore: 70, StepUpScore: 40},
			wantErr: nil,
		},
		{
			name:    "valid required policy",
			policy:  NewDevicePolicy{TPMPolicy: TPMRequired, MinMatchScore: 80, StepUpScore: 50},
			wantErr: nil,
		},
		{
			name:    "valid preferred policy",
			policy:  NewDevicePolicy{TPMPolicy: TPMPreferred, MinMatchScore: 60, StepUpScore: 30},
			wantErr: nil,
		},
		{
			name:    "invalid tpm policy",
			policy:  NewDevicePolicy{TPMPolicy: "invalid", MinMatchScore: 70, StepUpScore: 40},
			wantErr: ErrDevicePolicyInvalidTPMPolicy,
		},
		{
			name:    "score too high",
			policy:  NewDevicePolicy{TPMPolicy: TPMOptional, MinMatchScore: 101, StepUpScore: 40},
			wantErr: ErrDevicePolicyInvalidScore,
		},
		{
			name:    "score negative",
			policy:  NewDevicePolicy{TPMPolicy: TPMOptional, MinMatchScore: -1, StepUpScore: 40},
			wantErr: ErrDevicePolicyInvalidScore,
		},
		{
			name:    "step up too high",
			policy:  NewDevicePolicy{TPMPolicy: TPMOptional, MinMatchScore: 50, StepUpScore: 60},
			wantErr: ErrDevicePolicyStepUpTooHigh,
		},
		{
			name:    "step up equal to min",
			policy:  NewDevicePolicy{TPMPolicy: TPMOptional, MinMatchScore: 50, StepUpScore: 50},
			wantErr: ErrDevicePolicyStepUpTooHigh,
		},
		{
			name:    "negative cooldown",
			policy:  NewDevicePolicy{TPMPolicy: TPMOptional, MinMatchScore: 70, StepUpScore: 40, RebindCooldownSeconds: -1},
			wantErr: ErrDevicePolicyInvalidCooldown,
		},
		{
			name:    "negative change limit",
			policy:  NewDevicePolicy{TPMPolicy: TPMOptional, MinMatchScore: 70, StepUpScore: 40, MaxDeviceChangesPer30d: -1},
			wantErr: ErrDevicePolicyInvalidChangeLimit,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.policy.Validate()
			if err != test.wantErr {
				t.Fatalf("Validate() = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestValidTPMPolicies(t *testing.T) {
	valid := []TPMPolicy{TPMRequired, TPMPreferred, TPMOptional}
	for _, p := range valid {
		if !ValidTPMPolicies[p] {
			t.Errorf("ValidTPMPolicies[%q] = false, want true", p)
		}
	}
	invalid := []TPMPolicy{"", "mandatory", "yes"}
	for _, p := range invalid {
		if ValidTPMPolicies[p] {
			t.Errorf("ValidTPMPolicies[%q] = true, want false", p)
		}
	}
}
