package coinbase

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
)

// Candle is one Coinbase OHLCV bucket used only for research and paper tests.
type Candle struct {
	Start  string `json:"start"`
	Open   string `json:"open"`
	Close  string `json:"close"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Volume string `json:"volume"`
}

// SMABacktest compares a simple moving-average paper strategy against
// buy-and-hold. The signal uses only previous daily closes and fills at the
// following daily open, avoiding same-candle look-ahead. It is research-only.
type SMABacktest struct {
	SMAWindow              int    `json:"sma_window"`
	Candles                int    `json:"candles"`
	FeeRate                string `json:"fee_rate"`
	StrategyTrades         int    `json:"strategy_trades"`
	StrategyEndUSDC        string `json:"strategy_end_usdc"`
	StrategyReturnPct      string `json:"strategy_return_pct"`
	StrategyMaxDrawdownPct string `json:"strategy_max_drawdown_pct"`
	BuyHoldEndUSDC         string `json:"buy_hold_end_usdc"`
	BuyHoldReturnPct       string `json:"buy_hold_return_pct"`
	BuyHoldMaxDrawdownPct  string `json:"buy_hold_max_drawdown_pct"`
	OutperformanceUSDC     string `json:"outperformance_usdc"`
	Method                 string `json:"method"`
}

// SMA7Backtest is retained for callers that use the original SMA-7 report.
type SMA7Backtest = SMABacktest

// BacktestSMA7 is the original seven-day strategy convenience wrapper.
func BacktestSMA7(candleJSON []byte, startingUSDC, feeRate string) (SMABacktest, error) {
	return BacktestSMA(candleJSON, startingUSDC, feeRate, 7)
}

// BacktestSMA parses Coinbase candle JSON and simulates an SMA strategy.
// feeRate is charged on every paper buy and sell and must be a decimal, e.g.
// 0.012 for 1.2%.
func BacktestSMA(candleJSON []byte, startingUSDC, feeRate string, window int) (SMABacktest, error) {
	if window < 2 {
		return SMABacktest{}, fmt.Errorf("SMA window must be at least 2")
	}
	var response struct {
		Candles []Candle `json:"candles"`
	}
	if err := json.Unmarshal(candleJSON, &response); err != nil {
		return SMABacktest{}, fmt.Errorf("decode Coinbase candles: %w", err)
	}
	if len(response.Candles) < window+2 {
		return SMABacktest{}, fmt.Errorf("SMA-%d backtest requires at least %d daily candles", window, window+2)
	}
	for index, candle := range response.Candles {
		if _, err := strconv.ParseInt(candle.Start, 10, 64); err != nil {
			return SMABacktest{}, fmt.Errorf("invalid candle start at index %d", index)
		}
	}
	sort.Slice(response.Candles, func(i, j int) bool {
		left, _ := strconv.ParseInt(response.Candles[i].Start, 10, 64)
		right, _ := strconv.ParseInt(response.Candles[j].Start, 10, 64)
		return left < right
	})
	start, err := positiveRat("starting USDC", startingUSDC)
	if err != nil {
		return SMABacktest{}, err
	}
	fee, err := nonNegativeRat("fee rate", feeRate)
	if err != nil || fee.Cmp(big.NewRat(1, 1)) >= 0 {
		return SMABacktest{}, fmt.Errorf("fee rate must be a decimal from 0 up to but not including 1")
	}
	prices := make([]struct{ open, close *big.Rat }, len(response.Candles))
	for index, candle := range response.Candles {
		open, openErr := positiveRat("candle open", candle.Open)
		close, closeErr := positiveRat("candle close", candle.Close)
		if openErr != nil || closeErr != nil {
			return SMABacktest{}, fmt.Errorf("invalid candle at index %d", index)
		}
		prices[index] = struct{ open, close *big.Rat }{open, close}
	}
	strategyCash := new(big.Rat).Set(start)
	strategyBTC := new(big.Rat)
	trades := 0
	strategyPeak := new(big.Rat).Set(start)
	strategyMaxDrawdown := new(big.Rat)
	for index := window; index < len(prices); index++ {
		sum := new(big.Rat)
		for prior := index - window; prior < index; prior++ {
			sum.Add(sum, prices[prior].close)
		}
		sma := sum.Quo(sum, big.NewRat(int64(window), 1))
		shouldHoldBTC := prices[index-1].close.Cmp(sma) > 0
		if shouldHoldBTC && strategyCash.Sign() > 0 {
			afterFee := new(big.Rat).Mul(strategyCash, new(big.Rat).Sub(big.NewRat(1, 1), fee))
			strategyBTC.Quo(afterFee, prices[index].open)
			strategyCash.SetInt64(0)
			trades++
		}
		if !shouldHoldBTC && strategyBTC.Sign() > 0 {
			proceeds := new(big.Rat).Mul(strategyBTC, prices[index].open)
			strategyCash.Mul(proceeds, new(big.Rat).Sub(big.NewRat(1, 1), fee))
			strategyBTC.SetInt64(0)
			trades++
		}
		strategyValue := new(big.Rat).Add(strategyCash, new(big.Rat).Mul(strategyBTC, prices[index].close))
		updateDrawdown(strategyValue, strategyPeak, strategyMaxDrawdown)
	}
	lastClose := prices[len(prices)-1].close
	strategyEnd := new(big.Rat).Add(strategyCash, new(big.Rat).Mul(strategyBTC, lastClose))
	buyHoldBTC := new(big.Rat).Quo(new(big.Rat).Mul(start, new(big.Rat).Sub(big.NewRat(1, 1), fee)), prices[0].open)
	buyHoldEnd := new(big.Rat).Mul(buyHoldBTC, lastClose)
	buyHoldPeak := new(big.Rat).Set(start)
	buyHoldMaxDrawdown := new(big.Rat)
	for _, price := range prices {
		buyHoldValue := new(big.Rat).Mul(buyHoldBTC, price.close)
		updateDrawdown(buyHoldValue, buyHoldPeak, buyHoldMaxDrawdown)
	}
	return SMABacktest{
		SMAWindow:              window,
		Candles:                len(prices),
		FeeRate:                fee.FloatString(6),
		StrategyTrades:         trades,
		StrategyEndUSDC:        strategyEnd.FloatString(8),
		StrategyReturnPct:      percentageChange(strategyEnd, start),
		StrategyMaxDrawdownPct: strategyMaxDrawdown.FloatString(4),
		BuyHoldEndUSDC:         buyHoldEnd.FloatString(8),
		BuyHoldReturnPct:       percentageChange(buyHoldEnd, start),
		BuyHoldMaxDrawdownPct:  buyHoldMaxDrawdown.FloatString(4),
		OutperformanceUSDC:     new(big.Rat).Sub(strategyEnd, buyHoldEnd).FloatString(8),
		Method:                 fmt.Sprintf("SMA-%d signal from prior closes; next-open fills; fees on every paper trade", window),
	}, nil
}

// SMAResearchWindow is one independent historical window evaluated by the
// research gate. Passing it never authorizes a live order.
type SMAResearchWindow struct {
	Days           int         `json:"days"`
	Result         SMABacktest `json:"result"`
	Passed         bool        `json:"passed"`
	FailureReasons []string    `json:"failure_reasons"`
}

// SMAResearchGate rejects a research rule unless it outperforms buy-and-hold
// after fees and has no larger drawdown in every supplied historical window.
// A pass is a research result only, never authority to submit an order.
type SMAResearchGate struct {
	SMAWindow int                 `json:"sma_window"`
	FeeRate   string              `json:"fee_rate"`
	Passed    bool                `json:"passed"`
	Reason    string              `json:"reason"`
	Windows   []SMAResearchWindow `json:"windows"`
}

// SMARollingResearchWindow records one chronological test period. Its candle
// range does not overlap another window when the caller uses stepDays equal to
// windowDays.
type SMARollingResearchWindow struct {
	Start          string      `json:"start"`
	End            string      `json:"end"`
	Result         SMABacktest `json:"result"`
	Passed         bool        `json:"passed"`
	FailureReasons []string    `json:"failure_reasons"`
}

// SMARollingResearchGate applies the same conservative checks to many
// chronological windows spanning a longer history. It is research-only and
// has no authority to submit an order.
type SMARollingResearchGate struct {
	SMAWindow  int                        `json:"sma_window"`
	WindowDays int                        `json:"window_days"`
	StepDays   int                        `json:"step_days"`
	FeeRate    string                     `json:"fee_rate"`
	Passed     bool                       `json:"passed"`
	Reason     string                     `json:"reason"`
	Windows    []SMARollingResearchWindow `json:"windows"`
}

// EvaluateSMAResearchGate evaluates the newest days in each requested
// duration. The candle payload may contain up to Coinbase's 350 daily buckets.
func EvaluateSMAResearchGate(candleJSON []byte, startingUSDC, feeRate string, smaWindow int, durations []int) (SMAResearchGate, error) {
	if len(durations) == 0 {
		return SMAResearchGate{}, fmt.Errorf("at least one research duration is required")
	}
	var response struct {
		Candles []Candle `json:"candles"`
	}
	if err := json.Unmarshal(candleJSON, &response); err != nil {
		return SMAResearchGate{}, fmt.Errorf("decode Coinbase candles: %w", err)
	}
	if len(response.Candles) == 0 {
		return SMAResearchGate{}, fmt.Errorf("Coinbase returned no candles")
	}
	for index, candle := range response.Candles {
		if _, err := strconv.ParseInt(candle.Start, 10, 64); err != nil {
			return SMAResearchGate{}, fmt.Errorf("invalid candle start at index %d", index)
		}
	}
	sort.Slice(response.Candles, func(i, j int) bool {
		left, _ := strconv.ParseInt(response.Candles[i].Start, 10, 64)
		right, _ := strconv.ParseInt(response.Candles[j].Start, 10, 64)
		return left < right
	})
	gate := SMAResearchGate{SMAWindow: smaWindow, FeeRate: feeRate, Passed: true}
	seen := make(map[int]bool)
	for _, days := range durations {
		if days < smaWindow+2 || days > len(response.Candles) || seen[days] {
			return SMAResearchGate{}, fmt.Errorf("research duration %d must be unique and between %d and %d days", days, smaWindow+2, len(response.Candles))
		}
		seen[days] = true
		payload, err := json.Marshal(struct {
			Candles []Candle `json:"candles"`
		}{Candles: response.Candles[len(response.Candles)-days:]})
		if err != nil {
			return SMAResearchGate{}, fmt.Errorf("encode research window: %w", err)
		}
		report, err := BacktestSMA(payload, startingUSDC, feeRate, smaWindow)
		if err != nil {
			return SMAResearchGate{}, err
		}
		outperformance, _ := new(big.Rat).SetString(report.OutperformanceUSDC)
		strategyDrawdown, _ := new(big.Rat).SetString(report.StrategyMaxDrawdownPct)
		buyHoldDrawdown, _ := new(big.Rat).SetString(report.BuyHoldMaxDrawdownPct)
		window := SMAResearchWindow{Days: days, Result: report, Passed: true}
		if outperformance.Sign() <= 0 {
			window.Passed = false
			window.FailureReasons = append(window.FailureReasons, "did not outperform buy-and-hold after modeled fees")
		}
		if strategyDrawdown.Cmp(buyHoldDrawdown) > 0 {
			window.Passed = false
			window.FailureReasons = append(window.FailureReasons, "maximum drawdown exceeded buy-and-hold")
		}
		if !window.Passed {
			gate.Passed = false
		}
		gate.Windows = append(gate.Windows, window)
	}
	if gate.Passed {
		gate.Reason = "passed research gate only; this does not authorize a live order"
	} else {
		gate.Reason = "rejected: every historical window must beat buy-and-hold after fees with no larger drawdown"
	}
	return gate, nil
}

// EvaluateSMARollingResearchGate evaluates fixed-length chronological windows
// from a longer candle history. A window passes only if the strategy
// outperforms buy-and-hold after fees and has no larger drawdown.
func EvaluateSMARollingResearchGate(candleJSON []byte, startingUSDC, feeRate string, smaWindow, windowDays, stepDays int) (SMARollingResearchGate, error) {
	if smaWindow < 2 || windowDays < smaWindow+2 || stepDays < 1 {
		return SMARollingResearchGate{}, fmt.Errorf("invalid rolling research settings")
	}
	var response struct {
		Candles []Candle `json:"candles"`
	}
	if err := json.Unmarshal(candleJSON, &response); err != nil {
		return SMARollingResearchGate{}, fmt.Errorf("decode Coinbase candles: %w", err)
	}
	if len(response.Candles) < windowDays {
		return SMARollingResearchGate{}, fmt.Errorf("rolling research requires at least %d daily candles", windowDays)
	}
	for index, candle := range response.Candles {
		if _, err := strconv.ParseInt(candle.Start, 10, 64); err != nil {
			return SMARollingResearchGate{}, fmt.Errorf("invalid candle start at index %d", index)
		}
	}
	sort.Slice(response.Candles, func(i, j int) bool {
		left, _ := strconv.ParseInt(response.Candles[i].Start, 10, 64)
		right, _ := strconv.ParseInt(response.Candles[j].Start, 10, 64)
		return left < right
	})
	gate := SMARollingResearchGate{
		SMAWindow: smaWindow, WindowDays: windowDays, StepDays: stepDays, FeeRate: feeRate, Passed: true,
	}
	for first := 0; first+windowDays <= len(response.Candles); first += stepDays {
		candles := response.Candles[first : first+windowDays]
		payload, err := json.Marshal(struct {
			Candles []Candle `json:"candles"`
		}{Candles: candles})
		if err != nil {
			return SMARollingResearchGate{}, fmt.Errorf("encode rolling research window: %w", err)
		}
		report, err := BacktestSMA(payload, startingUSDC, feeRate, smaWindow)
		if err != nil {
			return SMARollingResearchGate{}, err
		}
		outperformance, _ := new(big.Rat).SetString(report.OutperformanceUSDC)
		strategyDrawdown, _ := new(big.Rat).SetString(report.StrategyMaxDrawdownPct)
		buyHoldDrawdown, _ := new(big.Rat).SetString(report.BuyHoldMaxDrawdownPct)
		window := SMARollingResearchWindow{Start: candles[0].Start, End: candles[len(candles)-1].Start, Result: report, Passed: true}
		if outperformance.Sign() <= 0 {
			window.Passed = false
			window.FailureReasons = append(window.FailureReasons, "did not outperform buy-and-hold after modeled fees")
		}
		if strategyDrawdown.Cmp(buyHoldDrawdown) > 0 {
			window.Passed = false
			window.FailureReasons = append(window.FailureReasons, "maximum drawdown exceeded buy-and-hold")
		}
		if !window.Passed {
			gate.Passed = false
		}
		gate.Windows = append(gate.Windows, window)
	}
	if gate.Passed {
		gate.Reason = "passed chronological research windows only; this does not authorize a live order"
	} else {
		gate.Reason = "rejected: every chronological window must beat buy-and-hold after fees with no larger drawdown"
	}
	return gate, nil
}

func positiveRat(name, value string) (*big.Rat, error) {
	decimal, ok := new(big.Rat).SetString(value)
	if !ok || decimal.Sign() <= 0 {
		return nil, fmt.Errorf("%s must be a positive decimal", name)
	}
	return decimal, nil
}

func nonNegativeRat(name, value string) (*big.Rat, error) {
	decimal, ok := new(big.Rat).SetString(value)
	if !ok || decimal.Sign() < 0 {
		return nil, fmt.Errorf("%s must be a non-negative decimal", name)
	}
	return decimal, nil
}

func percentageChange(value, startingValue *big.Rat) string {
	change := new(big.Rat).Sub(value, startingValue)
	return new(big.Rat).Mul(new(big.Rat).Quo(change, startingValue), big.NewRat(100, 1)).FloatString(4)
}

func updateDrawdown(value, peak, maximum *big.Rat) {
	if value.Cmp(peak) > 0 {
		peak.Set(value)
		return
	}
	if peak.Sign() == 0 {
		return
	}
	drawdown := new(big.Rat).Mul(new(big.Rat).Quo(new(big.Rat).Sub(peak, value), peak), big.NewRat(100, 1))
	if drawdown.Cmp(maximum) > 0 {
		maximum.Set(drawdown)
	}
}
