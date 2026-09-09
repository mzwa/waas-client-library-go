package coinbase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// VaultKVv2 reads a HashiCorp Vault KV v2 secret using the standard Vault HTTP
// API. The token must be constrained to the one requested secret path.
type VaultKVv2 struct {
	Address string
	Token   string
	Client  *http.Client
}

// ReadCredentials loads key_id and key_secret from a Vault KV v2 secret.
func (v VaultKVv2) ReadCredentials(ctx context.Context, mount, secretPath string) (Credentials, error) {
	if strings.TrimSpace(v.Address) == "" || strings.TrimSpace(v.Token) == "" {
		return Credentials{}, fmt.Errorf("VAULT_ADDR and VAULT_TOKEN are required")
	}
	if strings.Trim(mount, "/") == "" || strings.Trim(secretPath, "/") == "" {
		return Credentials{}, fmt.Errorf("Vault mount and secret path are required")
	}

	base, err := url.Parse(v.Address)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return Credentials{}, fmt.Errorf("invalid Vault address")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/" + url.PathEscape(strings.Trim(mount, "/")) + "/data/" + strings.Trim(secretPath, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return Credentials{}, fmt.Errorf("create Vault request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.Token)

	client := v.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return Credentials{}, fmt.Errorf("read Vault secret: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Credentials{}, fmt.Errorf("read Vault secret: unexpected status %s", response.Status)
	}

	var payload struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Credentials{}, fmt.Errorf("decode Vault secret: %w", err)
	}
	credentials := Credentials{KeyID: payload.Data.Data["key_id"], KeySecret: payload.Data.Data["key_secret"]}
	if err := credentials.Validate(); err != nil {
		return Credentials{}, err
	}
	return credentials, nil
}
