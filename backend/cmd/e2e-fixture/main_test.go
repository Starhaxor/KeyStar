package main

import (
	"context"
	"errors"
	"testing"

	"github.com/starloader/backend/internal/domain"
)

type fakeFixtureApplicationProvisioner struct {
	backfillErr error
	createErr   error
	calls       []string
	inputs      []domain.NewApplication
}

func (fake *fakeFixtureApplicationProvisioner) Backfill(context.Context) (int, error) {
	fake.calls = append(fake.calls, "backfill")
	return 1, fake.backfillErr
}

func (fake *fakeFixtureApplicationProvisioner) Create(_ context.Context, input domain.NewApplication) (*domain.Application, error) {
	fake.calls = append(fake.calls, "create")
	fake.inputs = append(fake.inputs, input)
	if fake.createErr != nil {
		return nil, fake.createErr
	}
	return &domain.Application{ID: input.Slug + "-id", Name: input.Name, Slug: input.Slug}, nil
}

type fakeDefaultApplicationFinder struct {
	application *domain.Application
	err         error
}

func (finder fakeDefaultApplicationFinder) FindDefaultApplication(context.Context) (*domain.Application, error) {
	return finder.application, finder.err
}

func TestVerifyRequiredDefaultApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		finder    fakeDefaultApplicationFinder
		wantError bool
	}{
		{
			name:   "migration seeded required application",
			finder: fakeDefaultApplicationFinder{application: &domain.Application{Slug: "starloader"}},
		},
		{
			name:      "required application missing",
			finder:    fakeDefaultApplicationFinder{err: domain.ErrApplicationNotFound},
			wantError: true,
		},
		{
			name:      "wrong default application",
			finder:    fakeDefaultApplicationFinder{application: &domain.Application{Slug: "other"}},
			wantError: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := verifyRequiredDefaultApplication(context.Background(), test.finder)
			if test.wantError && err == nil {
				t.Fatal("verifyRequiredDefaultApplication() error = nil, want an error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("verifyRequiredDefaultApplication() error = %v", err)
			}
		})
	}
}

func TestValidateDedicatedDatabaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		databaseURL string
		wantError   bool
	}{
		{
			name:        "dedicated e2e database",
			databaseURL: "postgres://keystar_test:secret@localhost:5432/keystar_test?sslmode=disable",
		},
		{
			name:        "developer database",
			databaseURL: "postgres://postgres:secret@localhost:5432/keystar?sslmode=disable",
			wantError:   true,
		},
		{
			name:        "similarly named database",
			databaseURL: "postgres://postgres:secret@localhost:5432/keystar_test_backup?sslmode=disable",
			wantError:   true,
		},
		{
			name:        "missing database name",
			databaseURL: "postgres://postgres:secret@localhost:5432/?sslmode=disable",
			wantError:   true,
		},
		{
			name:        "invalid URL",
			databaseURL: "://not-a-postgres-url",
			wantError:   true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateDedicatedDatabaseURL(test.databaseURL)
			if test.wantError && err == nil {
				t.Fatal("validateDedicatedDatabaseURL() error = nil, want an error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("validateDedicatedDatabaseURL() error = %v", err)
			}
		})
	}
}

func TestProvisionFixtureApplicationsBackfillsDefaultBeforeCreatingApplications(t *testing.T) {
	provisioner := &fakeFixtureApplicationProvisioner{}

	alpha, beta, err := provisionFixtureApplications(context.Background(), provisioner, "organization-id")
	if err != nil {
		t.Fatalf("provisionFixtureApplications() error = %v", err)
	}
	if len(provisioner.calls) != 3 || provisioner.calls[0] != "backfill" || provisioner.calls[1] != "create" || provisioner.calls[2] != "create" {
		t.Fatalf("provisioning calls = %#v, want backfill followed by two creates", provisioner.calls)
	}
	if len(provisioner.inputs) != 2 || provisioner.inputs[0].OrganizationID != "organization-id" || provisioner.inputs[0].Slug != "e2e-alpha" || provisioner.inputs[1].Slug != "e2e-beta" {
		t.Fatalf("provisioning inputs = %#v", provisioner.inputs)
	}
	if alpha.ID != "e2e-alpha-id" || beta.ID != "e2e-beta-id" {
		t.Fatalf("provisioned applications = %#v, %#v", alpha, beta)
	}
}

func TestProvisionFixtureApplicationsStopsWhenDefaultBackfillFails(t *testing.T) {
	backfillErr := errors.New("backfill failed")
	provisioner := &fakeFixtureApplicationProvisioner{backfillErr: backfillErr}

	_, _, err := provisionFixtureApplications(context.Background(), provisioner, "organization-id")
	if !errors.Is(err, backfillErr) {
		t.Fatalf("provisionFixtureApplications() error = %v, want %v", err, backfillErr)
	}
	if len(provisioner.calls) != 1 || provisioner.calls[0] != "backfill" {
		t.Fatalf("provisioning calls after backfill failure = %#v", provisioner.calls)
	}
}
