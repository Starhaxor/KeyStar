package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/starloader/backend/internal/domain"
)

const devicePolicyColumns = `
	id::text, application_id::text, tpm_policy, min_match_score, step_up_score,
	allow_auto_rebind, rebind_cooldown_seconds, max_device_changes_per_30d,
	created_at, updated_at`

// GetDevicePolicy returns the device policy for an application. When no row
// exists, a default policy is returned without touching the database so the
// behaviour is identical to the hard-coded defaults that existed before this
// table was introduced.
func (s *Store) GetDevicePolicy(ctx context.Context, applicationID string) (*domain.DevicePolicy, error) {
	policy, err := scanDevicePolicy(s.db.QueryRow(ctx,
		`select `+devicePolicyColumns+` from application_device_policies where application_id = $1::uuid`,
		applicationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DefaultDevicePolicy(applicationID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("get device policy: %w", err)
	}
	return policy, nil
}

// UpsertDevicePolicy creates or replaces the device policy for an application.
func (s *Store) UpsertDevicePolicy(ctx context.Context, applicationID string, input domain.NewDevicePolicy) (*domain.DevicePolicy, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	policy, err := scanDevicePolicy(s.db.QueryRow(ctx, `
		insert into application_device_policies (
			application_id, tpm_policy, min_match_score, step_up_score,
			allow_auto_rebind, rebind_cooldown_seconds, max_device_changes_per_30d
		) values ($1::uuid, $2, $3, $4, $5, $6, $7)
		on conflict (application_id) do update set
			tpm_policy = excluded.tpm_policy,
			min_match_score = excluded.min_match_score,
			step_up_score = excluded.step_up_score,
			allow_auto_rebind = excluded.allow_auto_rebind,
			rebind_cooldown_seconds = excluded.rebind_cooldown_seconds,
			max_device_changes_per_30d = excluded.max_device_changes_per_30d,
			updated_at = now()
		returning `+devicePolicyColumns,
		applicationID, input.TPMPolicy, input.MinMatchScore, input.StepUpScore,
		input.AllowAutoRebind, input.RebindCooldownSeconds, input.MaxDeviceChangesPer30d))
	if err != nil {
		return nil, fmt.Errorf("upsert device policy: %w", err)
	}
	return policy, nil
}

// DeleteDevicePolicy removes the device policy for an application, reverting
// to the hard-coded defaults on next read.
func (s *Store) DeleteDevicePolicy(ctx context.Context, applicationID string) error {
	var deleted bool
	err := s.db.QueryRow(ctx,
		`delete from application_device_policies where application_id = $1::uuid returning true`,
		applicationID).Scan(&deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrDevicePolicyNotFound
	}
	if err != nil {
		return fmt.Errorf("delete device policy: %w", err)
	}
	return nil
}

func scanDevicePolicy(row pgx.Row) (*domain.DevicePolicy, error) {
	var policy domain.DevicePolicy
	err := row.Scan(
		&policy.ID, &policy.ApplicationID, &policy.TPMPolicy,
		&policy.MinMatchScore, &policy.StepUpScore,
		&policy.AllowAutoRebind, &policy.RebindCooldownSeconds,
		&policy.MaxDeviceChangesPer30d,
		&policy.CreatedAt, &policy.UpdatedAt,
	)
	return &policy, err
}
