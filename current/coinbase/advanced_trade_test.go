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
	"time"
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

func TestGetAdvancedTradeOrderUsesReadOnlyOrderEndpoint(t *testing.T) {
	credentials := testCredentials(t)
	orderID := "e2ac36ac-25c4-465b-9783-bdef0db2cac1"
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != "https://api.coinbase.com"+historicalOrderPath+orderID {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"order":{"order_id":"e2ac36ac-25c4-465b-9783-bdef0db2cac1"}}`)), Header: make(http.Header)}, nil
	})}
	response, err := GetAdvancedTradeOrder(context.Background(), client, credentials, orderID)
	if err != nil || !strings.Contains(string(response), orderID) {
		t.Fatalf("GetAdvancedTradeOrder() = %s, %v", response, err)
	}
}

func TestListPublicProductsDoesNotSendAuthorization(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != "https://api.coinbase.com"+publicProductsPath {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "" {
			t.Fatal("public products request unexpectedly sent Authorization")
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"products":[]}`)), Header: make(http.Header)}, nil
	})}

	response, err := ListPublicProducts(context.Background(), client)
	if err != nil {
		t.Fatalf("ListPublicProducts() error = %v", err)
	}
	if string(response) != `{"products":[]}` {
		t.Fatalf("ListPublicProducts() = %s", response)
	}
}

func TestGetPublicProductUsesProductPath(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.coinbase.com"+publicProductsPath+"/BTC-USD" {
			t.Fatalf("unexpected URL: %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"product_id":"BTC-USD"}`)), Header: make(http.Header)}, nil
	})}
	response, err := GetPublicProduct(context.Background(), client, "BTC-USD")
	if err != nil || string(response) != `{"product_id":"BTC-USD"}` {
		t.Fatalf("GetPublicProduct() = %s, %v", response, err)
	}
}

func TestGetPublicProductCandlesUsesBoundedUnauthenticatedRequest(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(30 * 24 * time.Hour)
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != publicProductsPath+"/BTC-USDC/candles" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
		}
		query := request.URL.Query()
		if query.Get("start") != "1700000000" || query.Get("end") != "1702592000" || query.Get("granularity") != "ONE_DAY" || query.Get("limit") != "30" {
			t.Fatalf("unexpected candle query: %s", request.URL.RawQuery)
		}
		if request.Header.Get("Authorization") != "" {
			t.Fatal("public candle request unexpectedly sent Authorization")
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"candles":[]}`)), Header: make(http.Header)}, nil
	})}

	response, err := GetPublicProductCandles(context.Background(), client, "BTC-USDC", start, end, "ONE_DAY", 30)
	if err != nil || string(response) != `{"candles":[]}` {
		t.Fatalf("GetPublicProductCandles() = %s, %v", response, err)
	}
}

func TestGetPublicProductCandleHistoryChunksAndDeduplicates(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(351 * 24 * time.Hour)
	calls := 0
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Header.Get("Authorization") != "" {
			t.Fatal("public candle history unexpectedly sent Authorization")
		}
		body := `{"candles":[{"start":"1700000000","open":"1","close":"1"}]}`
		if calls == 2 {
			body = `{"candles":[{"start":"1730240000","open":"2","close":"2"}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}

	response, err := GetPublicProductCandleHistory(context.Background(), client, "BTC-USDC", start, end, "ONE_DAY")
	if err != nil {
		t.Fatalf("GetPublicProductCandleHistory() error = %v", err)
	}
	if calls != 2 || !strings.Contains(string(response), `"start":"1700000000"`) || !strings.Contains(string(response), `"start":"1730240000"`) {
		t.Fatalf("calls = %d, response = %s", calls, response)
	}
}

func TestPreviewOrderPostsOnlyToPreviewEndpoint(t *testing.T) {
	credentials := testCredentials(t)
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "https://api.coinbase.com"+previewOrderPath {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q", request.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ey") {
			t.Fatal("Authorization header does not contain a JWT")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"product_id":"BTC-USD","side":"BUY","order_configuration":{"market_market_ioc":{"quote_size":"10.00","rfq_disabled":true}}}`
		if string(body) != want {
			t.Fatalf("request body = %s", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"preview_id":"preview-123"}`)), Header: make(http.Header)}, nil
	})}

	response, err := PreviewOrder(context.Background(), client, credentials, MarketOrderPreview{ProductID: "BTC-USD", Side: "BUY", QuoteSize: "10.00"})
	if err != nil || string(response) != `{"preview_id":"preview-123"}` {
		t.Fatalf("PreviewOrder() = %s, %v", response, err)
	}
}

func TestPreviewOrderRejectsAmbiguousOrInvalidIntent(t *testing.T) {
	credentials := testCredentials(t)
	for _, preview := range []MarketOrderPreview{
		{ProductID: "BTC-USD", Side: "BUY"},
		{ProductID: "BTC-USD", Side: "BUY", BaseSize: "0.1", QuoteSize: "10"},
		{ProductID: "BTC-USD", Side: "HOLD", QuoteSize: "10"},
		{ProductID: "BTC-USD", Side: "BUY", QuoteSize: "0"},
	} {
		if _, err := PreviewOrder(context.Background(), nil, credentials, preview); err == nil {
			t.Fatalf("PreviewOrder(%+v) unexpectedly succeeded", preview)
		}
	}
}

func TestCreateApprovedOneUSDCBTCBuyPostsOnlyAfterApproval(t *testing.T) {
	credentials := testCredentials(t)
	intent := MarketOrderPreview{ProductID: "BTC-USDC", Side: "BUY", QuoteSize: "1.00"}
	previewID := "2305bea7-c7af-47f2-b087-a7a52ffd85d8"
	phrase, err := intent.ApprovalPhraseForPreview(previewID)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "https://api.coinbase.com"+createOrderPath {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"client_order_id":"2305bea7-c7af-47f2-b087-a7a52ffd85d8","preview_id":"2305bea7-c7af-47f2-b087-a7a52ffd85d8","product_id":"BTC-USDC","side":"BUY","order_configuration":{"market_market_ioc":{"quote_size":"1.00","rfq_disabled":true}}}`
		if string(body) != want {
			t.Fatalf("request body = %s", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"success":true}`)), Header: make(http.Header)}, nil
	})}

	response, err := CreateApprovedOneUSDCBTCBuy(context.Background(), client, credentials, intent, previewID, phrase)
	if err != nil || string(response) != `{"success":true}` {
		t.Fatalf("CreateApprovedOneUSDCBTCBuy() = %s, %v", response, err)
	}
}

func TestCreateApprovedOneUSDCBTCBuyRefusesUnsafeIntentBeforeRequest(t *testing.T) {
	credentials := testCredentials(t)
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsafe live order reached HTTP client")
		return nil, nil
	})}
	previewID := "2305bea7-c7af-47f2-b087-a7a52ffd85d8"
	for _, intent := range []MarketOrderPreview{
		{ProductID: "BTC-USDC", Side: "BUY", QuoteSize: "1.01"},
		{ProductID: "BTC-USD", Side: "BUY", QuoteSize: "1.00"},
		{ProductID: "BTC-USDC", Side: "SELL", QuoteSize: "1.00"},
		{ProductID: "BTC-USDC", Side: "BUY", BaseSize: "0.001"},
	} {
		if _, err := CreateApprovedOneUSDCBTCBuy(context.Background(), client, credentials, intent, previewID, "invalid"); err == nil {
			t.Fatalf("unsafe intent %+v unexpectedly proceeded", intent)
		}
	}
}

func TestRequireLiveOrderApprovalBindsPhraseToExactIntent(t *testing.T) {
	intent := MarketOrderPreview{ProductID: "BTC-USD", Side: "BUY", QuoteSize: "10.00"}
	phrase, err := intent.ApprovalPhrase()
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireLiveOrderApproval(intent, phrase); err != nil {
		t.Fatalf("RequireLiveOrderApproval() error = %v", err)
	}
	if err := RequireLiveOrderApproval(MarketOrderPreview{ProductID: "BTC-USD", Side: "BUY", QuoteSize: "11.00"}, phrase); err == nil {
		t.Fatal("approval phrase unexpectedly approved a different amount")
	}
}

func TestRequireLiveOrderApprovalForPreviewBindsPhraseToPreviewAndIntent(t *testing.T) {
	intent := MarketOrderPreview{ProductID: "BTC-USDC", Side: "BUY", QuoteSize: "1.00"}
	previewID := "2305bea7-c7af-47f2-b087-a7a52ffd85d8"
	phrase, err := intent.ApprovalPhraseForPreview(previewID)
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireLiveOrderApprovalForPreview(intent, previewID, phrase); err != nil {
		t.Fatalf("RequireLiveOrderApprovalForPreview() error = %v", err)
	}
	if err := RequireLiveOrderApprovalForPreview(intent, "e1c62f97-f4da-4ba6-9468-2f0adced3b7f", phrase); err == nil {
		t.Fatal("approval phrase unexpectedly approved a different preview")
	}
	if _, err := intent.ApprovalPhraseForPreview(""); err == nil {
		t.Fatal("empty preview ID unexpectedly produced an approval phrase")
	}
}

func testCredentials(t *testing.T) Credentials {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return Credentials{
		KeyID:     "organizations/test/apiKeys/test",
		KeySecret: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encoded})),
	}
}
