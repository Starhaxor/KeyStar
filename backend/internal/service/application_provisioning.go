package service

import (
	"context"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
)

type ApplicationProvisioningRepository interface {
	CreateApplicationWithSigningKey(context.Context, domain.NewApplication, func(string) (domain.NewApplicationSigningKey, error)) (*domain.Application, error)
	ListApplicationsWithoutSigningKey(context.Context) ([]string, error)
	CreateInitialSigningKey(context.Context, string, domain.NewApplicationSigningKey) (bool, error)
}

type ApplicationProvisioner struct {
	repository ApplicationProvisioningRepository
	keys       *security.ApplicationKeyCipher
	now        func() time.Time
}

func NewApplicationProvisioner(
	repository ApplicationProvisioningRepository,
	keys *security.ApplicationKeyCipher,
	now func() time.Time,
) *ApplicationProvisioner {
	if now == nil {
		now = time.Now
	}
	return &ApplicationProvisioner{repository: repository, keys: keys, now: now}
}

func (service *ApplicationProvisioner) Create(ctx context.Context, input domain.NewApplication) (*domain.Application, error) {
	return service.repository.CreateApplicationWithSigningKey(ctx, input, service.newActiveKey)
}

func (service *ApplicationProvisioner) Backfill(ctx context.Context) (int, error) {
	applicationIDs, err := service.repository.ListApplicationsWithoutSigningKey(ctx)
	if err != nil {
		return 0, err
	}

	created := 0
	for _, applicationID := range applicationIDs {
		key, err := service.newActiveKey(applicationID)
		if err != nil {
			return created, err
		}
		inserted, err := service.repository.CreateInitialSigningKey(ctx, applicationID, key)
		if err != nil {
			return created, err
		}
		if inserted {
			created++
		}
	}
	return created, nil
}

func (service *ApplicationProvisioner) newActiveKey(applicationID string) (domain.NewApplicationSigningKey, error) {
	key, err := service.keys.Generate(applicationID)
	if err != nil {
		return domain.NewApplicationSigningKey{}, err
	}
	activatedAt := service.now().UTC()
	key.Status = domain.ApplicationSigningKeyActive
	key.ActivatedAt = &activatedAt
	return key, nil
}
