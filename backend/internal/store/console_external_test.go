package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExternalConsoleUserList(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repository := New(pool)
	application, err := repository.FindApplicationBySlug(context.Background(), "starloader")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = repository.ListConsoleUsers(context.Background(), application.ID, 0, 20, "", ""); err != nil {
		t.Fatal(err)
	}
}

// TestExternalConsoleListFilters seeds a private fixture set and verifies the
// server-side search/status filters of the console list queries against real
// PostgreSQL, including the dynamic placeholder numbering.
func TestExternalConsoleListFilters(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanups run LIFO: the fixture deletion below is registered after this,
	// so rows are removed while the pool is still open.
	t.Cleanup(pool.Close)
	repository := New(pool)
	application, err := repository.FindApplicationBySlug(ctx, "starloader")
	if err != nil {
		t.Fatal(err)
	}

	suffix := time.Now().UnixNano() | 1 // never zero, unique per run
	tag := strconv.FormatInt(suffix, 36)
	aliceEmail := "ext-alice-" + tag + "@example.com"
	bobEmail := "ext-bob-" + tag + "@example.com"

	var productID string
	if err := pool.QueryRow(ctx,
		`insert into products (application_id, name, slug) values ($1, $2, $3) returning id`,
		application.ID, "Ext Product "+tag, "ext-product-"+tag).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	insertUser := func(email string) string {
		var id string
		if err := pool.QueryRow(ctx,
			`insert into users (application_id, email, password_hash) values ($1, $2, 'external-test') returning id`,
			application.ID, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	aliceID := insertUser(aliceEmail)
	bobID := insertUser(bobEmail)
	insertLicense := func(userID, status, notes string) string {
		var id string
		hmacHex := sha256.Sum256([]byte("ext-hmac-" + tag + "-" + userID))
		if err := pool.QueryRow(ctx,
			`insert into licenses (application_id, license_hmac, user_id, product_id, status, max_devices, expires_at, notes)
			 values ($1, $2, $3, $4, $5, 1, now() + interval '30 days', $6) returning id`,
			application.ID, hex.EncodeToString(hmacHex[:]), userID, productID, status, notes).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	aliceLicense := insertLicense(aliceID, "active", "extnote-"+tag)
	bobLicense := insertLicense(bobID, "revoked", "")

	var deviceID string
	deviceFingerprint := sha256.Sum256([]byte("ext-fp-" + tag))
	tpmKey := sha256.Sum256([]byte("ext-tpm-" + tag))
	if err := pool.QueryRow(ctx,
		`insert into devices (application_id, user_id, license_id, tpm_public_key, tpm_public_key_sha256, fingerprint_hmac, status)
		 values ($1, $2, $3, $4, $5, $6, 'active') returning id`,
		application.ID, aliceID, aliceLicense, []byte("ext-tpm-key"), tpmKey[:], hex.EncodeToString(deviceFingerprint[:])).Scan(&deviceID); err != nil {
		t.Fatal(err)
	}
	var sessionID string
	if err := pool.QueryRow(ctx,
		`insert into auth_sessions (application_id, user_id, license_id, status, expires_at)
		 values ($1, $2, $3, 'verified', now() + interval '1 day') returning id`,
		application.ID, aliceID, aliceLicense).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`insert into audit_logs (actor_email, action, resource_type, resource_id) values ($1, 'EXT_TEST_LIST', 'license', $2)`,
		aliceEmail, bobLicense); err != nil {
		t.Fatal(err)
	}
	eventKind := "ext_test_" + tag
	if _, err := pool.Exec(ctx,
		`insert into security_events (kind, severity, actor_email) values ($1, 'critical', $2)`,
		eventKind, bobEmail); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, statement := range []string{
			`delete from auth_sessions where id = ` + quoteUUID(sessionID),
			`delete from devices where id = ` + quoteUUID(deviceID),
			`delete from licenses where id in (` + quoteUUID(aliceLicense) + `, ` + quoteUUID(bobLicense) + `)`,
			`delete from users where id in (` + quoteUUID(aliceID) + `, ` + quoteUUID(bobID) + `)`,
			`delete from products where id = ` + quoteUUID(productID),
			`delete from audit_logs where action = 'EXT_TEST_LIST'`,
			`delete from security_events where kind = '` + eventKind + `'`,
		} {
			if _, err := pool.Exec(cleanupCtx, statement); err != nil {
				t.Logf("cleanup: %v", err)
			}
		}
	})

	licenses, total, err := repository.ListConsoleLicenses(ctx, application.ID, 0, 50, aliceEmail, "")
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 {
		t.Fatalf("license search %q returned %d rows", aliceEmail, total)
	}
	for _, license := range licenses {
		if license.UserEmail != aliceEmail && license.Notes != "extnote-"+tag {
			t.Fatalf("license search returned non-matching row %#v", license.UserEmail)
		}
	}

	revoked, _, err := repository.ListConsoleLicenses(ctx, application.ID, 0, 50, "", "revoked")
	if err != nil {
		t.Fatal(err)
	}
	foundBobLicense := false
	for _, license := range revoked {
		if license.Status != "revoked" {
			t.Fatalf("status filter returned row with status %q", license.Status)
		}
		if license.ID == bobLicense {
			foundBobLicense = true
		}
	}
	if !foundBobLicense {
		t.Fatal("status=revoked filter did not return the revoked fixture license")
	}

	devices, _, err := repository.ListConsoleDevices(ctx, application.ID, 0, 50, aliceEmail, "")
	if err != nil {
		t.Fatal(err)
	}
	deviceMatched := false
	for _, device := range devices {
		if device.UserEmail != aliceEmail {
			t.Fatalf("device search returned non-matching row for %q", device.UserEmail)
		}
		if device.ID == deviceID {
			deviceMatched = true
		}
	}
	if !deviceMatched {
		t.Fatal("device search did not return the fixture device")
	}

	sessions, _, err := repository.ListConsoleSessions(ctx, application.ID, 0, 50, "", "verified")
	if err != nil {
		t.Fatal(err)
	}
	sessionMatched := false
	for _, session := range sessions {
		if session.ID == sessionID {
			sessionMatched = true
		}
	}
	if !sessionMatched {
		t.Fatal("session status filter did not return the fixture session")
	}

	logs, logTotal, err := repository.ListAuditLogs(ctx, 0, 50, tag)
	if err != nil {
		t.Fatal(err)
	}
	if logTotal < 1 {
		t.Fatal("audit log search returned no rows")
	}
	for _, entry := range logs {
		if entry.ActorEmail != aliceEmail {
			t.Fatalf("audit search returned non-matching actor %q", entry.ActorEmail)
		}
	}

	events, eventTotal, err := repository.ListSecurityEvents(ctx, 0, 50, tag, "critical")
	if err != nil {
		t.Fatal(err)
	}
	if eventTotal < 1 {
		t.Fatal("security event search returned no rows")
	}
	for _, event := range events {
		if event.Severity != "critical" {
			t.Fatalf("severity filter returned row with severity %q", event.Severity)
		}
		if event.Kind != eventKind && event.ActorEmail != bobEmail {
			t.Fatalf("event search returned non-matching row kind=%q", event.Kind)
		}
	}
}

func quoteUUID(id string) string {
	return "'" + id + "'"
}
