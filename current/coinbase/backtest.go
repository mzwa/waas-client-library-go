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

// SMA7Backtest compares a seven-day moving-average paper strategy against
// buy-and-hold. The signal uses only previous daily closes and fills at the
// following daily open, avoiding same-candle look-ahead. It is research-only.
type SMA7Backtest struct {
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

// BacktestSMA7 parses Coinbase candle JSON and simulates an SMA-7 strategy.
// feeRate is charged on every paper buy and sell and must be a decimal, e.g.
// 0.012 for 1.2%.
func BacktestSMA7(candleJSON []byte, startingUSDC, feeRate string) (SMA7Backtest, error) {
	var response struct {
		Candles []Candle `json:"candles"`
	}
	if err := json.Unmarshal(candleJSON, &response); err != nil {
		return SMA7Backtest{}, fmt.Errorf("decode Coinbase candles: %w", err)
	}
	if len(response.Candles) < 9 {
		return SMA7Backtest{}, fmt.Errorf("SMA-7 backtest requires at least 9 daily candles")
	}
	for index, candle := range response.Candles {
		if _, err := strconv.ParseInt(candle.Start, 10, 64); err != nil {
			return SMA7Backtest{}, fmt.Errorf("invalid candle start at index %d", index)
		}
	}
	sort.Slice(response.Candles, func(i, j int) bool {
		left, _ := strconv.ParseInt(response.Candles[i].Start, 10, 64)
		right, _ := strconv.ParseInt(response.Candles[j].Start, 10, 64)
		return left < right
	})
	start, err := positiveRat("starting USDC", startingUSDC)
	if err != nil {
		return SMA7Backtest{}, err
	}
	fee, err := nonNegativeRat("fee rate", feeRate)
	if err != nil || fee.Cmp(big.NewRat(1, 1)) >= 0 {
		return SMA7Backtest{}, fmt.Errorf("fee rate must be a decimal from 0 up to but not including 1")
	}
	prices := make([]struct{ open, close *big.Rat }, len(response.Candles))
	for index, candle := range response.Candles {
		open, openErr := positiveRat("candle open", candle.Open)
		close, closeErr := positiveRat("candle close", candle.Close)
		if openErr != nil || closeErr != nil {
			return SMA7Backtest{}, fmt.Errorf("invalid candle at index %d", index)
		}
		prices[index] = struct{ open, close *big.Rat }{open, close}
	}
	strategyCash := new(big.Rat).Set(start)
	strategyBTC := new(big.Rat)
	trades := 0
	strategyPeak := new(big.Rat).Set(start)
	strategyMaxDrawdown := new(big.Rat)
	for index := 7; index < len(prices); index++ {
		sum := new(big.Rat)
		for prior := index - 7; prior < index; prior++ {
			sum.Add(sum, prices[prior].close)
		}
		sma := sum.Quo(sum, big.NewRat(7, 1))
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
	return SMA7Backtest{
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
		Method:                 "SMA-7 signal from prior closes; next-open fills; fees on every paper trade",
	}, nil
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
