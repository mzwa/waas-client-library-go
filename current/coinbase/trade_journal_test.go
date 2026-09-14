package coinbase

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTradeJournalChainsAndDetectsTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trades.jsonl")
	status := []byte(`{"order":{"order_id":"e2ac36ac-25c4-465b-9783-bdef0db2cac1","client_order_id":"488b8be3-fa7e-473e-a8bf-bb855a17ebe6","product_id":"BTC-USDC","side":"BUY","status":"FILLED","completion_percentage":"100","filled_size":"0.00001264","average_filled_price":"77478.01","filled_value":"0.98","total_fees":"0.01","total_value_after_fees":"0.99"}}`)
	entry, err := AppendOrderStatusToJournal(path, status, time.Date(2026, 9, 14, 7, 40, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	lastHash, entries, err := VerifyTradeJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if entries != 1 || lastHash != entry.Hash {
		t.Fatalf("VerifyTradeJournal() = %d entries, %q", entries, lastHash)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(contents), "77478.01", "1.00", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyTradeJournal(path); err == nil {
		t.Fatal("tampered journal unexpectedly verified")
	}
}

func TestSummarizeOrderStatusOmitsAccountIdentifiers(t *testing.T) {
	status := []byte(`{"order":{"order_id":"e2ac36ac-25c4-465b-9783-bdef0db2cac1","client_order_id":"488b8be3-fa7e-473e-a8bf-bb855a17ebe6","product_id":"BTC-USDC","side":"BUY","status":"FILLED","completion_percentage":"99.107","filled_size":"0.00001264","average_filled_price":"77478.01","filled_value":"0.9793220464","total_fees":"0.0117518645568","total_value_after_fees":"0.9910739109568","created_time":"2026-09-14T05:36:33Z","last_fill_time":"2026-09-14T05:36:33Z","settled":true,"user_id":"private-user","retail_portfolio_id":"private-portfolio"}}`)
	summary, err := SummarizeOrderStatus(status)
	if err != nil {
		t.Fatal(err)
	}
	if summary.OrderID != "e2ac36ac-25c4-465b-9783-bdef0db2cac1" || !summary.Settled {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	encoded := fmt.Sprintf("%+v", summary)
	if strings.Contains(encoded, "private-user") || strings.Contains(encoded, "private-portfolio") {
		t.Fatalf("summary leaked an account identifier: %s", encoded)
	}
}

func TestVerifyTradeJournalAcceptsEntryFromPreSpendSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trades.jsonl")
	type legacyEntry struct {
		RecordedAt           string `json:"recorded_at"`
		OrderID              string `json:"order_id"`
		PreviewID            string `json:"preview_id"`
		ProductID            string `json:"product_id"`
		Side                 string `json:"side"`
		Status               string `json:"status"`
		CompletionPercentage string `json:"completion_percentage"`
		FilledSize           string `json:"filled_size"`
		AverageFilledPrice   string `json:"average_filled_price"`
		TotalFees            string `json:"total_fees"`
		PreviousHash         string `json:"previous_hash"`
		Hash                 string `json:"hash"`
	}
	entry := legacyEntry{
		RecordedAt:           "2026-09-14T05:40:00Z",
		OrderID:              "e2ac36ac-25c4-465b-9783-bdef0db2cac1",
		PreviewID:            "488b8be3-fa7e-473e-a8bf-bb855a17ebe6",
		ProductID:            "BTC-USDC",
		Side:                 "BUY",
		Status:               "FILLED",
		CompletionPercentage: "99.107",
		FilledSize:           "0.00001264",
		AverageFilledPrice:   "77478.01",
		TotalFees:            "0.0117518645568",
	}
	canonical, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	entry.Hash = hex.EncodeToString(digest[:])
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, entries, err := VerifyTradeJournal(path); err != nil || entries != 1 {
		t.Fatalf("legacy journal verification = %d entries, %v", entries, err)
	}
}
