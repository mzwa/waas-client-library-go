package coinbase

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvaluateDailyBuyPolicyEnforcesOneOrderPerJohannesburgDay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trades.jsonl")
	status := []byte(`{"order":{"order_id":"e2ac36ac-25c4-465b-9783-bdef0db2cac1","client_order_id":"488b8be3-fa7e-473e-a8bf-bb855a17ebe6","product_id":"BTC-USDC","side":"BUY","status":"FILLED","completion_percentage":"100","filled_size":"0.00001264","average_filled_price":"77478.01","total_fees":"0.01"}}`)
	if _, err := AppendOrderStatusToJournal(path, status, time.Date(2026, 9, 14, 5, 40, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluateDailyBuyPolicy(path, "", time.Date(2026, 9, 14, 20, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.OrdersToday != 1 || decision.SpendTodayUSDC != "0.98932205" || decision.Reason != "daily order limit already reached" {
		t.Fatalf("unexpected policy decision: %+v", decision)
	}
}

func TestEvaluateDailyBuyPolicyHonorsKillSwitch(t *testing.T) {
	directory := t.TempDir()
	killSwitch := filepath.Join(directory, "DISABLED")
	if err := os.WriteFile(killSwitch, nil, 0600); err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluateDailyBuyPolicy(filepath.Join(directory, "trades.jsonl"), killSwitch, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || !decision.KillSwitchEnabled {
		t.Fatalf("kill switch did not deny policy: %+v", decision)
	}
}

func TestSimulateOneUSDCBTCBuyUsesPublicPriceOnly(t *testing.T) {
	simulation, err := SimulateOneUSDCBTCBuy([]byte(`{"product_id":"BTC-USDC","price":"80000"}`), "1.00")
	if err != nil {
		t.Fatal(err)
	}
	if simulation.EstimatedBTC != "0.0000125000000000" || simulation.ReferencePrice != "80000" {
		t.Fatalf("unexpected simulation: %+v", simulation)
	}
}
