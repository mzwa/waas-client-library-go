package coinbase

import (
	"strings"
	"testing"
)

func TestBacktestSMA7SortsCandlesAndReportsDrawdown(t *testing.T) {
	candles := `{"candles":[
{"start":"10","open":"10","close":"9","high":"10","low":"9","volume":"1"},
{"start":"9","open":"10","close":"10","high":"10","low":"10","volume":"1"},
{"start":"8","open":"10","close":"11","high":"11","low":"10","volume":"1"},
{"start":"7","open":"10","close":"12","high":"12","low":"10","volume":"1"},
{"start":"6","open":"10","close":"13","high":"13","low":"10","volume":"1"},
{"start":"5","open":"10","close":"12","high":"12","low":"10","volume":"1"},
{"start":"4","open":"10","close":"11","high":"11","low":"10","volume":"1"},
{"start":"3","open":"10","close":"10","high":"10","low":"10","volume":"1"},
{"start":"2","open":"10","close":"9","high":"10","low":"9","volume":"1"},
{"start":"1","open":"10","close":"8","high":"10","low":"8","volume":"1"}
]}`

	report, err := BacktestSMA7([]byte(candles), "100", "0.01")
	if err != nil {
		t.Fatalf("BacktestSMA7() error = %v", err)
	}
	if report.Candles != 10 {
		t.Fatalf("Candles = %d, want 10", report.Candles)
	}
	if report.StrategyTrades == 0 {
		t.Fatal("StrategyTrades = 0, want a simulated trade")
	}
	if report.StrategyMaxDrawdownPct == "0.0000" || report.BuyHoldMaxDrawdownPct == "0.0000" {
		t.Fatalf("drawdown was not reported: %+v", report)
	}
	if !strings.Contains(report.Method, "next-open") {
		t.Fatalf("Method = %q", report.Method)
	}
}

func TestBacktestSMA7RejectsUnsafeInputs(t *testing.T) {
	if _, err := BacktestSMA7([]byte(`{"candles":[]}`), "100", "0.01"); err == nil {
		t.Fatal("empty candles unexpectedly succeeded")
	}
	valid := `{"candles":[
{"start":"1","open":"10","close":"10"},{"start":"2","open":"10","close":"10"},{"start":"3","open":"10","close":"10"},
{"start":"4","open":"10","close":"10"},{"start":"5","open":"10","close":"10"},{"start":"6","open":"10","close":"10"},
{"start":"7","open":"10","close":"10"},{"start":"8","open":"10","close":"10"},{"start":"9","open":"10","close":"10"}
]}`
	if _, err := BacktestSMA7([]byte(valid), "100", "1"); err == nil {
		t.Fatal("100% fee unexpectedly succeeded")
	}
	if _, err := BacktestSMA7([]byte(valid), "0", "0.01"); err == nil {
		t.Fatal("zero starting capital unexpectedly succeeded")
	}
}
