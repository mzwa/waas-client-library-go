package coinbase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const advancedTradeHost = "api.coinbase.com"
const keyPermissionsPath = "/api/v3/brokerage/key_permissions"
const accountsPath = "/api/v3/brokerage/accounts"
const publicProductsPath = "/api/v3/brokerage/market/products"
const previewOrderPath = "/api/v3/brokerage/orders/preview"

// MarketOrderPreview is a non-executable market-order request. Exactly one of
// BaseSize and QuoteSize must be provided. It is accepted by PreviewOrder only;
// this package deliberately contains no method that creates an order.
type MarketOrderPreview struct {
	ProductID string
	Side      string
	BaseSize  string
	QuoteSize string
}

type marketOrderPreviewPayload struct {
	ProductID          string `json:"product_id"`
	Side               string `json:"side"`
	OrderConfiguration struct {
		MarketMarketIOC struct {
			BaseSize    string `json:"base_size,omitempty"`
			QuoteSize   string `json:"quote_size,omitempty"`
			RFQDisabled bool   `json:"rfq_disabled"`
		} `json:"market_market_ioc"`
	} `json:"order_configuration"`
}

// Validate ensures that an order preview is unambiguous before it is sent to
// Coinbase. It accepts decimal values only, avoiding float rounding.
func (p MarketOrderPreview) Validate() error {
	if strings.TrimSpace(p.ProductID) == "" {
		return fmt.Errorf("Coinbase product ID is required")
	}
	if p.Side != "BUY" && p.Side != "SELL" {
		return fmt.Errorf("Coinbase order side must be BUY or SELL")
	}
	if (strings.TrimSpace(p.BaseSize) == "") == (strings.TrimSpace(p.QuoteSize) == "") {
		return fmt.Errorf("provide exactly one of base size or quote size")
	}
	if p.BaseSize != "" {
		if err := validatePositiveDecimal("base size", p.BaseSize); err != nil {
			return err
		}
	}
	if p.QuoteSize != "" {
		if err := validatePositiveDecimal("quote size", p.QuoteSize); err != nil {
			return err
		}
	}
	return nil
}

func validatePositiveDecimal(name, value string) error {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || (len(parts) == 2 && len(parts[1]) == 0) {
		return fmt.Errorf("Coinbase %s must be a positive decimal", name)
	}
	for _, part := range parts {
		for _, character := range part {
			if character < '0' || character > '9' {
				return fmt.Errorf("Coinbase %s must be a positive decimal", name)
			}
		}
	}
	decimal, ok := new(big.Rat).SetString(value)
	if !ok || decimal.Sign() <= 0 {
		return fmt.Errorf("Coinbase %s must be a positive decimal", name)
	}
	return nil
}

// PreviewOrder asks Coinbase to calculate the outcome, fees, and validation
// errors for a prospective order. Previewing never submits an order.
func PreviewOrder(ctx context.Context, client *http.Client, credentials Credentials, preview MarketOrderPreview) ([]byte, error) {
	if err := preview.Validate(); err != nil {
		return nil, err
	}
	payload := marketOrderPreviewPayload{ProductID: preview.ProductID, Side: preview.Side}
	payload.OrderConfiguration.MarketMarketIOC.BaseSize = strings.TrimSpace(preview.BaseSize)
	payload.OrderConfiguration.MarketMarketIOC.QuoteSize = strings.TrimSpace(preview.QuoteSize)
	payload.OrderConfiguration.MarketMarketIOC.RFQDisabled = true
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Coinbase order preview: %w", err)
	}
	return postAdvancedTrade(ctx, client, credentials, previewOrderPath, body)
}

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

// ListPublicProducts retrieves public Advanced Trade product metadata. It does
// not require credentials and cannot access account data or place orders.
func ListPublicProducts(ctx context.Context, client *http.Client) ([]byte, error) {
	return getPublicAdvancedTrade(ctx, client, publicProductsPath)
}

// GetPublicProduct retrieves metadata for one public market, such as BTC-USD.
func GetPublicProduct(ctx context.Context, client *http.Client, productID string) ([]byte, error) {
	if strings.TrimSpace(productID) == "" {
		return nil, fmt.Errorf("Coinbase product ID is required")
	}
	return getPublicAdvancedTrade(ctx, client, publicProductsPath+"/"+url.PathEscape(productID))
}

func getPublicAdvancedTrade(ctx context.Context, client *http.Client, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+advancedTradeHost+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create Coinbase public products request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call Coinbase public products: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read Coinbase public products response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Coinbase public GET %s: unexpected status %s", path, response.Status)
	}
	return body, nil
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

func postAdvancedTrade(ctx context.Context, client *http.Client, credentials Credentials, path string, requestBody []byte) ([]byte, error) {
	token, err := BuildRESTJWT(credentials, http.MethodPost, advancedTradeHost, path, time.Now())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+advancedTradeHost+path, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("create Coinbase request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call Coinbase order preview: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read Coinbase order preview response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(body))
		if len(message) > 4096 {
			message = message[:4096] + "…"
		}
		if message == "" {
			return nil, fmt.Errorf("Coinbase POST %s: unexpected status %s", path, response.Status)
		}
		return nil, fmt.Errorf("Coinbase POST %s: unexpected status %s: %s", path, response.Status, message)
	}
	return body, nil
}
