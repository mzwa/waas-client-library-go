package coinbase

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"gopkg.in/square/go-jose.v2/jwt"
)

func TestBuildRESTJWTBindsTokenToRequest(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	credentials := Credentials{
		KeyID:     "organizations/test/apiKeys/test",
		KeySecret: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encoded})),
	}

	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	token, err := BuildRESTJWT(credentials, "get", "api.coinbase.com", "/api/v3/brokerage/key_permissions", now)
	if err != nil {
		t.Fatalf("BuildRESTJWT() error = %v", err)
	}

	parsed, err := jwt.ParseSigned(token)
	if err != nil {
		t.Fatalf("ParseSigned() error = %v", err)
	}
	var claims struct {
		Issuer  string `json:"iss"`
		Subject string `json:"sub"`
		URI     string `json:"uri"`
		*jwt.Claims
	}
	if err := parsed.UnsafeClaimsWithoutVerification(&claims); err != nil {
		t.Fatalf("UnsafeClaimsWithoutVerification() error = %v", err)
	}
	if claims.Issuer != "cdp" || claims.Subject != credentials.KeyID {
		t.Fatalf("wrong principal claims: %#v", claims)
	}
	if claims.URI != "GET api.coinbase.com/api/v3/brokerage/key_permissions" {
		t.Fatalf("wrong URI claim: %q", claims.URI)
	}
	if !claims.Expiry.Time().Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("wrong expiry: %v", claims.Expiry)
	}
}
