// Command e2e-fixture resets and seeds only the dedicated KeyStar E2E database.
// It is test support for the Playwright suite and deliberately ignores
// DATABASE_URL so a developer database can never become an implicit target.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
	"github.com/starloader/backend/internal/store"
)

const (
	dedicatedDatabaseName = "keystar_test"
	adminEmail            = "e2e-admin@keystar.test"
	adminPassword         = "E2E-Admin-Password!2026"
	adminTOTPSecret       = "JBSWY3DPEHPK3PXP"
	unenrolledAdminEmail  = "e2e-enrollment@keystar.test"
)

type applicationFixture struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	UserEmail     string `json:"userEmail"`
	LicenseID     string `json:"licenseId"`
	DeviceID      string `json:"deviceId"`
	AuthSessionID string `json:"authSessionId"`
}

type fixtureData struct {
	Admin struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		TOTPSecret string `json:"totpSecret"`
	} `json:"admin"`
	UnenrolledAdmin struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	} `json:"unenrolledAdmin"`
	Applications struct {
		Alpha applicationFixture `json:"alpha"`
		Beta  applicationFixture `json:"beta"`
	} `json:"applications"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, output *os.File) error {
	if len(args) != 1 || (args[0] != "reset" && args[0] != "seed") {
		return errors.New("usage: go run ./cmd/e2e-fixture reset|seed")
	}

	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("TEST_DATABASE_URL must be set to the dedicated keystar_test database")
	}
	if err := validateDedicatedDatabaseURL(databaseURL); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("connect to dedicated E2E database")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return errors.New("dedicated E2E database is unavailable")
	}

	if args[0] == "reset" {
		return resetDatabase(ctx, pool)
	}
	fixture, err := seedDatabase(ctx, store.New(pool))
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(fixture)
}

func validateDedicatedDatabaseURL(databaseURL string) error {
	configuration, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return errors.New("parse TEST_DATABASE_URL")
	}
	if configuration.ConnConfig.Database != dedicatedDatabaseName {
		return fmt.Errorf(
			"refusing E2E database %q: TEST_DATABASE_URL must name exactly %q",
			configuration.ConnConfig.Database,
			dedicatedDatabaseName,
		)
	}
	return nil
}

func resetDatabase(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "drop schema public cascade; create schema public"); err != nil {
		return fmt.Errorf("reset dedicated E2E database: %w", err)
	}
	if err := store.MigrateUp(ctx, pool); err != nil {
		return fmt.Errorf("migrate dedicated E2E database: %w", err)
	}
	return nil
}

func seedDatabase(ctx context.Context, repository *store.Store) (fixtureData, error) {
	var fixture fixtureData
	fixture.Admin.Email = adminEmail
	fixture.Admin.Password = adminPassword
	fixture.Admin.TOTPSecret = adminTOTPSecret
	fixture.UnenrolledAdmin.Email = unenrolledAdminEmail
	fixture.UnenrolledAdmin.Password = adminPassword

	passwordHash, err := security.HashPassword(adminPassword)
	if err != nil {
		return fixture, fmt.Errorf("hash E2E admin password: %w", err)
	}
	account, err := repository.CreateAdminAccount(ctx, domain.NewAdminAccount{
		Email: adminEmail, PasswordHash: passwordHash, RoleName: domain.RoleOwner,
	})
	if err != nil {
		return fixture, fmt.Errorf("create E2E administrator: %w", err)
	}
	if err := repository.StartAdminTOTPEnrollment(ctx, account.ID, adminTOTPSecret); err != nil {
		return fixture, fmt.Errorf("start E2E administrator MFA enrollment: %w", err)
	}
	if err := repository.ConfirmAdminTOTPEnrollment(ctx, account.ID); err != nil {
		return fixture, fmt.Errorf("confirm E2E administrator MFA enrollment: %w", err)
	}
	if _, err := repository.CreateAdminAccount(ctx, domain.NewAdminAccount{
		Email: unenrolledAdminEmail, PasswordHash: passwordHash, RoleName: domain.RoleOwner,
	}); err != nil {
		return fixture, fmt.Errorf("create unenrolled E2E administrator: %w", err)
	}

	organization, err := repository.CreateOrganization(ctx, "e2e organization")
	if err != nil {
		return fixture, fmt.Errorf("create E2E organization: %w", err)
	}
	alpha, err := repository.CreateApplication(ctx, domain.NewApplication{
		OrganizationID: organization.ID, Name: "E2E Alpha", Slug: "e2e-alpha",
	})
	if err != nil {
		return fixture, fmt.Errorf("create E2E alpha application: %w", err)
	}
	beta, err := repository.CreateApplication(ctx, domain.NewApplication{
		OrganizationID: organization.ID, Name: "E2E Beta", Slug: "e2e-beta",
	})
	if err != nil {
		return fixture, fmt.Errorf("create E2E beta application: %w", err)
	}

	fixture.Applications.Alpha, err = seedApplication(ctx, repository, alpha.ID, alpha.Name, "alpha-user@keystar.test", "a")
	if err != nil {
		return fixture, err
	}
	fixture.Applications.Beta, err = seedApplication(ctx, repository, beta.ID, beta.Name, "beta-user@keystar.test", "b")
	if err != nil {
		return fixture, err
	}
	return fixture, nil
}

func seedApplication(ctx context.Context, repository *store.Store, applicationID, applicationName, userEmail, marker string) (applicationFixture, error) {
	fixture := applicationFixture{ID: applicationID, Name: applicationName, UserEmail: userEmail}
	userPasswordHash, err := security.HashPassword("E2E-End-User-Password!2026")
	if err != nil {
		return fixture, fmt.Errorf("hash %s user password: %w", marker, err)
	}
	user, err := repository.CreateUser(ctx, applicationID, domain.NewUser{Email: userEmail, PasswordHash: userPasswordHash})
	if err != nil {
		return fixture, fmt.Errorf("create %s E2E user: %w", marker, err)
	}
	productID, planID, err := repository.ResolveProductPlan(ctx, applicationID, "E2E Product "+strings.ToUpper(marker))
	if err != nil {
		return fixture, fmt.Errorf("create %s E2E product: %w", marker, err)
	}
	license, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: strings.Repeat(marker, 64),
		UserID:      user.ID, ProductID: productID, PlanID: planID, MaxDevices: 2,
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		return fixture, fmt.Errorf("create %s E2E license: %w", marker, err)
	}
	fixture.LicenseID = license.ID

	challengeHash := sha256.Sum256([]byte("e2e-challenge-" + marker))
	pending, err := repository.CreatePendingSession(ctx, applicationID, domain.NewPendingSession{
		UserID: user.ID, LicenseID: license.ID, ChallengeSHA256: challengeHash[:],
		ExpiresAt: time.Now().UTC().Add(2 * time.Hour),
	})
	if err != nil {
		return fixture, fmt.Errorf("create %s E2E session: %w", marker, err)
	}
	fixture.AuthSessionID = pending.Session.ID

	publicKey := []byte("e2e-tpm-public-key-" + marker)
	publicKeyHash := sha256.Sum256(publicKey)
	hardwareHMAC := strings.Repeat(marker, 64)
	err = repository.WithLockedChallenge(ctx, applicationID, pending.Session.ID, func(locked *store.LockedChallenge) error {
		device, createErr := locked.CreateDevice(ctx, domain.NewDevice{
			TPMPublicKey: publicKey, TPMPublicKeySHA256: publicKeyHash[:],
			SMBIOSUUIDHMAC: hardwareHMAC, MotherboardSerialHMAC: hardwareHMAC,
			BIOSSerialHMAC: hardwareHMAC, SystemDiskSerialHMAC: hardwareHMAC,
			MachineGuidHMAC: hardwareHMAC, FingerprintHMAC: hardwareHMAC,
			SeenAt: time.Now().UTC(),
		})
		if createErr != nil {
			return createErr
		}
		fixture.DeviceID = device.ID
		return locked.MarkSessionVerified(ctx, time.Now().UTC())
	})
	if err != nil {
		return fixture, fmt.Errorf("verify %s E2E device and session: %w", marker, err)
	}
	return fixture, nil
}
