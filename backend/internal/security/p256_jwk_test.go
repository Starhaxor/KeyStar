package security

import (
	"encoding/json"
	"strings"
	"testing"
)

// Generator point of P-256 as a JWK (independent fixture).
const (
	testP256X = "axfR8uEsQkf4vOblY6RA8ncDfYEt6zOg9KE5RdiYwpY"
	testP256Y = "T-NC4v4af5uO5-tKfA-eFivOM1drMV7Oy7ZAaDe_UfU"
	// RFC 7638 SHA-256 thumbprint of the generator point above.
	testP256Thumbprint = "xx0BcA-wMohw8atYDJOe6peGModklG2wRHBlXHMvl0M"
)

func validP256JWK() json.RawMessage {
	return json.RawMessage(`{"crv":"P-256","kty":"EC","x":"` + testP256X + `","y":"` + testP256Y + `"}`)
}

func TestP256JWKThumbprintConstructionMatchesRFC7638Section31(t *testing.T) {
	// RFC 7638 section 3.1 example RSA key; only the canonical hash
	// construction is shared with the P-256 path.
	canonical := `{"e":"AQAB","kty":"RSA","n":"0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw"}`
	if got := rfc7638ThumbprintSHA256(canonical); got != "NzbLsXh8uDCcd-6MNwXF4W_7noWXFZAfHkxZsRGC9Xs" {
		t.Fatalf("RFC 7638 section 3.1 thumbprint = %q", got)
	}
}

func TestParseP256JWKAcceptsGeneratorPoint(t *testing.T) {
	key, thumbprint, err := ParseP256JWK(validP256JWK())
	if err != nil {
		t.Fatalf("ParseP256JWK() error = %v", err)
	}
	if key == nil || key.Curve == nil || key.X == nil || key.Y == nil {
		t.Fatalf("ParseP256JWK() returned incomplete key: %#v", key)
	}
	if key.X.Text(16) != "6b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c296" ||
		key.Y.Text(16) != "4fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5" {
		t.Fatalf("ParseP256JWK() coordinates = (%v, %v)", key.X, key.Y)
	}
	if thumbprint != testP256Thumbprint {
		t.Fatalf("ParseP256JWK() thumbprint = %q, want %q", thumbprint, testP256Thumbprint)
	}
}

func TestParseP256JWKRejectsMalformedKeys(t *testing.T) {
	offCurveY := "T-NC4v4af5uO5-tKfA-eFivOM1drMV7Oy7ZAaDe_UfQ"
	shortX := testP256X[:42]
	paddedX := testP256X + "="
	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ``},
		{name: "not JSON", raw: `not-json`},
		{name: "array", raw: `[]`},
		{name: "null", raw: `null`},
		{name: "duplicate member", raw: `{"crv":"P-256","kty":"EC","x":"` + testP256X + `","x":"` + testP256X + `","y":"` + testP256Y + `"}`},
		{name: "extra kid member", raw: `{"crv":"P-256","kty":"EC","x":"` + testP256X + `","y":"` + testP256Y + `","kid":"x"}`},
		{name: "extra alg member", raw: `{"alg":"ES256","crv":"P-256","kty":"EC","x":"` + testP256X + `","y":"` + testP256Y + `"}`},
		{name: "missing x", raw: `{"crv":"P-256","kty":"EC","y":"` + testP256Y + `"}`},
		{name: "missing y", raw: `{"crv":"P-256","kty":"EC","x":"` + testP256X + `"}`},
		{name: "missing kty", raw: `{"crv":"P-256","x":"` + testP256X + `","y":"` + testP256Y + `"}`},
		{name: "missing crv", raw: `{"kty":"EC","x":"` + testP256X + `","y":"` + testP256Y + `"}`},
		{name: "wrong kty", raw: `{"crv":"P-256","kty":"RSA","x":"` + testP256X + `","y":"` + testP256Y + `"}`},
		{name: "wrong crv", raw: `{"crv":"P-384","kty":"EC","x":"` + testP256X + `","y":"` + testP256Y + `"}`},
		{name: "short x", raw: `{"crv":"P-256","kty":"EC","x":"` + shortX + `","y":"` + testP256Y + `"}`},
		{name: "padded x", raw: `{"crv":"P-256","kty":"EC","x":"` + paddedX + `","y":"` + testP256Y + `"}`},
		{name: "standard base64 x", raw: `{"crv":"P-256","kty":"EC","x":"axfR8uEsQkf4vOblY6RA8ncDfYEt6zOg9KE5RdiYwpY","y":"T+NC4v4af5uO5+tKfA+eFivOM1drMV7Oy7ZAaDe/UfU"}`},
		{name: "invalid base64 y", raw: `{"crv":"P-256","kty":"EC","x":"` + testP256X + `","y":"!!!"}`},
		{name: "numeric x", raw: `{"crv":"P-256","kty":"EC","x":42,"y":"` + testP256Y + `"}`},
		{name: "point off curve", raw: `{"crv":"P-256","kty":"EC","x":"` + testP256X + `","y":"` + offCurveY + `"}`},
		{name: "zero point", raw: `{"crv":"P-256","kty":"EC","x":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","y":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`},
		{name: "oversized", raw: `{"crv":"P-256","kty":"EC","x":"` + testP256X + `","y":"` + testP256Y + `"}` + strings.Repeat(" ", 16*1024)},
	} {
		t.Run(test.name, func(t *testing.T) {
			key, thumbprint, err := ParseP256JWK(json.RawMessage(test.raw))
			if err == nil {
				t.Fatalf("ParseP256JWK() = (%v, %q), want error", key, thumbprint)
			}
			if key != nil || thumbprint != "" {
				t.Fatalf("ParseP256JWK() returned (%v, %q) with error %v; want zero values", key, thumbprint, err)
			}
			if strings.Contains(err.Error(), testP256X) || strings.Contains(err.Error(), testP256Y) {
				t.Fatalf("ParseP256JWK() error reflects key material: %v", err)
			}
		})
	}
}
