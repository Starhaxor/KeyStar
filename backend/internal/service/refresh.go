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
	RevokeRefreshSessionFamily(ctx context.Context, userID, deviceID string) (int64, error)
	RevokeAllUserRefreshSessions(ctx context.Context, userID string) (int64, error)
}

// RefreshService manages the lifecycle of refresh tokens: issuance on device
// verification, rotation on refresh, and reuse detection.
type RefreshService struct {
	repository  RefreshTokenRepository
	hmacKey     []byte
	tokenIssuer SessionTokenIssuer
	now         func() time.Time
}

// NewRefreshService builds a RefreshService. The hmacKey is used only for
// the optional HMAC-binding in the opaque token prefix (not for hashing).
func NewRefreshService(repository RefreshTokenRepository, hmacKey []byte, issuer SessionTokenIssuer) *RefreshService {
	now := time.Now
	return &RefreshService{
		repository:  repository,
		hmacKey:     append([]byte(nil), hmacKey...),
		tokenIssuer: issuer,
		now:         now,
	}
}

// IssueRefreshToken creates a new refresh session and returns the opaque
// token that should be returned to the client exactly once.
func (service *RefreshService) IssueRefreshToken(ctx context.Context, applicationID, userID, deviceID string) (string, time.Time, error) {
	if service == nil || service.repository == nil {
		return "", time.Time{}, errors.New("refresh service is not configured")
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
		_, _ = service.repository.RevokeRefreshSessionFamily(ctx, session.UserID, session.DeviceID)
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
	refreshToken, _, err := service.IssueRefreshToken(ctx, session.ApplicationID, session.UserID, session.DeviceID)
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
	accessExpiry := now.Add(sessionTokenLifetime)
	token, err := service.tokenIssuer.Issue(security.SessionClaims{
		Subject:       session.UserID,
		ApplicationID: applicationID,
		LicenseID:     "",
		DeviceID:      session.DeviceID,
		Product:       "",
		Features:      []string{},
		Issuer:        "",
		Audience:      "",
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
