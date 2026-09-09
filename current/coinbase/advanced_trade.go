package coinbase

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const advancedTradeHost = "api.coinbase.com"
const keyPermissionsPath = "/api/v3/brokerage/key_permissions"
const accountsPath = "/api/v3/brokerage/accounts"

// CheckAdvancedTradePermissions performs the least-privileged authenticated
// request available: it reports the permissions assigned to this API key.
func CheckAdvancedTradePermissions(ctx context.Context, client *http.Client, credentials Credentials) ([]byte, error) {
	return getAdvancedTrade(ctx, client, credentials, keyPermissionsPath)
}

// ListAdvancedTradeAccounts retrieves the account list available to a
// view-scoped Advanced Trade key. It does not place orders or move funds.
func ListAdvancedTradeAccounts(ctx context.Context, client *http.Client, credentials Credentials) ([]byte, error) {
	return getAdvancedTrade(ctx, client, credentials, accountsPath)
}

func getAdvancedTrade(ctx context.Context, client *http.Client, credentials Credentials, path string) ([]byte, error) {
	token, err := BuildRESTJWT(credentials, http.MethodGet, advancedTradeHost, path, time.Now())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+advancedTradeHost+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create Coinbase request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call Coinbase key permissions: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read Coinbase response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(body))
		if len(message) > 4096 {
			message = message[:4096] + "…"
		}
		if message == "" {
			return nil, fmt.Errorf("Coinbase GET %s: unexpected status %s", path, response.Status)
		}
		return nil, fmt.Errorf("Coinbase GET %s: unexpected status %s: %s", path, response.Status, message)
	}
	return body, nil
}
