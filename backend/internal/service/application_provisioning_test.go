package service

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
)

type applicationProvisioningRepositoryFake struct {
	createdApplication domain.Application
	createdKey         domain.NewApplicationSigningKey
	withoutKey         []string
	existingActive     map[string]bool
	initialKeys        map[string]domain.NewApplicationSigningKey
}

func (fake *applicationProvisioningRepositoryFake) CreateApplicationWithSigningKey(
	_ context.Context,
	_ domain.NewApplication,
	keyFactory func(string) (domain.NewApplicationSigningKey, error),
) (*domain.Application, error) {
	key, err := keyFactory(fake.createdApplication.ID)
	if err != nil {
		return nil, err
	}
	fake.createdKey = key
	application := fake.createdApplication
	return &application, nil
}

func (fake *applicationProvisioningRepositoryFake) ListApplicationsWithoutSigningKey(context.Context) ([]string, error) {
	return append([]string(nil), fake.withoutKey...), nil
}

func (fake *applicationProvisioningRepositoryFake) CreateInitialSigningKey(
	_ context.Context,
	applicationID string,
	key domain.NewApplicationSigningKey,
) (bool, error) {
	if fake.existingActive[applicationID] {
		return false, nil
	}
	if fake.initialKeys == nil {
		fake.initialKeys = make(map[string]domain.NewApplicationSigningKey)
	}
	fake.initialKeys[applicationID] = key
	fake.existingActive[applicationID] = true
	return true, nil
}

func TestApplicationProvisionerCreateUsesDatabaseAssignedApplicationID(t *testing.T) {
	const applicationID = "0198fc4b-d115-7000-8000-000000000123"
	repository := &applicationProvisioningRepositoryFake{
		createdApplication: domain.Application{ID: applicationID, Name: "Desktop", Slug: "desktop"},
		existingActive:     make(map[string]bool),
	}
	cipher := newProvisioningTestCipher(t, bytes.Repeat([]byte{0x31}, 60))
	localTime := time.Date(2026, time.August, 31, 14, 25, 30, 0, time.FixedZone("TRT", 3*60*60))
	provisioner := NewApplicationProvisioner(repository, cipher, func() time.Time { return localTime })

	application, err := provisioner.Create(context.Background(), domain.NewApplication{
		OrganizationID: "0198fc4b-d115-7000-8000-000000000001",
		Name:           "Desktop",
		Slug:           "desktop",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if application.ID != applicationID {
		t.Fatalf("Create() application ID = %q, want database-assigned ID %q", application.ID, applicationID)
	}
	if repository.createdKey.ApplicationID != applicationID {
		t.Fatalf("key application ID = %q, want database-assigned ID %q", repository.createdKey.ApplicationID, applicationID)
	}
	if repository.createdKey.Status != domain.ApplicationSigningKeyActive {
		t.Fatalf("key status = %q, want %q", repository.createdKey.Status, domain.ApplicationSigningKeyActive)
	}
	wantActivatedAt := localTime.UTC()
	if repository.createdKey.ActivatedAt == nil || !repository.createdKey.ActivatedAt.Equal(wantActivatedAt) {
		t.Fatalf("key activated at = %v, want %v", repository.createdKey.ActivatedAt, wantActivatedAt)
	}
	if repository.createdKey.ActivatedAt.Location() != time.UTC {
		t.Fatalf("key activation location = %v, want UTC", repository.createdKey.ActivatedAt.Location())
	}
}

func TestApplicationProvisionerBackfillDoesNotReplaceExistingActiveKey(t *testing.T) {
	const firstApplicationID = "0198fc4b-d115-7000-8000-000000000201"
	const secondApplicationID = "0198fc4b-d115-7000-8000-000000000202"
	repository := &applicationProvisioningRepositoryFake{
		withoutKey:     []string{firstApplicationID, secondApplicationID},
		existingActive: map[string]bool{secondApplicationID: true},
	}
	cipher := newProvisioningTestCipher(t, bytes.Repeat([]byte{0x42}, 120))
	activatedAt := time.Date(2026, time.August, 31, 10, 0, 0, 0, time.UTC)
	provisioner := NewApplicationProvisioner(repository, cipher, func() time.Time { return activatedAt })

	created, err := provisioner.Backfill(context.Background())
	if err != nil {
		t.Fatalf("Backfill() error = %v", err)
	}
	if created != 1 {
		t.Fatalf("Backfill() created = %d, want 1", created)
	}
	key, ok := repository.initialKeys[firstApplicationID]
	if !ok {
		t.Fatalf("Backfill() did not create a key for %q", firstApplicationID)
	}
	if key.ApplicationID != firstApplicationID || key.Status != domain.ApplicationSigningKeyActive {
		t.Fatalf("created key application/status = %q/%q, want %q/%q", key.ApplicationID, key.Status, firstApplicationID, domain.ApplicationSigningKeyActive)
	}
	if _, replaced := repository.initialKeys[secondApplicationID]; replaced {
		t.Fatalf("Backfill() replaced the existing active key for %q", secondApplicationID)
	}
}

func newProvisioningTestCipher(t *testing.T, randomness []byte) *security.ApplicationKeyCipher {
	t.Helper()
	cipher, err := security.NewApplicationKeyCipher(
		map[int][]byte{1: bytes.Repeat([]byte{0x19}, 32)},
		1,
		bytes.NewReader(randomness),
	)
	if err != nil {
		t.Fatalf("NewApplicationKeyCipher() error = %v", err)
	}
	return cipher
}
