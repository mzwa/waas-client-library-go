package coinbase

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

const johannesburgTimeZone = "Africa/Johannesburg"

// DailyBuyPolicy is intentionally conservative for the first live-trading
// stage: one BTC-USDC buy of at most 1 USDC per Johannesburg calendar day.
type DailyBuyPolicy struct {
	MaxOrdersPerDay int    `json:"max_orders_per_day"`
	MaxSpendUSDC    string `json:"max_spend_usdc"`
	TimeZone        string `json:"time_zone"`
}

// DefaultDailyBuyPolicy is the currently approved live policy.
var DefaultDailyBuyPolicy = DailyBuyPolicy{
	MaxOrdersPerDay: 1,
	MaxSpendUSDC:    "1",
	TimeZone:        johannesburgTimeZone,
}

// PolicyDecision describes whether another one-USDC BTC-USDC buy is allowed
// without submitting or modifying anything.
type PolicyDecision struct {
	Allowed           bool           `json:"allowed"`
	Reason            string         `json:"reason,omitempty"`
	Policy            DailyBuyPolicy `json:"policy"`
	OrdersToday       int            `json:"orders_today"`
	SpendTodayUSDC    string         `json:"spend_today_usdc"`
	KillSwitchEnabled bool           `json:"kill_switch_enabled"`
}

// EvaluateDailyBuyPolicy checks the verified journal and optional kill-switch
// path. It never sends a Coinbase request. The kill switch is enabled when the
// file exists.
func EvaluateDailyBuyPolicy(journalPath, killSwitchPath string, now time.Time) (PolicyDecision, error) {
	decision := PolicyDecision{Policy: DefaultDailyBuyPolicy, SpendTodayUSDC: "0"}
	if strings.TrimSpace(journalPath) == "" {
		return decision, fmt.Errorf("trade journal path is required for live-order policy")
	}
	if strings.TrimSpace(killSwitchPath) != "" {
		if _, err := os.Stat(killSwitchPath); err == nil {
			decision.KillSwitchEnabled = true
			decision.Reason = "live trading is disabled by the kill-switch file"
			return decision, nil
		} else if !os.IsNotExist(err) {
			return decision, fmt.Errorf("check live-trading kill switch: %w", err)
		}
	}
	journal, err := ReadTradeJournal(journalPath)
	if err != nil {
		return decision, err
	}
	location, err := time.LoadLocation(DefaultDailyBuyPolicy.TimeZone)
	if err != nil {
		return decision, fmt.Errorf("load policy time zone: %w", err)
	}
	today := now.In(location).Format("2006-01-02")
	spend := new(big.Rat)
	for _, entry := range journal {
		recordedAt, err := time.Parse(time.RFC3339Nano, entry.RecordedAt)
		if err != nil || recordedAt.In(location).Format("2006-01-02") != today || entry.ProductID != liveOrderProductID || entry.Side != liveOrderSide {
			continue
		}
		decision.OrdersToday++
		if amount, ok := journalEntrySpend(entry); ok {
			spend.Add(spend, amount)
		}
	}
	decision.SpendTodayUSDC = spend.FloatString(8)
	maxSpend, _ := new(big.Rat).SetString(DefaultDailyBuyPolicy.MaxSpendUSDC)
	if decision.OrdersToday >= DefaultDailyBuyPolicy.MaxOrdersPerDay {
		decision.Reason = "daily order limit already reached"
		return decision, nil
	}
	if spend.Cmp(maxSpend) >= 0 {
		decision.Reason = "daily USDC spend limit already reached"
		return decision, nil
	}
	decision.Allowed = true
	return decision, nil
}

func journalEntrySpend(entry TradeJournalEntry) (*big.Rat, bool) {
	if amount, ok := new(big.Rat).SetString(entry.TotalValueAfterFees); ok && amount.Sign() >= 0 {
		return amount, true
	}
	filledSize, sizeOK := new(big.Rat).SetString(entry.FilledSize)
	averagePrice, priceOK := new(big.Rat).SetString(entry.AverageFilledPrice)
	if !sizeOK || !priceOK || filledSize.Sign() < 0 || averagePrice.Sign() < 0 {
		return nil, false
	}
	spend := new(big.Rat).Mul(filledSize, averagePrice)
	if entry.TotalFees == "" {
		return spend, true
	}
	fees, feesOK := new(big.Rat).SetString(entry.TotalFees)
	if !feesOK || fees.Sign() < 0 {
		return nil, false
	}
	return spend.Add(spend, fees), true
}

// PaperBuySimulation estimates a BTC-USDC market buy using current public
// product data. It never uses credentials and cannot place an order.
type PaperBuySimulation struct {
	ProductID         string `json:"product_id"`
	Side              string `json:"side"`
	QuoteSizeUSDC     string `json:"quote_size_usdc"`
	ReferencePrice    string `json:"reference_price_usdc"`
	EstimatedBTC      string `json:"estimated_btc_excluding_fees"`
	FeeEstimateNotice string `json:"fee_estimate_notice"`
}

// BTCUSDCPerformance reports the mark-to-market result for BTC-USDC buys that
// remain in the local journal. The current project has no sell path, so all
// filled buys are treated as still held. It is read-only and uses public price
// data only.
type BTCUSDCPerformance struct {
	JournalEntries      int    `json:"journal_entries"`
	BTCHeld             string `json:"btc_held"`
	CostBasisUSDC       string `json:"cost_basis_usdc"`
	CurrentPriceUSDC    string `json:"current_price_usdc"`
	CurrentValueUSDC    string `json:"current_value_usdc"`
	UnrealizedPnLUSDC   string `json:"unrealized_pnl_usdc"`
	UnrealizedReturnPct string `json:"unrealized_return_pct"`
}

// CalculateBTCUSDCPerformance combines a verified journal with one public
// BTC-USDC product response. It never reads credentials or sends an order.
func CalculateBTCUSDCPerformance(journalPath string, productJSON []byte) (BTCUSDCPerformance, error) {
	journal, err := ReadTradeJournal(journalPath)
	if err != nil {
		return BTCUSDCPerformance{}, err
	}
	var product struct {
		ProductID string `json:"product_id"`
		Price     string `json:"price"`
	}
	if err := json.Unmarshal(productJSON, &product); err != nil {
		return BTCUSDCPerformance{}, fmt.Errorf("decode Coinbase public product: %w", err)
	}
	if product.ProductID != liveOrderProductID {
		return BTCUSDCPerformance{}, fmt.Errorf("performance refused: expected %s public product", liveOrderProductID)
	}
	price, ok := new(big.Rat).SetString(product.Price)
	if !ok || price.Sign() <= 0 {
		return BTCUSDCPerformance{}, fmt.Errorf("performance refused: Coinbase product price is invalid")
	}
	btcHeld := new(big.Rat)
	costBasis := new(big.Rat)
	entries := 0
	for _, entry := range journal {
		if entry.ProductID != liveOrderProductID || entry.Side != liveOrderSide || entry.Status != "FILLED" {
			continue
		}
		btc, ok := new(big.Rat).SetString(entry.FilledSize)
		if !ok || btc.Sign() < 0 {
			return BTCUSDCPerformance{}, fmt.Errorf("performance refused: journal has invalid BTC fill for order %s", entry.OrderID)
		}
		spend, ok := journalEntrySpend(entry)
		if !ok {
			return BTCUSDCPerformance{}, fmt.Errorf("performance refused: journal has invalid spend for order %s", entry.OrderID)
		}
		btcHeld.Add(btcHeld, btc)
		costBasis.Add(costBasis, spend)
		entries++
	}
	if entries == 0 || costBasis.Sign() == 0 {
		return BTCUSDCPerformance{}, fmt.Errorf("performance unavailable: no filled BTC-USDC buys in journal")
	}
	currentValue := new(big.Rat).Mul(btcHeld, price)
	pnl := new(big.Rat).Sub(currentValue, costBasis)
	returnPercent := new(big.Rat).Mul(new(big.Rat).Quo(pnl, costBasis), big.NewRat(100, 1))
	return BTCUSDCPerformance{
		JournalEntries:      entries,
		BTCHeld:             btcHeld.FloatString(16),
		CostBasisUSDC:       costBasis.FloatString(8),
		CurrentPriceUSDC:    price.FloatString(2),
		CurrentValueUSDC:    currentValue.FloatString(8),
		UnrealizedPnLUSDC:   pnl.FloatString(8),
		UnrealizedReturnPct: returnPercent.FloatString(4),
	}, nil
}

// SimulateOneUSDCBTCBuy calculates a paper buy from Coinbase public-product
// JSON. Fees are deliberately not guessed; use an authenticated preview for a
// fee quote before any real order.
func SimulateOneUSDCBTCBuy(productJSON []byte, quoteSize string) (PaperBuySimulation, error) {
	intent := MarketOrderPreview{ProductID: liveOrderProductID, Side: liveOrderSide, QuoteSize: quoteSize}
	if err := validateOneUSDCBTCBuy(intent); err != nil {
		return PaperBuySimulation{}, err
	}
	var product struct {
		ProductID string `json:"product_id"`
		Price     string `json:"price"`
	}
	if err := json.Unmarshal(productJSON, &product); err != nil {
		return PaperBuySimulation{}, fmt.Errorf("decode Coinbase public product: %w", err)
	}
	if product.ProductID != liveOrderProductID {
		return PaperBuySimulation{}, fmt.Errorf("paper trade refused: expected %s public product", liveOrderProductID)
	}
	price, ok := new(big.Rat).SetString(product.Price)
	if !ok || price.Sign() <= 0 {
		return PaperBuySimulation{}, fmt.Errorf("paper trade refused: Coinbase product price is invalid")
	}
	quote, _ := new(big.Rat).SetString(strings.TrimSpace(quoteSize))
	estimatedBTC := new(big.Rat).Quo(quote, price)
	return PaperBuySimulation{
		ProductID:         liveOrderProductID,
		Side:              liveOrderSide,
		QuoteSizeUSDC:     strings.TrimSpace(quoteSize),
		ReferencePrice:    product.Price,
		EstimatedBTC:      estimatedBTC.FloatString(16),
		FeeEstimateNotice: "fees excluded; use a fresh authenticated preview before a real order",
	}, nil
}
