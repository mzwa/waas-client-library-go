package coinbase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTradeJournalChainsAndDetectsTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trades.jsonl")
	status := []byte(`{"order":{"order_id":"e2ac36ac-25c4-465b-9783-bdef0db2cac1","client_order_id":"488b8be3-fa7e-473e-a8bf-bb855a17ebe6","product_id":"BTC-USDC","side":"BUY","status":"FILLED","completion_percentage":"100","filled_size":"0.00001264","average_filled_price":"77478.01","total_fees":"0.01"}}`)
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
