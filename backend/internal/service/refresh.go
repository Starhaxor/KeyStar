package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
)

const (
	// refreshTokenLifetime is how long a refresh token is valid.
	refreshTokenLifetime = 30 * 24 * time.Hour // 30 days
	// refreshTokenBytes is the raw byte length of the refresh token.
	refreshTokenBytes = 32
)

// RefreshTokenRepository abstracts the persistence boundary for refresh
// session management.
type RefreshTokenRepository interface {
	CreateRefreshSession(ctx context.Context, input domain.NewRefreshSession) (*domain.RefreshSession, error)
	FindRefreshSessionByTokenHash(ctx context.Context, tokenHash []byte) (*domain.RefreshSession, error)
	RotateRefreshSession(ctx context.Context, sessionID string, now time.Time) (*domain.RefreshSession, error)
	RevokeRefreshSession(ctx context.Context, applicationID, sessionID string) error
	RevokeRefreshSessionFamily(ctx context.Context, applicationID, userID, deviceID string) (int64, error)
	RevokeAllUserRefreshSessions(ctx context.Context, applicationID, userID string) (int64, error)
}

// rejectProofBoundApplication denies refresh use for proof-bound
// applications with the generic unauthorized error. Policy comes only from
// authoritative storage; unknown applications and lookup failures fail
// closed with the same generic error and issue nothing.
func (service *RefreshService) rejectProofBoundApplication(ctx context.Context, applicationID string) error {
	if service.applicationResolver == nil {
		return nil
	}
	application, err := service.applicationResolver.FindApplicationByID(ctx, applicationID)
	if err != nil || application == nil || application.AuthProfile == domain.ApplicationAuthProofBound {
		return ErrInvalidRefreshToken
	}
	return nil
}

// Revoke invalidates the presented refresh token within its application.
func (service *RefreshService) Revoke(ctx context.Context, input RefreshInput) error {
	if service == nil || service.repository == nil || input.ApplicationID == "" || input.RefreshToken == "" {
		return ErrInvalidRefreshToken
	}
	if err := service.rejectProofBoundApplication(ctx, input.ApplicationID); err != nil {
		return err
	}
	raw, err := base64.RawURLEncoding.DecodeString(input.RefreshToken)
	if err != nil || len(raw) != refreshTokenBytes {
		return ErrInvalidRefreshToken
	}
	session, err := service.repository.FindRefreshSessionByTokenHash(ctx, hashRefreshToken(raw))
	if err != nil || session.ApplicationID != input.ApplicationID {
		return ErrInvalidRefreshToken
	}
	if err := service.repository.RevokeRefreshSession(ctx, input.ApplicationID, session.ID); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

type RefreshProfileRepository interface {
	LoadProfile(ctx context.Context, applicationID, userID, licenseID, deviceID string) (*domain.UserProfile, error)
}

type RefreshServiceConfig struct {
	Repository  RefreshTokenRepository
	Profile     RefreshProfileRepository
	HMACKey     []byte
	TokenIssuer SessionTokenIssuer
	Issuer      string
	Audience    string
	Product     string
	// ApplicationResolver rejects proof-bound applications from the
	// refresh flow using authoritative storage. Nil preserves legacy
	// behavior for callers without policy context.
	ApplicationResolver ApplicationResolver
}

// RefreshService manages the lifecycle of refresh tokens: issuance on device
// verification, rotation on refresh, and reuse detection.
type RefreshService struct {
	repository          RefreshTokenRepository
	hmacKey             []byte
	tokenIssuer         SessionTokenIssuer
	profile             RefreshProfileRepository
	issuer              string
	audience            string
	product             string
	now                 func() time.Time
	applicationResolver ApplicationResolver
}

// NewRefreshService builds a RefreshService. The hmacKey is used only for
// the optional HMAC-binding in the opaque token prefix (not for hashing).
func NewRefreshService(config RefreshServiceConfig) *RefreshService {
	now := time.Now
	return &RefreshService{
		repository:  config.Repository,
		profile:     config.Profile,
		hmacKey:     append([]byte(nil), config.HMACKey...),
		tokenIssuer: config.TokenIssuer,
		issuer:      config.Issuer, audience: config.Audience, product: config.Product,
		now:                 now,
		applicationResolver: config.ApplicationResolver,
	}
}

// IssueRefreshToken creates a new refresh session and returns the opaque
// token that should be returned to the client exactly once.
func (service *RefreshService) IssueRefreshToken(ctx context.Context, applicationID, userID, licenseID, deviceID string) (string, time.Time, error) {
	if service == nil || service.repository == nil {
		return "", time.Time{}, errors.New("refresh service is not configured")
	}
	if err := service.rejectProofBoundApplication(ctx, applicationID); err != nil {
		return "", time.Time{}, err
	}
	token, err := generateOpaqueToken()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate refresh token: %w", err)
	}
	tokenHash := hashRefreshToken(token)
	now := service.now().UTC()
	expiresAt := now.Add(refreshTokenLifetime)

	_, err = service.repository.CreateRefreshSession(ctx, domain.NewRefreshSession{
		ApplicationID: applicationID,
		UserID:        userID,
		LicenseID:     licenseID,
		DeviceID:      deviceID,
		TokenHash:     tokenHash,
		ExpiresAt:     expiresAt,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("persist refresh token: %w", err)
	}

	tokenString := base64.RawURLEncoding.EncodeToString(token)
	return tokenString, expiresAt, nil
}

// RefreshResult is returned after a successful refresh rotation.
type RefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// Refresh rotates the refresh token: validates the old token, detects reuse,
// issues a new access+refresh token pair and invalidates the old refresh token.
func (service *RefreshService) Refresh(ctx context.Context, input RefreshInput) (RefreshResult, error) {
	if service == nil || service.repository == nil || service.tokenIssuer == nil {
		return RefreshResult{}, errors.New("refresh service is not configured")
	}
	if input.RefreshToken == "" || input.ApplicationID == "" {
		return RefreshResult{}, ErrInvalidVerifyRequest
	}
	if err := service.rejectProofBoundApplication(ctx, input.ApplicationID); err != nil {
		return RefreshResult{}, err
	}

	tokenBytes, err := base64.RawURLEncoding.DecodeString(input.RefreshToken)
	if err != nil || len(tokenBytes) != refreshTokenBytes {
		return RefreshResult{}, ErrInvalidRefreshToken
	}
	tokenHash := hashRefreshToken(tokenBytes)

	now := service.now().UTC()
	session, err := service.repository.FindRefreshSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, domain.ErrRefreshSessionNotFound) {
			return RefreshResult{}, ErrInvalidRefreshToken
		}
		return RefreshResult{}, fmt.Errorf("lookup refresh token: %w", err)
	}

	// Ownership checks.
	if session.ApplicationID != input.ApplicationID {
		return RefreshResult{}, ErrInvalidRefreshToken
	}

	// Reuse detection: a rotated or revoked token means the family is
	// compromised. Revoke every active session for this user+device pair.
	if session.Status == domain.RefreshSessionStatusRotated {
		_, _ = service.repository.RevokeRefreshSessionFamily(ctx, session.ApplicationID, session.UserID, session.DeviceID)
		return RefreshResult{}, domain.ErrRefreshTokenReuse
	}
	if session.Status == domain.RefreshSessionStatusRevoked {
		return RefreshResult{}, domain.ErrRefreshTokenRevoked
	}
	if session.ExpiresAt.Before(now) {
		return RefreshResult{}, domain.ErrRefreshTokenExpired
	}

	// Issue new access token before rotation so we have all data.
	accessToken, accessExpiry, err := service.issueAccessToken(ctx, session, input.ApplicationID, now)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("issue access token: %w", err)
	}

	// Rotate: mark the old token as rotated.
	_, err = service.repository.RotateRefreshSession(ctx, session.ID, now)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("rotate refresh token: %w", err)
	}

	// Issue new refresh token.
	refreshToken, _, err := service.IssueRefreshToken(ctx, session.ApplicationID, session.UserID, session.LicenseID, session.DeviceID)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("issue new refresh token: %w", err)
	}

	return RefreshResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    accessExpiry,
	}, nil
}

// RefreshInput carries the parameters for a refresh request.
type RefreshInput struct {
	ApplicationID string
	RefreshToken  string
	LicenseID     string
	DeviceID      string
	UserID        string
	Product       string
	Features      []string
}

func (service *RefreshService) issueAccessToken(ctx context.Context, session *domain.RefreshSession, applicationID string, now time.Time) (string, time.Time, error) {
	if service.profile == nil || service.issuer == "" || service.audience == "" || service.product == "" || session.LicenseID == "" {
		return "", time.Time{}, errors.New("refresh service token policy is not configured")
	}
	profile, err := service.profile.LoadProfile(ctx, applicationID, session.UserID, session.LicenseID, session.DeviceID)
	if err != nil || profile.AccountStatus != domain.UserStatusActive || profile.LicenseStatus != domain.LicenseStatusActive ||
		!profile.LicenseExpiresAt.After(now) || profile.DeviceStatus != domain.DeviceStatusActive || profile.Product != service.product {
		return "", time.Time{}, ErrInvalidRefreshToken
	}
	accessExpiry := now.Add(sessionTokenLifetime)
	token, err := service.tokenIssuer.Issue(security.SessionClaims{
		Subject:       session.UserID,
		ApplicationID: applicationID,
		LicenseID:     session.LicenseID,
		DeviceID:      session.DeviceID,
		Product:       service.product,
		Features:      []string{},
		Issuer:        service.issuer,
		Audience:      service.audience,
		IssuedAt:      now.Truncate(time.Second),
		ExpiresAt:     accessExpiry,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("issue access token: %w", err)
	}
	return token, accessExpiry, nil
}

func generateOpaqueToken() ([]byte, error) {
	token := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	return token, nil
}

// Unused but kept for testing: hashRefreshToken is a thin wrapper.
func hashRefreshToken(token []byte) []byte {
	digest := sha256.Sum256(token)
	return digest[:]
}

// storeHashRefreshToken wraps hashRefreshToken for use with the store layer.
func storeHashRefreshToken(token []byte) []byte {
	return hashRefreshToken(token)
}
