package coinbase

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestListAdvancedTradeAccountsUsesAuthenticatedReadOnlyRequest(t *testing.T) {
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

	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != "https://api.coinbase.com"+accountsPath {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Accept") != "application/json" {
			t.Fatalf("Accept header = %q", request.Header.Get("Accept"))
		}
		if !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ey") {
			t.Fatalf("Authorization header does not contain a JWT")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"accounts":[]}`)),
			Header:     make(http.Header),
		}, nil
	})}

	response, err := ListAdvancedTradeAccounts(context.Background(), client, credentials)
	if err != nil {
		t.Fatalf("ListAdvancedTradeAccounts() error = %v", err)
	}
	if string(response) != `{"accounts":[]}` {
		t.Fatalf("ListAdvancedTradeAccounts() = %s", response)
	}
}
