package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
	"github.com/starloader/backend/internal/store"
)

const (
	// deviceMatchThresholdLegacy is the original hard-coded threshold kept
	// as a fallback only when no device policy row exists and the default
	// policy cannot be loaded. In practice DefaultDevicePolicy mirrors this.
	deviceMatchThresholdLegacy = 70
	sessionTokenLifetime       = time.Hour
	maxSessionIDBytes          = 128
	maxHardwareValueBytes      = 4096
)

var (
	ErrInvalidVerifyRequest   = errors.New("invalid device verification request")
	ErrChallengeExpired       = errors.New("challenge expired")
	ErrInvalidDeviceSignature = errors.New("invalid device signature")
	ErrDeviceLimitReached     = errors.New("device limit reached")
	ErrDeviceRevoked          = errors.New("device revoked")
	ErrDeviceBanned           = errors.New("device banned")
	ErrTPMRequired            = errors.New("tpm is required by application device policy")
	ErrInvalidRefreshToken    = errors.New("invalid refresh token")
)

type HardwareSignals struct {
	SMBIOSUUID        string
	MotherboardSerial string
	BIOSSerial        string
	SystemDiskSerial  string
	MachineGuid       string
	Fingerprint       string
}

type VerifyInput struct {
	ApplicationID      string
	SessionID          string
	Challenge          string
	ChallengeSignature string
	TPMPublicKey       string
	// DeviceJWK carries the TPM P-256 public key for proof-bound
	// applications. It is required when the application auth profile is
	// proof_bound and ignored by the legacy flow.
	DeviceJWK json.RawMessage
	Hardware  HardwareSignals
}

type VerifiedSession struct {
	Token        string
	RefreshToken string
	ExpiresAt    time.Time
	LicenseID    string
	DeviceID     string
}

type DeviceTransaction interface {
	PendingSession() domain.AuthSession
	PendingChallenge() domain.DeviceChallenge
	LockLicense(context.Context) (*domain.License, error)
	ListDevices(context.Context) ([]domain.Device, error)
	CreateDevice(context.Context, domain.NewDevice) (*domain.Device, error)
	UpdateDevice(context.Context, domain.UpdateDevice) error
	IsDeviceBanned(context.Context, string, time.Time) (bool, error)
	MarkSessionVerified(context.Context, time.Time) error
}

type DeviceRepository interface {
	WithLockedChallenge(context.Context, string, string, func(DeviceTransaction) error) error
	GetDevicePolicy(ctx context.Context, applicationID string) (*domain.DevicePolicy, error)
}

type SessionTokenIssuer interface {
	Issue(security.SessionClaims) (string, error)
}

// ApplicationResolver loads the authoritative application policy. It is
// satisfied by store.Store; fakes back service and HTTP tests.
type ApplicationResolver interface {
	FindApplicationByID(context.Context, string) (*domain.Application, error)
}

// ProofBoundIssuer issues application-scoped proof-bound access tokens. It
// is satisfied by security.ApplicationSigner.
type ProofBoundIssuer interface {
	IssueProofBound(context.Context, string, security.SessionClaims) (string, time.Time, error)
}

type DeviceServiceConfig struct {
	HardwareHMACKey []byte
	TokenIssuer     SessionTokenIssuer
	Issuer          string
	Audience        string
	Product         string
	RefreshService  *RefreshService
	Now             func() time.Time
	// ApplicationResolver selects the verification flow from authoritative
	// storage. Nil preserves the legacy flow.
	ApplicationResolver ApplicationResolver
	// ProofBoundIssuer issues 600-second key-bound tokens. Required only
	// for proof_bound applications.
	ProofBoundIssuer ProofBoundIssuer
}

type DeviceService struct {
	repository          DeviceRepository
	hardwareHMACKey     []byte
	tokenIssuer         SessionTokenIssuer
	issuer              string
	audience            string
	product             string
	refreshService      *RefreshService
	now                 func() time.Time
	applicationResolver ApplicationResolver
	proofBoundIssuer    ProofBoundIssuer
}

type storeDeviceRepository struct {
	store *store.Store
}

func NewStoreDeviceRepository(repository *store.Store) DeviceRepository {
	return &storeDeviceRepository{store: repository}
}

func (repository *storeDeviceRepository) WithLockedChallenge(ctx context.Context, applicationID, sessionID string, callback func(DeviceTransaction) error) error {
	if repository == nil || repository.store == nil {
		return errors.New("device repository is not configured")
	}
	return repository.store.WithLockedChallenge(ctx, applicationID, sessionID, func(locked *store.LockedChallenge) error {
		return callback(locked)
	})
}

func (repository *storeDeviceRepository) GetDevicePolicy(ctx context.Context, applicationID string) (*domain.DevicePolicy, error) {
	if repository == nil || repository.store == nil {
		return domain.DefaultDevicePolicy(applicationID), nil
	}
	return repository.store.GetDevicePolicy(ctx, applicationID)
}

func NewDeviceService(repository DeviceRepository, config DeviceServiceConfig) *DeviceService {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &DeviceService{
		repository: repository, hardwareHMACKey: append([]byte(nil), config.HardwareHMACKey...),
		tokenIssuer: config.TokenIssuer, issuer: config.Issuer, audience: config.Audience,
		product: config.Product, refreshService: config.RefreshService, now: now,
		applicationResolver: config.ApplicationResolver, proofBoundIssuer: config.ProofBoundIssuer,
	}
}

func (service *DeviceService) Verify(ctx context.Context, input VerifyInput) (VerifiedSession, error) {
	if service == nil || service.repository == nil || len(service.hardwareHMACKey) == 0 ||
		strings.TrimSpace(service.issuer) == "" || strings.TrimSpace(service.audience) == "" || strings.TrimSpace(service.product) == "" {
		return VerifiedSession{}, errors.New("device service is not configured")
	}
	if strings.TrimSpace(input.ApplicationID) == "" {
		return VerifiedSession{}, ErrInvalidVerifyRequest
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" || len(sessionID) > maxSessionIDBytes || !hardwareInputIsBounded(input.Hardware) {
		return VerifiedSession{}, ErrInvalidVerifyRequest
	}
	proofBound, err := service.resolveAuthProfile(ctx, input.ApplicationID)
	if err != nil {
		return VerifiedSession{}, fmt.Errorf("load application auth profile: %w", err)
	}
	if proofBound {
		return service.verifyProofBound(ctx, input, sessionID)
	}
	return service.verifyLegacy(ctx, input, sessionID)
}

// resolveAuthProfile loads the authoritative application auth profile. A
// nil resolver preserves the legacy flow. Lookup failures are returned so
// verification aborts fail-closed without issuing any token.
func (service *DeviceService) resolveAuthProfile(ctx context.Context, applicationID string) (bool, error) {
	if service.applicationResolver == nil {
		return false, nil
	}
	application, err := service.applicationResolver.FindApplicationByID(ctx, applicationID)
	if err != nil || application == nil {
		return false, errors.New("application policy unavailable")
	}
	return application.AuthProfile == domain.ApplicationAuthProofBound, nil
}

// verifyProofBound verifies the TPM challenge with the presented P-256 JWK,
// binds the computed (never client-supplied) thumbprint into a 600-second
// token, and returns no refresh token.
func (service *DeviceService) verifyProofBound(ctx context.Context, input VerifyInput, sessionID string) (VerifiedSession, error) {
	if service.proofBoundIssuer == nil {
		return VerifiedSession{}, errors.New("device service proof-bound issuer is not configured")
	}
	if len(input.DeviceJWK) == 0 {
		return VerifiedSession{}, ErrInvalidVerifyRequest
	}
	deviceKey, thumbprint, err := security.ParseP256JWK(input.DeviceJWK)
	if err != nil {
		return VerifiedSession{}, ErrInvalidVerifyRequest
	}
	challenge, err := decodeCanonicalBase64(input.Challenge, 32)
	if err != nil {
		return VerifiedSession{}, ErrInvalidVerifyRequest
	}
	signature, err := decodeCanonicalBase64(input.ChallengeSignature, 64)
	if err != nil {
		return VerifiedSession{}, ErrInvalidVerifyRequest
	}
	if err := verifyP256ChallengeSignature(deviceKey, challenge, signature); err != nil {
		return VerifiedSession{}, ErrInvalidDeviceSignature
	}
	deviceKeyBytes := uncompressedP256Key(deviceKey)
	if strings.TrimSpace(input.TPMPublicKey) != "" {
		presented, err := decodeCanonicalBase64(input.TPMPublicKey, 72)
		if err != nil || !cngBlobMatchesP256Key(presented, deviceKey) {
			return VerifiedSession{}, ErrInvalidDeviceSignature
		}
	}
	userID, licenseID, deviceID, err := service.runVerificationTransaction(ctx, input, sessionID, challenge, deviceKeyBytes, func() error {
		return verifyP256ChallengeSignature(deviceKey, challenge, signature)
	})
	if err != nil {
		return VerifiedSession{}, err
	}
	token, expiresAt, err := service.proofBoundIssuer.IssueProofBound(ctx, input.ApplicationID, security.SessionClaims{
		Subject: userID, ApplicationID: input.ApplicationID, LicenseID: licenseID, DeviceID: deviceID, Product: service.product,
		Features: []string{}, Issuer: service.issuer, Audience: service.audience,
		ProofBound: &security.ProofBoundClaims{SessionID: sessionID, DeviceKeyThumbprint: thumbprint},
	})
	if err != nil {
		return VerifiedSession{}, fmt.Errorf("issue proof-bound session token: %w", err)
	}
	return VerifiedSession{Token: token, ExpiresAt: expiresAt, LicenseID: licenseID, DeviceID: deviceID}, nil
}

func (service *DeviceService) verifyLegacy(ctx context.Context, input VerifyInput, sessionID string) (VerifiedSession, error) {
	if service.tokenIssuer == nil {
		return VerifiedSession{}, errors.New("device service is not configured")
	}
	challenge, err := decodeCanonicalBase64(input.Challenge, 32)
	if err != nil {
		return VerifiedSession{}, ErrInvalidVerifyRequest
	}
	publicKey, err := decodeCanonicalBase64(input.TPMPublicKey, 72)
	if err != nil {
		return VerifiedSession{}, ErrInvalidVerifyRequest
	}
	signature, err := decodeCanonicalBase64(input.ChallengeSignature, 64)
	if err != nil {
		return VerifiedSession{}, ErrInvalidVerifyRequest
	}
	userID, licenseID, deviceID, err := service.runVerificationTransaction(ctx, input, sessionID, challenge, publicKey, func() error {
		return security.VerifyCNGP256(publicKey, challenge, signature)
	})
	if err != nil {
		return VerifiedSession{}, err
	}

	issuedAt := service.now().UTC().Truncate(time.Second)
	expiresAt := issuedAt.Add(sessionTokenLifetime)
	token, err := service.tokenIssuer.Issue(security.SessionClaims{
		Subject: userID, ApplicationID: input.ApplicationID, LicenseID: licenseID, DeviceID: deviceID, Product: service.product,
		Features: []string{}, Issuer: service.issuer, Audience: service.audience,
		IssuedAt: issuedAt, ExpiresAt: expiresAt,
	})
	if err != nil {
		return VerifiedSession{}, fmt.Errorf("issue verified session token: %w", err)
	}
	result := VerifiedSession{Token: token, ExpiresAt: expiresAt, LicenseID: licenseID, DeviceID: deviceID}
	// Issue a refresh token when the refresh service is configured.
	if service.refreshService != nil {
		refreshToken, _, refreshErr := service.refreshService.IssueRefreshToken(
			ctx, input.ApplicationID, userID, licenseID, deviceID)
		if refreshErr == nil {
			result.RefreshToken = refreshToken
		}
	}
	return result, nil
}

// runVerificationTransaction executes the shared challenge-consumption flow
// for both profiles: challenge binding, possession check via checkSignature,
// license and device-policy enforcement, and session verification. The
// caller performs signature pre-checks and token issuance.
func (service *DeviceService) runVerificationTransaction(
	ctx context.Context,
	input VerifyInput,
	sessionID string,
	challenge, deviceKeyBytes []byte,
	checkSignature func() error,
) (userID, licenseID, deviceID string, err error) {
	presented := protectedDeviceInput(service.hardwareHMACKey, deviceKeyBytes, input.Hardware)
	if presented.FingerprintHMAC == "" {
		return "", "", "", ErrInvalidVerifyRequest
	}

	// Load per-application device policy. When no row exists the defaults
	// mirror the original hard-coded behaviour (min_match_score=70, TPM
	// optional).
	devicePolicy, err := service.repository.GetDevicePolicy(ctx, input.ApplicationID)
	if err != nil {
		return "", "", "", fmt.Errorf("load device policy: %w", err)
	}
	policyNow := service.now().UTC()
	err = service.repository.WithLockedChallenge(ctx, input.ApplicationID, sessionID, func(transaction DeviceTransaction) error {
		session := transaction.PendingSession()
		deviceChallenge := transaction.PendingChallenge()
		if session.ID != sessionID || deviceChallenge.SessionID != sessionID || session.ApplicationID != input.ApplicationID {
			return ErrInvalidVerifyRequest
		}
		if session.Status == domain.SessionStatusExpired {
			return ErrChallengeExpired
		}
		if session.Status != domain.SessionStatusPending {
			return domain.ErrChallengeConsumed
		}
		if !session.ExpiresAt.After(policyNow) || !deviceChallenge.ExpiresAt.After(policyNow) {
			return ErrChallengeExpired
		}
		digest := sha256.Sum256(challenge)
		if len(deviceChallenge.ChallengeSHA256) != sha256.Size || !hmac.Equal(digest[:], deviceChallenge.ChallengeSHA256) {
			return ErrInvalidDeviceSignature
		}
		if err := checkSignature(); err != nil {
			return ErrInvalidDeviceSignature
		}

		// Enforce TPM policy. When required and no TPM key was presented,
		// reject immediately.
		if devicePolicy.TPMPolicy == domain.TPMRequired && len(deviceKeyBytes) == 0 {
			return ErrTPMRequired
		}

		license, err := transaction.LockLicense(ctx)
		if err != nil {
			return fmt.Errorf("lock verification license: %w", err)
		}
		if license.ID != session.LicenseID || license.UserID != session.UserID || license.Product != service.product {
			return ErrInvalidCredentials
		}
		if license.Status == domain.LicenseStatusRevoked {
			return ErrLicenseRevoked
		}
		if license.Status == domain.LicenseStatusExpired || !license.ExpiresAt.After(policyNow) {
			return ErrLicenseExpired
		}
		if license.Status != domain.LicenseStatusActive {
			return ErrInvalidCredentials
		}

		devices, err := transaction.ListDevices(ctx)
		if err != nil {
			return fmt.Errorf("list verification devices: %w", err)
		}
		matched, activeCount, err := matchDevice(devices, presented, devicePolicy)
		if err != nil {
			return err
		}
		if matched != nil {
			banned, banErr := transaction.IsDeviceBanned(ctx, matched.ID, policyNow)
			if banErr != nil {
				return fmt.Errorf("check device ban: %w", banErr)
			}
			if banned {
				return ErrDeviceBanned
			}
			if err := transaction.UpdateDevice(ctx, domain.UpdateDevice{
				ID: matched.ID, SMBIOSUUIDHMAC: presented.SMBIOSUUIDHMAC,
				MotherboardSerialHMAC: presented.MotherboardSerialHMAC, BIOSSerialHMAC: presented.BIOSSerialHMAC,
				SystemDiskSerialHMAC: presented.SystemDiskSerialHMAC, MachineGuidHMAC: presented.MachineGuidHMAC,
				FingerprintHMAC: presented.FingerprintHMAC, SeenAt: policyNow,
			}); err != nil {
				return fmt.Errorf("update verified device: %w", err)
			}
			deviceID = matched.ID
		} else {
			if activeCount >= license.MaxDevices {
				return ErrDeviceLimitReached
			}
			device, err := transaction.CreateDevice(ctx, domain.NewDevice{
				UserID: session.UserID, LicenseID: session.LicenseID,
				TPMPublicKey: deviceKeyBytes, TPMPublicKeySHA256: presented.TPMPublicKeySHA256,
				SMBIOSUUIDHMAC: presented.SMBIOSUUIDHMAC, MotherboardSerialHMAC: presented.MotherboardSerialHMAC,
				BIOSSerialHMAC: presented.BIOSSerialHMAC, SystemDiskSerialHMAC: presented.SystemDiskSerialHMAC,
				MachineGuidHMAC: presented.MachineGuidHMAC, FingerprintHMAC: presented.FingerprintHMAC, SeenAt: policyNow,
			})
			if err != nil {
				return fmt.Errorf("create verified device: %w", err)
			}
			if device == nil || device.ID == "" {
				return errors.New("create verified device: repository returned invalid device")
			}
			deviceID = device.ID
		}
		if err := transaction.MarkSessionVerified(ctx, policyNow); err != nil {
			return fmt.Errorf("mark verified session: %w", err)
		}
		licenseID = license.ID
		userID = session.UserID
		return nil
	})
	if err != nil {
		return "", "", "", err
	}
	return userID, licenseID, deviceID, nil
}

// verifyP256ChallengeSignature verifies a raw fixed-width r||s ECDSA P-256
// signature over sha256(challenge) with the JWK device key.
func verifyP256ChallengeSignature(key *ecdsa.PublicKey, challenge, signature []byte) error {
	if key == nil || key.Curve == nil || len(challenge) == 0 || len(signature) != 2*p256SignatureHalfBytes {
		return security.ErrInvalidDeviceProof
	}
	curveOrder := key.Curve.Params().N
	r := new(big.Int).SetBytes(signature[:p256SignatureHalfBytes])
	s := new(big.Int).SetBytes(signature[p256SignatureHalfBytes:])
	if r.Sign() <= 0 || s.Sign() <= 0 || r.Cmp(curveOrder) >= 0 || s.Cmp(curveOrder) >= 0 {
		return security.ErrInvalidDeviceProof
	}
	digest := sha256.Sum256(challenge)
	if !ecdsa.Verify(key, digest[:], r, s) {
		return security.ErrInvalidDeviceProof
	}
	return nil
}

const p256SignatureHalfBytes = 32

// uncompressedP256Key renders the JWK key as 0x04||x||y for device
// matching and storage. The sha256 of these bytes identifies the device.
func uncompressedP256Key(key *ecdsa.PublicKey) []byte {
	rendered := make([]byte, 1+2*p256SignatureHalfBytes)
	rendered[0] = 4
	key.X.FillBytes(rendered[1 : 1+p256SignatureHalfBytes])
	key.Y.FillBytes(rendered[1+p256SignatureHalfBytes:])
	return rendered
}

// cngBlobMatchesP256Key reports whether a presented CNG P-256 public blob
// carries the same coordinates as the verified JWK key.
func cngBlobMatchesP256Key(blob []byte, key *ecdsa.PublicKey) bool {
	if len(blob) != 8+2*p256SignatureHalfBytes || key == nil || key.X == nil || key.Y == nil {
		return false
	}
	if binary.LittleEndian.Uint32(blob[:4]) != 0x31534345 || binary.LittleEndian.Uint32(blob[4:8]) != p256SignatureHalfBytes {
		return false
	}
	x := new(big.Int).SetBytes(blob[8:40])
	y := new(big.Int).SetBytes(blob[40:72])
	return x.Cmp(key.X) == 0 && y.Cmp(key.Y) == 0
}

type protectedDevice struct {
	TPMPublicKeySHA256    []byte
	SMBIOSUUIDHMAC        string
	MotherboardSerialHMAC string
	BIOSSerialHMAC        string
	SystemDiskSerialHMAC  string
	MachineGuidHMAC       string
	FingerprintHMAC       string
}

func protectedDeviceInput(key, publicKey []byte, hardware HardwareSignals) protectedDevice {
	digest := sha256.Sum256(publicKey)
	return protectedDevice{
		TPMPublicKeySHA256:    digest[:],
		SMBIOSUUIDHMAC:        hashHardware(key, hardware.SMBIOSUUID),
		MotherboardSerialHMAC: hashHardware(key, hardware.MotherboardSerial),
		BIOSSerialHMAC:        hashHardware(key, hardware.BIOSSerial),
		SystemDiskSerialHMAC:  hashHardware(key, hardware.SystemDiskSerial),
		MachineGuidHMAC:       hashHardware(key, hardware.MachineGuid),
		FingerprintHMAC:       hashHardware(key, hardware.Fingerprint),
	}
}

func matchDevice(devices []domain.Device, presented protectedDevice, policy *domain.DevicePolicy) (*domain.Device, int, error) {
	presentedSignals := domain.DeviceSignals{
		TPM: hex.EncodeToString(presented.TPMPublicKeySHA256), SMBIOS: presented.SMBIOSUUIDHMAC,
		Motherboard: presented.MotherboardSerialHMAC, BIOS: presented.BIOSSerialHMAC,
		SystemDisk: presented.SystemDiskSerialHMAC, MachineGuid: presented.MachineGuidHMAC,
	}
	minScore := deviceMatchThresholdLegacy
	if policy != nil && policy.MinMatchScore > 0 {
		minScore = policy.MinMatchScore
	}
	activeCount := 0
	var matched *domain.Device
	bestScore := -1
	for index := range devices {
		device := &devices[index]
		if device.Status == domain.DeviceStatusActive {
			activeCount++
		}
		if device.Status == domain.DeviceStatusRevoked && len(device.TPMPublicKeySHA256) == sha256.Size &&
			hmac.Equal(device.TPMPublicKeySHA256, presented.TPMPublicKeySHA256) {
			return nil, activeCount, ErrDeviceRevoked
		}
		score := domain.ScoreDevice(domain.DeviceSignals{
			TPM: hex.EncodeToString(device.TPMPublicKeySHA256), SMBIOS: device.SMBIOSUUIDHMAC,
			Motherboard: device.MotherboardSerialHMAC, BIOS: device.BIOSSerialHMAC,
			SystemDisk: device.SystemDiskSerialHMAC, MachineGuid: device.MachineGuidHMAC,
		}, presentedSignals)
		if score < minScore {
			continue
		}
		if device.Status == domain.DeviceStatusRevoked {
			return nil, activeCount, ErrDeviceRevoked
		}
		if device.Status == domain.DeviceStatusActive && score > bestScore {
			matched = device
			bestScore = score
		}
	}
	return matched, activeCount, nil
}

func decodeCanonicalBase64(encoded string, exactBytes int) ([]byte, error) {
	if encoded == "" || len(encoded) != base64.StdEncoding.EncodedLen(exactBytes) {
		return nil, ErrInvalidVerifyRequest
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != exactBytes || base64.StdEncoding.EncodeToString(decoded) != encoded {
		return nil, ErrInvalidVerifyRequest
	}
	return decoded, nil
}

func hardwareInputIsBounded(hardware HardwareSignals) bool {
	for _, value := range []string{hardware.SMBIOSUUID, hardware.MotherboardSerial, hardware.BIOSSerial, hardware.SystemDiskSerial, hardware.MachineGuid, hardware.Fingerprint} {
		if !utf8.ValidString(value) || len(value) > maxHardwareValueBytes {
			return false
		}
	}
	return true
}

func hashHardware(key []byte, raw string) string {
	normalized := normalizeHardware(raw)
	if normalized == "" {
		return ""
	}
	return security.HMACHex(key, normalized)
}

func normalizeHardware(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	return strings.NewReplacer(" ", "", "-", "", "{", "", "}", "").Replace(value)
}
