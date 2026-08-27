package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
)

func TestRefreshServiceIssuesClaimsAcceptedByRealVerifier(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := security.NewTokenIssuer(privateKey, "starloader", "starloader-client", "StarLoader")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := security.NewTokenVerifier(publicKey, "starloader", "starloader-client", "StarLoader")
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakeRefreshRepository()
	svc := newTestRefreshService(repo, issuer)
	now := time.Now().UTC().Truncate(time.Second)
	svc.now = func() time.Time { return now }
	token, _, err := svc.IssueRefreshToken(context.Background(), "app-1", "user-1", "license-1", "device-1")
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Refresh(context.Background(), RefreshInput{ApplicationID: "app-1", RefreshToken: token})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifier.Verify(result.AccessToken)
	if err != nil {
		t.Fatalf("real verifier rejected refreshed token: %v", err)
	}
	if claims.LicenseID != "license-1" || claims.Product != "StarLoader" || claims.Issuer != "starloader" {
		t.Fatalf("claims=%#v", claims)
	}
}

// fakeRefreshRepository implements RefreshTokenRepository for testing.
type fakeRefreshRepository struct {
	sessions map[string]*domain.RefreshSession
	counter  int
}

func newFakeRefreshRepository() *fakeRefreshRepository {
	return &fakeRefreshRepository{sessions: make(map[string]*domain.RefreshSession)}
}

func (repo *fakeRefreshRepository) CreateRefreshSession(_ context.Context, input domain.NewRefreshSession) (*domain.RefreshSession, error) {
	repo.counter++
	id := "rs-" + string(rune('a'+repo.counter))
	session := &domain.RefreshSession{
		ID:            id,
		ApplicationID: input.ApplicationID,
		UserID:        input.UserID,
		LicenseID:     input.LicenseID,
		DeviceID:      input.DeviceID,
		TokenHash:     input.TokenHash,
		Status:        domain.RefreshSessionStatusActive,
		ExpiresAt:     input.ExpiresAt,
		CreatedAt:     time.Now(),
	}
	repo.sessions[id] = session
	return session, nil
}

func (repo *fakeRefreshRepository) LoadProfile(_ context.Context, applicationID, userID, licenseID, deviceID string) (*domain.UserProfile, error) {
	return &domain.UserProfile{ApplicationID: applicationID, AccountStatus: domain.UserStatusActive, Product: "StarLoader",
		LicenseStatus: domain.LicenseStatusActive, LicenseExpiresAt: time.Now().Add(24 * time.Hour),
		DeviceID: deviceID, DeviceStatus: domain.DeviceStatusActive}, nil
}

func (repo *fakeRefreshRepository) FindRefreshSessionByTokenHash(_ context.Context, tokenHash []byte) (*domain.RefreshSession, error) {
	for _, s := range repo.sessions {
		if bytesEqual(s.TokenHash, tokenHash) {
			return s, nil
		}
	}
	return nil, domain.ErrRefreshSessionNotFound
}

func (repo *fakeRefreshRepository) RotateRefreshSession(_ context.Context, sessionID string, _ time.Time) (*domain.RefreshSession, error) {
	s, ok := repo.sessions[sessionID]
	if !ok {
		return nil, domain.ErrRefreshSessionNotFound
	}
	s.Status = domain.RefreshSessionStatusRotated
	return s, nil
}

func (repo *fakeRefreshRepository) RevokeRefreshSession(_ context.Context, applicationID, sessionID string) error {
	session, ok := repo.sessions[sessionID]
	if !ok || session.ApplicationID != applicationID {
		return domain.ErrRefreshSessionNotFound
	}
	session.Status = domain.RefreshSessionStatusRevoked
	return nil
}

func (repo *fakeRefreshRepository) RevokeRefreshSessionFamily(_ context.Context, _ string, userID, deviceID string) (int64, error) {
	var count int64
	for _, s := range repo.sessions {
		if s.UserID == userID && s.DeviceID == deviceID && s.Status == domain.RefreshSessionStatusActive {
			s.Status = domain.RefreshSessionStatusRevoked
			count++
		}
	}
	return count, nil
}

func (repo *fakeRefreshRepository) RevokeAllUserRefreshSessions(_ context.Context, _ string, userID string) (int64, error) {
	var count int64
	for _, s := range repo.sessions {
		if s.UserID == userID && s.Status == domain.RefreshSessionStatusActive {
			s.Status = domain.RefreshSessionStatusRevoked
			count++
		}
	}
	return count, nil
}

// fakeSessionTokenIssuer implements SessionTokenIssuer for testing.
type fakeSessionTokenIssuer struct {
	token string
	err   error
}

func (issuer *fakeSessionTokenIssuer) Issue(_ security.SessionClaims) (string, error) {
	return issuer.token, issuer.err
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newTestRefreshService(repo *fakeRefreshRepository, issuer SessionTokenIssuer) *RefreshService {
	return NewRefreshService(RefreshServiceConfig{Repository: repo, Profile: repo, HMACKey: []byte("hmac-key"), TokenIssuer: issuer,
		Issuer: "starloader", Audience: "starloader-client", Product: "StarLoader"})
}

func TestRefreshServiceIssueRefreshToken(t *testing.T) {
	repo := newFakeRefreshRepository()
	issuer := &fakeSessionTokenIssuer{token: "access-token"}
	svc := newTestRefreshService(repo, issuer)

	token, expiresAt, err := svc.IssueRefreshToken(context.Background(), "app-1", "user-1", "license-1", "device-1")
	if err != nil {
		t.Fatalf("IssueRefreshToken() error = %v", err)
	}
	if token == "" {
		t.Fatal("IssueRefreshToken() returned empty token")
	}
	if expiresAt.Before(time.Now()) {
		t.Fatal("IssueRefreshToken() expiry is in the past")
	}
	if len(repo.sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(repo.sessions))
	}
}

func TestRefreshServiceRefreshRotation(t *testing.T) {
	repo := newFakeRefreshRepository()
	issuer := &fakeSessionTokenIssuer{token: "new-access-token"}
	now := time.Now().UTC().Truncate(time.Second)
	svc := newTestRefreshService(repo, issuer)
	svc.now = func() time.Time { return now }

	// Issue a refresh token.
	token, _, err := svc.IssueRefreshToken(context.Background(), "app-1", "user-1", "license-1", "device-1")
	if err != nil {
		t.Fatal(err)
	}

	// Refresh it.
	result, err := svc.Refresh(context.Background(), RefreshInput{
		ApplicationID: "app-1",
		RefreshToken:  token,
	})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if result.AccessToken == "" {
		t.Fatal("Refresh() returned empty access token")
	}
	if result.RefreshToken == "" {
		t.Fatal("Refresh() returned empty refresh token")
	}
	if result.RefreshToken == token {
		t.Fatal("Refresh() returned the same refresh token (no rotation)")
	}

	// The old token should be rotated. Find it by iterating sessions.
	var oldSession *domain.RefreshSession
	for _, s := range repo.sessions {
		if s.Status == domain.RefreshSessionStatusRotated {
			oldSession = s
			break
		}
	}
	if oldSession == nil {
		t.Fatal("no rotated session found")
	}
}

func TestRefreshServiceReuseDetection(t *testing.T) {
	repo := newFakeRefreshRepository()
	issuer := &fakeSessionTokenIssuer{token: "access"}
	now := time.Now().UTC().Truncate(time.Second)
	svc := newTestRefreshService(repo, issuer)
	svc.now = func() time.Time { return now }

	// Issue and rotate.
	token, _, _ := svc.IssueRefreshToken(context.Background(), "app-1", "user-1", "license-1", "device-1")
	_, err := svc.Refresh(context.Background(), RefreshInput{
		ApplicationID: "app-1",
		RefreshToken:  token,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reusing the old (rotated) token should fail and revoke the family.
	_, err = svc.Refresh(context.Background(), RefreshInput{
		ApplicationID: "app-1",
		RefreshToken:  token,
	})
	if err != domain.ErrRefreshTokenReuse {
		t.Fatalf("Refresh() on rotated token = %v, want ErrRefreshTokenReuse", err)
	}

	// All active sessions for user-1/device-1 should be revoked.
	for _, s := range repo.sessions {
		if s.UserID == "user-1" && s.DeviceID == "device-1" {
			if s.Status == domain.RefreshSessionStatusActive {
				t.Fatal("reuse detection did not revoke the family")
			}
		}
	}
}

func TestRefreshServiceRevokedToken(t *testing.T) {
	repo := newFakeRefreshRepository()
	issuer := &fakeSessionTokenIssuer{token: "access"}
	svc := newTestRefreshService(repo, issuer)

	token, _, _ := svc.IssueRefreshToken(context.Background(), "app-1", "user-1", "license-1", "device-1")

	// Find and manually revoke.
	for _, s := range repo.sessions {
		s.Status = domain.RefreshSessionStatusRevoked
		break
	}

	_, err := svc.Refresh(context.Background(), RefreshInput{
		ApplicationID: "app-1",
		RefreshToken:  token,
	})
	if err != domain.ErrRefreshTokenRevoked {
		t.Fatalf("Refresh() on revoked token = %v, want ErrRefreshTokenRevoked", err)
	}
}

func TestRefreshServiceRevokeScopesAndInvalidatesToken(t *testing.T) {
	repo := newFakeRefreshRepository()
	svc := newTestRefreshService(repo, &fakeSessionTokenIssuer{token: "access"})
	token, _, err := svc.IssueRefreshToken(context.Background(), "app-1", "user-1", "license-1", "device-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Revoke(context.Background(), RefreshInput{ApplicationID: "app-2", RefreshToken: token}); err != ErrInvalidRefreshToken {
		t.Fatalf("cross-tenant revoke error=%v", err)
	}
	if err := svc.Revoke(context.Background(), RefreshInput{ApplicationID: "app-1", RefreshToken: token}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(context.Background(), RefreshInput{ApplicationID: "app-1", RefreshToken: token}); err != domain.ErrRefreshTokenRevoked {
		t.Fatalf("revoked refresh error=%v", err)
	}
}

func TestRefreshServiceInvalidToken(t *testing.T) {
	repo := newFakeRefreshRepository()
	issuer := &fakeSessionTokenIssuer{token: "access"}
	svc := newTestRefreshService(repo, issuer)

	_, err := svc.Refresh(context.Background(), RefreshInput{
		ApplicationID: "app-1",
		RefreshToken:  "invalid",
	})
	if err != ErrInvalidRefreshToken {
		t.Fatalf("Refresh() on invalid token = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestRefreshServiceWrongApplication(t *testing.T) {
	repo := newFakeRefreshRepository()
	issuer := &fakeSessionTokenIssuer{token: "access"}
	svc := newTestRefreshService(repo, issuer)

	token, _, _ := svc.IssueRefreshToken(context.Background(), "app-1", "user-1", "license-1", "device-1")

	_, err := svc.Refresh(context.Background(), RefreshInput{
		ApplicationID: "app-2",
		RefreshToken:  token,
	})
	if err != ErrInvalidRefreshToken {
		t.Fatalf("Refresh() with wrong app = %v, want ErrInvalidRefreshToken", err)
	}
}
