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
	"sort"
	"strconv"
	"strings"
	"time"
)

const advancedTradeHost = "api.coinbase.com"
const keyPermissionsPath = "/api/v3/brokerage/key_permissions"
const accountsPath = "/api/v3/brokerage/accounts"
const publicProductsPath = "/api/v3/brokerage/market/products"
const previewOrderPath = "/api/v3/brokerage/orders/preview"
const createOrderPath = "/api/v3/brokerage/orders"
const historicalOrderPath = "/api/v3/brokerage/orders/historical/"

const (
	liveOrderProductID            = "BTC-USDC"
	liveOrderSide                 = "BUY"
	liveOrderMaximumQuoteSizeUSDC = "1"
)

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

type liveMarketOrderPayload struct {
	ClientOrderID      string `json:"client_order_id"`
	PreviewID          string `json:"preview_id"`
	ProductID          string `json:"product_id"`
	Side               string `json:"side"`
	OrderConfiguration struct {
		MarketMarketIOC struct {
			QuoteSize   string `json:"quote_size"`
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

// CreateApprovedOneUSDCBTCBuy creates one narrowly-scoped market buy. It is
// deliberately not a general-purpose order API: it can create only a BTC-USDC
// BUY with a quote size no greater than 1 USDC. The Coinbase preview ID serves
// as the client order ID, making retries for the same preview idempotent.
//
// The supplied approval phrase is verified immediately before the request is
// sent. Callers must obtain a fresh preview ID because Coinbase may reject an
// expired or changed preview.
func CreateApprovedOneUSDCBTCBuy(ctx context.Context, client *http.Client, credentials Credentials, intent MarketOrderPreview, previewID, approvalPhrase string) ([]byte, error) {
	if err := validateOneUSDCBTCBuy(intent); err != nil {
		return nil, err
	}
	if err := RequireLiveOrderApprovalForPreview(intent, previewID, approvalPhrase); err != nil {
		return nil, err
	}
	payload := liveMarketOrderPayload{
		ClientOrderID: strings.TrimSpace(previewID),
		PreviewID:     strings.TrimSpace(previewID),
		ProductID:     liveOrderProductID,
		Side:          liveOrderSide,
	}
	payload.OrderConfiguration.MarketMarketIOC.QuoteSize = strings.TrimSpace(intent.QuoteSize)
	payload.OrderConfiguration.MarketMarketIOC.RFQDisabled = true
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Coinbase live order: %w", err)
	}
	return postAdvancedTrade(ctx, client, credentials, createOrderPath, body)
}

func validateOneUSDCBTCBuy(intent MarketOrderPreview) error {
	if err := intent.Validate(); err != nil {
		return err
	}
	if intent.ProductID != liveOrderProductID || intent.Side != liveOrderSide || strings.TrimSpace(intent.BaseSize) != "" {
		return fmt.Errorf("live order refused: only a %s %s market buy with quote size is allowed", liveOrderProductID, liveOrderSide)
	}
	quoteSize, ok := new(big.Rat).SetString(strings.TrimSpace(intent.QuoteSize))
	if !ok || quoteSize.Cmp(big.NewRat(1, 1)) > 0 {
		return fmt.Errorf("live order refused: quote size may not exceed %s USDC", liveOrderMaximumQuoteSizeUSDC)
	}
	return nil
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

// GetAdvancedTradeOrder retrieves Coinbase's final record for one order. It is
// read-only and never modifies an order, portfolio, or balance.
func GetAdvancedTradeOrder(ctx context.Context, client *http.Client, credentials Credentials, orderID string) ([]byte, error) {
	orderID = strings.TrimSpace(orderID)
	if !isUUID(orderID) {
		return nil, fmt.Errorf("a UUID Coinbase order ID is required")
	}
	return getAdvancedTrade(ctx, client, credentials, historicalOrderPath+url.PathEscape(orderID))
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

// GetPublicProductCandles retrieves public OHLCV candles without credentials.
// Coinbase limits candle responses to 350 buckets, so callers should request a
// bounded time range.
func GetPublicProductCandles(ctx context.Context, client *http.Client, productID string, start, end time.Time, granularity string, limit int) ([]byte, error) {
	if strings.TrimSpace(productID) == "" {
		return nil, fmt.Errorf("Coinbase product ID is required")
	}
	if !end.After(start) {
		return nil, fmt.Errorf("candle end time must be after start time")
	}
	if strings.TrimSpace(granularity) == "" {
		return nil, fmt.Errorf("candle granularity is required")
	}
	if limit < 1 || limit > 350 {
		return nil, fmt.Errorf("candle limit must be between 1 and 350")
	}
	query := url.Values{}
	query.Set("start", strconv.FormatInt(start.Unix(), 10))
	query.Set("end", strconv.FormatInt(end.Unix(), 10))
	query.Set("granularity", granularity)
	query.Set("limit", strconv.Itoa(limit))
	path := publicProductsPath + "/" + url.PathEscape(productID) + "/candles?" + query.Encode()
	return getPublicAdvancedTrade(ctx, client, path)
}

// GetPublicProductCandleHistory retrieves a longer public candle range in
// Coinbase's maximum 350-bucket chunks. Boundary candles are de-duplicated by
// their start timestamp. It uses no credentials.
func GetPublicProductCandleHistory(ctx context.Context, client *http.Client, productID string, start, end time.Time, granularity string) ([]byte, error) {
	if !end.After(start) {
		return nil, fmt.Errorf("candle end time must be after start time")
	}
	byStart := make(map[string]Candle)
	for cursor := start; cursor.Before(end); {
		next := cursor.Add(350 * 24 * time.Hour)
		if next.After(end) {
			next = end
		}
		page, err := GetPublicProductCandles(ctx, client, productID, cursor, next, granularity, 350)
		if err != nil {
			return nil, err
		}
		var response struct {
			Candles []Candle `json:"candles"`
		}
		if err := json.Unmarshal(page, &response); err != nil {
			return nil, fmt.Errorf("decode Coinbase candle page: %w", err)
		}
		for _, candle := range response.Candles {
			byStart[candle.Start] = candle
		}
		cursor = next
	}
	candles := make([]Candle, 0, len(byStart))
	for _, candle := range byStart {
		candles = append(candles, candle)
	}
	sort.Slice(candles, func(i, j int) bool {
		left, _ := strconv.ParseInt(candles[i].Start, 10, 64)
		right, _ := strconv.ParseInt(candles[j].Start, 10, 64)
		return left < right
	})
	return json.Marshal(struct {
		Candles []Candle `json:"candles"`
	}{Candles: candles})
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
		return nil, fmt.Errorf("call Coinbase POST %s: %w", path, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read Coinbase POST %s response: %w", path, err)
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
