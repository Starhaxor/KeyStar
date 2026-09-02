package security

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProofBoundTokenUsesExactKeyedHeaderAndRequiredBindings(t *testing.T) {
	publicKey, privateKey := deterministicEd25519Key()
	now := time.Unix(1_788_343_200, 0).UTC()
	claims := requiredProofBoundClaims(now)

	token, err := issueProofBoundToken(privateKey, "ksk_test", claims)
	if err != nil {
		t.Fatalf("issueProofBoundToken() error = %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3", len(parts))
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(headerJSON), `{"alg":"EdDSA","typ":"JWT","kid":"ksk_test"}`; got != want {
		t.Fatalf("header = %s, want %s", got, want)
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"sub": "user-1", "app": "app-1", "license_id": "license-1", "device_id": "device-1",
		"product": "StarLoader", "iss": "keystar", "aud": "starloader-client", "sid": "session-1",
		"jti": "AAECAwQFBgcICQoLDA0ODw", "iat": float64(now.Unix()), "nbf": float64(now.Unix()),
		"exp": float64(now.Add(600 * time.Second).Unix()),
	} {
		if got := payload[key]; got != want {
			t.Fatalf("payload[%q] = %#v, want %#v", key, got, want)
		}
	}
	confirmation, ok := payload["cnf"].(map[string]any)
	if !ok || len(confirmation) != 1 || confirmation["jkt"] != claims.ProofBound.DeviceKeyThumbprint {
		t.Fatalf("payload cnf = %#v", payload["cnf"])
	}
	if got := int64(payload["exp"].(float64) - payload["iat"].(float64)); got != 600 {
		t.Fatalf("exp-iat = %d, want 600", got)
	}

	got, err := verifyProofBoundToken(token, publicKey, "ksk_test", "app-1", "keystar", "starloader-client", "StarLoader", now)
	if err != nil {
		t.Fatalf("verifyProofBoundToken() error = %v", err)
	}
	if !reflect.DeepEqual(got, claims) {
		t.Fatalf("verified claims = %#v, want %#v", got, claims)
	}
}

func TestProofBoundTokenRejectsMissingOrChangedBindings(t *testing.T) {
	_, privateKey := deterministicEd25519Key()
	now := time.Unix(1_788_343_200, 0).UTC()
	for _, test := range []struct {
		name   string
		mutate func(*SessionClaims)
	}{
		{name: "application", mutate: func(c *SessionClaims) { c.ApplicationID = "" }},
		{name: "product", mutate: func(c *SessionClaims) { c.Product = "" }},
		{name: "license", mutate: func(c *SessionClaims) { c.LicenseID = "" }},
		{name: "device", mutate: func(c *SessionClaims) { c.DeviceID = "" }},
		{name: "session", mutate: func(c *SessionClaims) { c.ProofBound.SessionID = "" }},
		{name: "token id", mutate: func(c *SessionClaims) { c.ProofBound.TokenID = "" }},
		{name: "thumbprint", mutate: func(c *SessionClaims) { c.ProofBound.DeviceKeyThumbprint = "" }},
		{name: "not before", mutate: func(c *SessionClaims) { c.ProofBound.NotBefore = time.Time{} }},
		{name: "wrong lifetime", mutate: func(c *SessionClaims) { c.ExpiresAt = c.IssuedAt.Add(601 * time.Second) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims := requiredProofBoundClaims(now)
			test.mutate(&claims)
			if _, err := issueProofBoundToken(privateKey, "ksk_test", claims); err != ErrInvalidSessionToken {
				t.Fatalf("issueProofBoundToken() error = %v, want %v", err, ErrInvalidSessionToken)
			}
		})
	}
}

func TestProofBoundTokenStrictParsingRejectsDuplicateMembersAndNoncanonicalSegments(t *testing.T) {
	publicKey, privateKey := deterministicEd25519Key()
	now := time.Unix(1_788_343_200, 0).UTC()
	payload := proofBoundPayloadJSON(now)
	header := `{"alg":"EdDSA","typ":"JWT","kid":"ksk_test"}`
	valid := signProofBoundJSON(t, privateKey, header, payload)
	validParts := strings.Split(valid, ".")
	noncanonicalPayload := validParts[1] + "="
	tests := []struct {
		name  string
		token string
	}{
		{name: "duplicate header member", token: signProofBoundJSON(t, privateKey, `{"alg":"EdDSA","typ":"JWT","kid":"ksk_test","kid":"ksk_test"}`, payload)},
		{name: "escaped duplicate header member", token: signProofBoundJSON(t, privateKey, `{"alg":"EdDSA","typ":"JWT","kid":"ksk_test","\u006b\u0069\u0064":"ksk_test"}`, payload)},
		{name: "duplicate payload member", token: signProofBoundJSON(t, privateKey, header, strings.Replace(payload, `"sid":"session-1"`, `"sid":"session-1","sid":"session-1"`, 1))},
		{name: "duplicate confirmation member", token: signProofBoundJSON(t, privateKey, header, strings.Replace(payload, `"jkt":"`, `"jkt":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","jkt":"`, 1))},
		{name: "unknown header member", token: signProofBoundJSON(t, privateKey, `{"alg":"EdDSA","typ":"JWT","kid":"ksk_test","extra":true}`, payload)},
		{name: "unknown payload member", token: signProofBoundJSON(t, privateKey, header, strings.TrimSuffix(payload, "}")+`,"extra":true}`)},
		{name: "unknown confirmation member", token: signProofBoundJSON(t, privateKey, header, strings.Replace(payload, `"cnf":{"jkt":`, `"cnf":{"extra":true,"jkt":`, 1))},
		{name: "trailing header JSON", token: signProofBoundJSON(t, privateKey, header+` {}`, payload)},
		{name: "trailing payload JSON", token: signProofBoundJSON(t, privateKey, header, payload+` {}`)},
		{name: "padded header", token: validParts[0] + "=." + validParts[1] + "." + validParts[2]},
		{name: "padded payload", token: signProofBoundSegments(t, privateKey, validParts[0], noncanonicalPayload)},
		{name: "padded signature", token: valid + "="},
		{name: "empty segment", token: validParts[0] + ".." + validParts[2]},
		{name: "oversized compact token", token: valid + strings.Repeat("A", maxSessionTokenBytes)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := verifyProofBoundToken(test.token, publicKey, "ksk_test", "app-1", "keystar", "starloader-client", "StarLoader", now); err != ErrInvalidSessionToken {
				t.Fatalf("verifyProofBoundToken() error = %v, want sanitized %v", err, ErrInvalidSessionToken)
			}
		})
	}
}

func TestProofBoundTokenRejectsUnknownKIDAndWrongSignedBindings(t *testing.T) {
	publicKey, privateKey := deterministicEd25519Key()
	now := time.Unix(1_788_343_200, 0).UTC()
	token, err := issueProofBoundToken(privateKey, "ksk_test", requiredProofBoundClaims(now))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, kid, app, issuer, audience, product string }{
		{name: "unknown kid", kid: "ksk_unknown", app: "app-1", issuer: "keystar", audience: "starloader-client", product: "StarLoader"},
		{name: "wrong application", kid: "ksk_test", app: "app-2", issuer: "keystar", audience: "starloader-client", product: "StarLoader"},
		{name: "wrong issuer", kid: "ksk_test", app: "app-1", issuer: "other", audience: "starloader-client", product: "StarLoader"},
		{name: "wrong audience", kid: "ksk_test", app: "app-1", issuer: "keystar", audience: "other", product: "StarLoader"},
		{name: "wrong product", kid: "ksk_test", app: "app-1", issuer: "keystar", audience: "starloader-client", product: "Other"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := verifyProofBoundToken(token, publicKey, test.kid, test.app, test.issuer, test.audience, test.product, now); err != ErrInvalidSessionToken {
				t.Fatalf("verifyProofBoundToken() error = %v, want %v", err, ErrInvalidSessionToken)
			}
		})
	}
}

func TestProofBoundTokenAcceptsOnlyWithinSixtySecondClockSkew(t *testing.T) {
	publicKey, privateKey := deterministicEd25519Key()
	now := time.Unix(1_788_343_200, 0).UTC()
	tests := []struct {
		name     string
		issuedAt time.Time
		verifyAt time.Time
		wantOK   bool
	}{
		{name: "not before exactly sixty seconds ahead", issuedAt: now.Add(60 * time.Second), verifyAt: now, wantOK: true},
		{name: "not before sixty one seconds ahead", issuedAt: now.Add(61 * time.Second), verifyAt: now},
		{name: "expired exactly sixty seconds ago", issuedAt: now.Add(-660 * time.Second), verifyAt: now, wantOK: true},
		{name: "expired sixty one seconds ago", issuedAt: now.Add(-661 * time.Second), verifyAt: now},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, err := issueProofBoundToken(privateKey, "ksk_test", requiredProofBoundClaims(test.issuedAt))
			if err != nil {
				t.Fatal(err)
			}
			_, err = verifyProofBoundToken(token, publicKey, "ksk_test", "app-1", "keystar", "starloader-client", "StarLoader", test.verifyAt)
			if test.wantOK && err != nil {
				t.Fatalf("verifyProofBoundToken() error = %v", err)
			}
			if !test.wantOK && err != ErrInvalidSessionToken {
				t.Fatalf("verifyProofBoundToken() error = %v, want %v", err, ErrInvalidSessionToken)
			}
		})
	}
}

func deterministicEd25519Key() (ed25519.PublicKey, ed25519.PrivateKey) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x42}, ed25519.SeedSize))
	return append(ed25519.PublicKey(nil), privateKey[ed25519.SeedSize:]...), privateKey
}

func requiredProofBoundClaims(issuedAt time.Time) SessionClaims {
	return SessionClaims{
		Subject: "user-1", ApplicationID: "app-1", LicenseID: "license-1", DeviceID: "device-1",
		Product: "StarLoader", Features: []string{"launch"}, Issuer: "keystar", Audience: "starloader-client",
		IssuedAt: issuedAt, ExpiresAt: issuedAt.Add(600 * time.Second),
		ProofBound: &ProofBoundClaims{
			SessionID: "session-1", TokenID: "AAECAwQFBgcICQoLDA0ODw",
			DeviceKeyThumbprint: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", NotBefore: issuedAt,
		},
	}
}

func proofBoundPayloadJSON(issuedAt time.Time) string {
	return fmt.Sprintf(`{"sub":"user-1","app":"app-1","license_id":"license-1","device_id":"device-1","product":"StarLoader","features":["launch"],"iss":"keystar","aud":"starloader-client","iat":%d,"exp":%d,"sid":"session-1","jti":"AAECAwQFBgcICQoLDA0ODw","nbf":%d,"cnf":{"jkt":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}`, issuedAt.Unix(), issuedAt.Add(600*time.Second).Unix(), issuedAt.Unix())
}

func signProofBoundJSON(t *testing.T, privateKey ed25519.PrivateKey, headerJSON, payloadJSON string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return signProofBoundSegments(t, privateKey, header, payload)
}

func signProofBoundSegments(t *testing.T, privateKey ed25519.PrivateKey, header, payload string) string {
	t.Helper()
	input := header + "." + payload
	return input + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(input)))
}

func TestEd25519SessionTokenRoundTripPreservesRequiredClaims(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_786_350_600, 0).UTC()
	issuer, err := NewTokenIssuer(privateKey, "starloader", "starloader-client", "StarLoader")
	if err != nil {
		t.Fatal(err)
	}
	issuer.now = func() time.Time { return now }
	verifier, err := NewTokenVerifier(publicKey, "starloader", "starloader-client", "StarLoader")
	if err != nil {
		t.Fatal(err)
	}
	verifier.now = func() time.Time { return now }
	want := SessionClaims{
		Subject:       "user-1",
		ApplicationID: "app-1",
		LicenseID:     "license-1",
		DeviceID:      "device-1",
		Product:       "StarLoader",
		Features:      []string{"launch"},
		Issuer:        "starloader",
		Audience:      "starloader-client",
		IssuedAt:      now,
		ExpiresAt:     now.Add(time.Hour),
	}

	token, err := issuer.Issue(want)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if parts := strings.Split(token, "."); len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3", len(parts))
	}
	got, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Verify() = %#v, want %#v", got, want)
	}
}

func TestTokenVerifierEnforcesIdentityAndExpiration(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_786_350_600, 0).UTC()
	claims := SessionClaims{
		Subject: "user-1", ApplicationID: "app-1", LicenseID: "license-1", DeviceID: "device-1",
		Product: "StarLoader", Issuer: "starloader", Audience: "starloader-client",
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	issuer, err := NewTokenIssuer(privateKey, claims.Issuer, claims.Audience, claims.Product)
	if err != nil {
		t.Fatal(err)
	}
	issuer.now = func() time.Time { return now }
	token, err := issuer.Issue(claims)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		issuer   string
		audience string
		product  string
		now      time.Time
	}{
		{name: "wrong issuer", issuer: "other", audience: claims.Audience, product: claims.Product, now: now},
		{name: "wrong audience", issuer: claims.Issuer, audience: "other", product: claims.Product, now: now},
		{name: "wrong product", issuer: claims.Issuer, audience: claims.Audience, product: "Other", now: now},
		{name: "expired", issuer: claims.Issuer, audience: claims.Audience, product: claims.Product, now: claims.ExpiresAt},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier, err := NewTokenVerifier(publicKey, test.issuer, test.audience, test.product)
			if err != nil {
				t.Fatal(err)
			}
			verifier.now = func() time.Time { return test.now }
			if _, err := verifier.Verify(token); err == nil {
				t.Fatal("Verify() accepted token outside policy")
			}
		})
	}
}

func TestTokenIssuerRejectsMissingLicenseOrDevice(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_786_350_600, 0).UTC()
	issuer, err := NewTokenIssuer(privateKey, "starloader", "client", "StarLoader")
	if err != nil {
		t.Fatal(err)
	}
	issuer.now = func() time.Time { return now }
	valid := SessionClaims{
		Subject: "user", ApplicationID: "app", LicenseID: "license", DeviceID: "device", Product: "StarLoader",
		Issuer: "starloader", Audience: "client", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	for _, mutate := range []func(*SessionClaims){
		func(claims *SessionClaims) { claims.ApplicationID = "" },
		func(claims *SessionClaims) { claims.LicenseID = "" },
		func(claims *SessionClaims) { claims.DeviceID = "" },
	} {
		claims := valid
		mutate(&claims)
		if _, err := issuer.Issue(claims); err == nil {
			t.Fatal("Issue() accepted missing bound identity")
		}
	}
}

func TestTokenVerifierRejectsChangedSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_786_350_600, 0).UTC()
	issuer, _ := NewTokenIssuer(privateKey, "starloader", "client", "StarLoader")
	issuer.now = func() time.Time { return now }
	verifier, _ := NewTokenVerifier(publicKey, "starloader", "client", "StarLoader")
	verifier.now = func() time.Time { return now }
	token, err := issuer.Issue(SessionClaims{
		Subject: "user", ApplicationID: "app", LicenseID: "license", DeviceID: "device", Product: "StarLoader",
		Issuer: "starloader", Audience: "client", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	tampered := []byte(token)
	tampered[len(tampered)-1] ^= 1
	if _, err := verifier.Verify(string(tampered)); err == nil {
		t.Fatal("Verify() accepted changed signature")
	}
}

func TestEd25519ConfigurationRejectsInconsistentExpandedPrivateKey(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	malformed := append(ed25519.PrivateKey(nil), privateKey...)
	malformed[len(malformed)-1] ^= 0x80
	if _, err := NewTokenIssuer(malformed, "starloader", "client", "StarLoader"); err == nil {
		t.Fatal("NewTokenIssuer() accepted an inconsistent expanded private key")
	}
	encoded := base64.StdEncoding.EncodeToString(malformed)
	if _, err := ParseEd25519PrivateKey(encoded); err == nil {
		t.Fatal("ParseEd25519PrivateKey() accepted an inconsistent expanded private key")
	}
}

func TestTokenIssuerRequiresExactOneHourLifetime(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_786_350_600, 0).UTC()
	issuer, err := NewTokenIssuer(privateKey, "starloader", "client", "StarLoader")
	if err != nil {
		t.Fatal(err)
	}
	issuer.now = func() time.Time { return now }
	for _, test := range []struct {
		name     string
		lifetime time.Duration
		wantOK   bool
	}{
		{name: "59 minutes", lifetime: 59 * time.Minute},
		{name: "exact hour", lifetime: time.Hour, wantOK: true},
		{name: "61 minutes", lifetime: 61 * time.Minute},
		{name: "one day", lifetime: 24 * time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := issuer.Issue(requiredTokenClaims(now, test.lifetime))
			if test.wantOK && err != nil {
				t.Fatalf("Issue() error = %v", err)
			}
			if !test.wantOK && err == nil {
				t.Fatalf("Issue() accepted lifetime %s", test.lifetime)
			}
		})
	}
}

func TestTokenVerifierRequiresExactOneHourLifetime(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_786_350_600, 0).UTC()
	verifier, err := NewTokenVerifier(publicKey, "starloader", "client", "StarLoader")
	if err != nil {
		t.Fatal(err)
	}
	verifier.now = func() time.Time { return now }
	for _, test := range []struct {
		name     string
		lifetime time.Duration
		wantOK   bool
	}{
		{name: "59 minutes", lifetime: 59 * time.Minute},
		{name: "exact hour", lifetime: time.Hour, wantOK: true},
		{name: "61 minutes", lifetime: 61 * time.Minute},
		{name: "one day", lifetime: 24 * time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := signSessionClaimsForTest(t, privateKey, requiredTokenClaims(now, test.lifetime))
			_, err := verifier.Verify(token)
			if test.wantOK && err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if !test.wantOK && err == nil {
				t.Fatalf("Verify() accepted lifetime %s", test.lifetime)
			}
		})
	}
}

func requiredTokenClaims(issuedAt time.Time, lifetime time.Duration) SessionClaims {
	return SessionClaims{
		Subject: "user", ApplicationID: "app", LicenseID: "license", DeviceID: "device", Product: "StarLoader",
		Features: []string{}, Issuer: "starloader", Audience: "client",
		IssuedAt: issuedAt, ExpiresAt: issuedAt.Add(lifetime),
	}
}

func signSessionClaimsForTest(t *testing.T, privateKey ed25519.PrivateKey, claims SessionClaims) string {
	t.Helper()
	headerJSON, err := json.Marshal(tokenHeader{Algorithm: "EdDSA", Type: "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payloadJSON, err := json.Marshal(claimsToWire(claims))
	if err != nil {
		t.Fatal(err)
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := header + "." + payload
	signature := ed25519.Sign(privateKey, []byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}
