package coinbase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadCredentialsUsesKVv2DataEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/secret/data/coinbase/advanced-trade/live" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Vault-Token") != "test-token" {
			t.Fatal("Vault token was not passed")
		}
		_, _ = w.Write([]byte(`{"data":{"data":{"key_id":"organizations/a/apiKeys/b","key_secret":"private-key"}}}`))
	}))
	defer server.Close()

	credentials, err := (VaultKVv2{Address: server.URL, Token: "test-token"}).ReadCredentials(context.Background(), "secret", "coinbase/advanced-trade/live")
	if err != nil {
		t.Fatalf("ReadCredentials() error = %v", err)
	}
	if credentials.KeyID != "organizations/a/apiKeys/b" || credentials.KeySecret != "private-key" {
		t.Fatalf("ReadCredentials() = %#v", credentials)
	}
}
