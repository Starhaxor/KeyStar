package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"math/big"
	"strings"
	"testing"
	"time"
)

type dpopFixture struct {
	key        *ecdsa.PrivateKey
	jwk        string
	thumbprint string
	token      SessionClaims
	now        time.Time
}

func newDPoPFixure(t *testing.T) *dpopFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	x := make([]byte, 32)
	y := make([]byte, 32)
	key.X.FillBytes(x)
	key.Y.FillBytes(y)
	encode := base64.RawURLEncoding.EncodeToString
	jwk := `{"crv":"P-256","kty":"EC","x":"` + encode(x) + `","y":"` + encode(y) + `"}`
	public, thumbprint, err := ParseP256JWK([]byte(jwk))
	if err != nil {
		t.Fatal(err)
	}
	_ = public
	now := time.Unix(1_788_343_200, 0).UTC()
	token := SessionClaims{
		Subject: "user-1", ApplicationID: "app-1", LicenseID: "license-1", DeviceID: "device-1",
		Product: "StarLoader", Issuer: "keystar", Audience: "starloader-client",
		IssuedAt: now, ExpiresAt: now.Add(600 * time.Second),
		ProofBound: &ProofBoundClaims{
			SessionID: "session-1", TokenID: base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
			DeviceKeyThumbprint: thumbprint, NotBefore: now,
		},
	}
	return &dpopFixture{key: key, jwk: jwk, thumbprint: thumbprint, token: token, now: now}
}

func accessTokenATH(token string) string {
	digest := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func mintDPoP(t *testing.T, key *ecdsa.PrivateKey, header, payload string) string {
	t.Helper()
	signingInput := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(payload))
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func validDPoPHeader(fixture *dpopFixture) string {
	return `{"alg":"ES256","jwk":` + fixture.jwk + `,"typ":"dpop+jwt"}`
}

func validDPoPPayload(fixture *dpopFixture, jti string, iat int64) string {
	return `{"ath":"` + accessTokenATH("proof-access-token") + `","htm":"GET","htu":"https://api.example.com/v1/me","iat":` + int64String(iat) + `,"jti":"` + jti + `"}`
}

func int64String(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func canonicalJTI(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func validDPoPInput(fixture *dpopFixture, proof string) DPoPInput {
	return DPoPInput{
		Proof: proof, AccessToken: "proof-access-token", Method: "GET",
		URI: "https://api.example.com/v1/me", Token: fixture.token, Now: fixture.now,
	}
}

func TestVerifyDPoPAcceptsValidProof(t *testing.T) {
	fixture := newDPoPFixure(t)
	jti := canonicalJTI(t)
	proof := mintDPoP(t, fixture.key, validDPoPHeader(fixture), validDPoPPayload(fixture, jti, fixture.now.Unix()))

	claims, err := VerifyDPoP(validDPoPInput(fixture, proof))
	if err != nil {
		t.Fatalf("VerifyDPoP() error = %v", err)
	}
	wantDigest := sha256.Sum256([]byte(jti))
	if claims.JTIDigest != wantDigest {
		t.Fatalf("VerifyDPoP() digest = %x, want %x", claims.JTIDigest, wantDigest)
	}
	if !claims.IssuedAt.Equal(fixture.now.Truncate(time.Second)) {
		t.Fatalf("VerifyDPoP() issued at = %v", claims.IssuedAt)
	}
	if claims.KeyThumbprint != fixture.thumbprint {
		t.Fatalf("VerifyDPoP() thumbprint = %q", claims.KeyThumbprint)
	}
}

func TestVerifyDPoPAcceptsSkewBoundaries(t *testing.T) {
	fixture := newDPoPFixure(t)
	for _, skew := range []int64{-60, 60} {
		jti := canonicalJTI(t)
		proof := mintDPoP(t, fixture.key, validDPoPHeader(fixture), validDPoPPayload(fixture, jti, fixture.now.Unix()+skew))
		if _, err := VerifyDPoP(validDPoPInput(fixture, proof)); err != nil {
			t.Fatalf("VerifyDPoP() at skew %ds error = %v", skew, err)
		}
	}
}

func TestVerifyDPoPRejectsMalformedProofs(t *testing.T) {
	fixture := newDPoPFixure(t)
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherX := make([]byte, 32)
	otherY := make([]byte, 32)
	other.X.FillBytes(otherX)
	other.Y.FillBytes(otherY)
	otherJWK := `{"crv":"P-256","kty":"EC","x":"` + base64.RawURLEncoding.EncodeToString(otherX) + `","y":"` + base64.RawURLEncoding.EncodeToString(otherY) + `"}`
	validJTI := canonicalJTI(t)
	validProof := mintDPoP(t, fixture.key, validDPoPHeader(fixture), validDPoPPayload(fixture, validJTI, fixture.now.Unix()))

	derSignatureProof := func() string {
		header := base64.RawURLEncoding.EncodeToString([]byte(validDPoPHeader(fixture)))
		payload := base64.RawURLEncoding.EncodeToString([]byte(validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix())))
		digest := sha256.Sum256([]byte(header + "." + payload))
		r, s, err := ecdsa.Sign(rand.Reader, fixture.key, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		der, err := asn1.Marshal(struct {
			R, S *big.Int
		}{R: r, S: s})
		if err != nil {
			t.Fatal(err)
		}
		return header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(der)
	}()

	for _, test := range []struct {
		name  string
		mutate func() DPoPInput
	}{
		{name: "empty proof", mutate: func() DPoPInput { input := validDPoPInput(fixture, ""); return input }},
		{name: "two segments", mutate: func() DPoPInput { parts := strings.Split(validProof, "."); return validDPoPInput(fixture, parts[0]+"."+parts[1]) }},
		{name: "empty signature", mutate: func() DPoPInput { parts := strings.Split(validProof, "."); return validDPoPInput(fixture, parts[0]+"."+parts[1]+".") }},
		{name: "noncanonical segment", mutate: func() DPoPInput {
			parts := strings.Split(validProof, ".")
			decoded, _ := base64.RawURLEncoding.DecodeString(parts[1])
			padded := base64.URLEncoding.EncodeToString(decoded)
			return validDPoPInput(fixture, parts[0]+"."+padded+"."+parts[2])
		}},
		{name: "wrong typ", mutate: func() DPoPInput {
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, `{"alg":"ES256","jwk":`+fixture.jwk+`,"typ":"JWT"}`, validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix())))
		}},
		{name: "wrong alg", mutate: func() DPoPInput {
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, `{"alg":"RS256","jwk":`+fixture.jwk+`,"typ":"dpop+jwt"}`, validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix())))
		}},
		{name: "header kid member", mutate: func() DPoPInput {
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, `{"alg":"ES256","jwk":`+fixture.jwk+`,"kid":"x","typ":"dpop+jwt"}`, validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix())))
		}},
		{name: "duplicate header member", mutate: func() DPoPInput {
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, `{"alg":"ES256","alg":"ES256","jwk":`+fixture.jwk+`,"typ":"dpop+jwt"}`, validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix())))
		}},
		{name: "duplicate payload member", mutate: func() DPoPInput {
			payload := validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix())
			payload = strings.TrimSuffix(payload, "}") + `,"htm":"GET"}`
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), payload))
		}},
		{name: "DER signature", mutate: func() DPoPInput { return validDPoPInput(fixture, derSignatureProof) }},
		{name: "wrong method", mutate: func() DPoPInput {
			payload := strings.Replace(validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix()), `"htm":"GET"`, `"htm":"POST"`, 1)
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), payload))
		}},
		{name: "htu with query", mutate: func() DPoPInput {
			payload := strings.Replace(validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix()), `https://api.example.com/v1/me`, `https://api.example.com/v1/me?x=1`, 1)
			input := validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), payload))
			input.URI = "https://api.example.com/v1/me"
			return input
		}},
		{name: "htu with fragment", mutate: func() DPoPInput {
			payload := strings.Replace(validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix()), `https://api.example.com/v1/me`, `https://api.example.com/v1/me#x`, 1)
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), payload))
		}},
		{name: "htu host mismatch", mutate: func() DPoPInput {
			payload := strings.Replace(validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix()), `https://api.example.com/v1/me`, `https://evil.example.com/v1/me`, 1)
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), payload))
		}},
		{name: "htu scheme mismatch", mutate: func() DPoPInput {
			payload := strings.Replace(validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix()), `https://api.example.com/v1/me`, `http://api.example.com/v1/me`, 1)
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), payload))
		}},
		{name: "wrong access token", mutate: func() DPoPInput {
			input := validDPoPInput(fixture, validProof)
			input.AccessToken = "stolen-token"
			return input
		}},
		{name: "different key", mutate: func() DPoPInput {
			header := `{"alg":"ES256","jwk":` + otherJWK + `,"typ":"dpop+jwt"}`
			return validDPoPInput(fixture, mintDPoP(t, other, header, validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix())))
		}},
		{name: "proof expired", mutate: func() DPoPInput {
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix()-61)))
		}},
		{name: "proof from future", mutate: func() DPoPInput {
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Unix()+61)))
		}},
		{name: "proof after token expiry", mutate: func() DPoPInput {
			input := validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), validDPoPPayload(fixture, canonicalJTI(t), fixture.now.Add(600*time.Second).Unix())))
			input.Now = fixture.now.Add(600 * time.Second)
			return input
		}},
		{name: "short jti", mutate: func() DPoPInput {
			short := base64.RawURLEncoding.EncodeToString(make([]byte, 15))
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), validDPoPPayload(fixture, short, fixture.now.Unix())))
		}},
		{name: "padded jti", mutate: func() DPoPInput {
			raw := make([]byte, 16)
			padded := base64.URLEncoding.EncodeToString(raw)
			return validDPoPInput(fixture, mintDPoP(t, fixture.key, validDPoPHeader(fixture), validDPoPPayload(fixture, padded, fixture.now.Unix())))
		}},
		{name: "legacy token without bindings", mutate: func() DPoPInput {
			input := validDPoPInput(fixture, validProof)
			input.Token = SessionClaims{Subject: "user-1", ExpiresAt: fixture.now.Add(time.Hour)}
			return input
		}},
		{name: "oversized proof", mutate: func() DPoPInput {
			return validDPoPInput(fixture, validProof+strings.Repeat("a", 16*1024))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims, err := VerifyDPoP(test.mutate())
			if err == nil {
				t.Fatalf("VerifyDPoP() = %#v, want error", claims)
			}
			if claims != (ProofClaims{}) {
				t.Fatalf("VerifyDPoP() returned %#v with error %v; want zero value", claims, err)
			}
			if err != ErrInvalidDPoPProof {
				t.Fatalf("VerifyDPoP() error = %v (%T), want sentinel", err, err)
			}
		})
	}
}
