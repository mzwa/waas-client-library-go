package coinbase

import "testing"

func TestCredentialsFromEnv(t *testing.T) {
	t.Setenv("COINBASE_KEY_ID", "organizations/test/apiKeys/test")
	t.Setenv("COINBASE_KEY_SECRET", "test-private-key")

	credentials, err := CredentialsFromEnv()
	if err != nil {
		t.Fatalf("CredentialsFromEnv() error = %v", err)
	}
	if credentials.KeyID != "organizations/test/apiKeys/test" || credentials.KeySecret != "test-private-key" {
		t.Fatalf("CredentialsFromEnv() = %#v", credentials)
	}
}
