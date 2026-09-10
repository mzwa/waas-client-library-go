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
