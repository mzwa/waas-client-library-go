// advanced-trade-smoke validates a Vault-stored Coinbase App API key with a
// authenticated Coinbase integration checker. Live execution is limited to an
// explicitly approved BTC-USDC buy of no more than 1 USDC.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/coinbase/waas-client-library-go/current/coinbase"
)

func main() {
	source := flag.String("source", "vault", "credential source: vault or env")
	mount := flag.String("vault-mount", "secret", "Vault KV v2 mount")
	path := flag.String("vault-path", "coinbase/advanced-trade/live", "Vault secret path")
	resource := flag.String("resource", "permissions", "resource: permissions, accounts, order-status, journal-order-status, verify-journal, paper-buy, performance, backtest-sma7, public-products, public-product, preview-order, approval-phrase, or live-order")
	productID := flag.String("product-id", "BTC-USD", "public product ID used with -resource=public-product")
	side := flag.String("side", "BUY", "order side used with -resource=preview-order: BUY or SELL")
	baseSize := flag.String("base-size", "", "base amount used with -resource=preview-order; set exactly one size")
	quoteSize := flag.String("quote-size", "", "quote amount used with -resource=preview-order; set exactly one size")
	previewID := flag.String("preview-id", "", "Coinbase preview UUID required for approval-phrase or live-order")
	approvalPhrase := flag.String("approval-phrase", "", "exact phrase required for -resource=live-order")
	confirmLiveOrder := flag.String("confirm-live-order", "", "must equal SUBMIT-1-USDC-BTC-USDC-BUY to submit a live order")
	orderID := flag.String("order-id", "", "Coinbase order UUID required for order-status or journal-order-status")
	journalPath := flag.String("journal-path", "", "local JSONL journal path required for journal-order-status or verify-journal")
	killSwitchPath := flag.String("kill-switch-path", "/var/lib/coinbase-trading/DISABLED", "existing file disables live orders")
	backtestDays := flag.Int("backtest-days", 30, "daily candle days used with -resource=backtest-sma7; 9 through 350")
	backtestStartingUSDC := flag.String("backtest-starting-usdc", "20", "paper starting capital used with -resource=backtest-sma7")
	backtestFeeRate := flag.String("backtest-fee-rate", "0.012", "paper fee rate per fill used with -resource=backtest-sma7, e.g. 0.012 for 1.2%")
	flag.Parse()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 60 * time.Second
	client := &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second,
	}
	if *resource == "public-products" {
		response, err := coinbase.ListPublicProducts(context.Background(), client)
		if err != nil {
			log.Fatalf("read Coinbase public products: %v", err)
		}
		fmt.Println(string(response))
		return
	}
	if *resource == "public-product" {
		response, err := coinbase.GetPublicProduct(context.Background(), client, *productID)
		if err != nil {
			log.Fatalf("read Coinbase public product: %v", err)
		}
		fmt.Println(string(response))
		return
	}
	if *resource == "paper-buy" {
		if *journalPath == "" {
			log.Fatal("paper buy policy: -journal-path is required")
		}
		product, err := coinbase.GetPublicProduct(context.Background(), client, "BTC-USDC")
		if err != nil {
			log.Fatalf("read Coinbase public product: %v", err)
		}
		simulation, err := coinbase.SimulateOneUSDCBTCBuy(product, *quoteSize)
		if err != nil {
			log.Fatalf("simulate paper buy: %v", err)
		}
		policy, err := coinbase.EvaluateDailyBuyPolicy(*journalPath, *killSwitchPath, time.Now())
		if err != nil {
			log.Fatalf("evaluate paper-buy policy: %v", err)
		}
		response, err := json.Marshal(struct {
			Simulation coinbase.PaperBuySimulation `json:"simulation"`
			Policy     coinbase.PolicyDecision     `json:"policy"`
		}{simulation, policy})
		if err != nil {
			log.Fatalf("encode paper-buy simulation: %v", err)
		}
		fmt.Println(string(response))
		return
	}
	if *resource == "performance" {
		if *journalPath == "" {
			log.Fatal("performance report: -journal-path is required")
		}
		product, err := coinbase.GetPublicProduct(context.Background(), client, "BTC-USDC")
		if err != nil {
			log.Fatalf("read Coinbase public product: %v", err)
		}
		report, err := coinbase.CalculateBTCUSDCPerformance(*journalPath, product)
		if err != nil {
			log.Fatalf("calculate BTC-USDC performance: %v", err)
		}
		response, err := json.Marshal(report)
		if err != nil {
			log.Fatalf("encode BTC-USDC performance: %v", err)
		}
		fmt.Println(string(response))
		return
	}
	if *resource == "backtest-sma7" {
		if *backtestDays < 9 || *backtestDays > 350 {
			log.Fatal("SMA-7 backtest: -backtest-days must be between 9 and 350")
		}
		end := time.Now().UTC()
		start := end.Add(-time.Duration(*backtestDays) * 24 * time.Hour)
		candles, err := coinbase.GetPublicProductCandles(context.Background(), client, "BTC-USDC", start, end, "ONE_DAY", *backtestDays)
		if err != nil {
			log.Fatalf("read Coinbase daily candles: %v", err)
		}
		report, err := coinbase.BacktestSMA7(candles, *backtestStartingUSDC, *backtestFeeRate)
		if err != nil {
			log.Fatalf("run SMA-7 paper backtest: %v", err)
		}
		response, err := json.Marshal(report)
		if err != nil {
			log.Fatalf("encode SMA-7 backtest: %v", err)
		}
		fmt.Println(string(response))
		return
	}
	intent := coinbase.MarketOrderPreview{
		ProductID: *productID,
		Side:      *side,
		BaseSize:  *baseSize,
		QuoteSize: *quoteSize,
	}
	if *resource == "approval-phrase" {
		phrase, err := intent.ApprovalPhraseForPreview(*previewID)
		if err != nil {
			log.Fatalf("prepare live order approval: %v", err)
		}
		fmt.Println(phrase)
		return
	}
	if *resource == "verify-journal" {
		if *journalPath == "" {
			log.Fatal("verify trade journal: -journal-path is required")
		}
		lastHash, entries, err := coinbase.VerifyTradeJournal(*journalPath)
		if err != nil {
			log.Fatalf("verify trade journal: %v", err)
		}
		fmt.Printf("{\"entries\":%d,\"last_hash\":%q}\n", entries, lastHash)
		return
	}
	if *resource == "live-order" {
		if *confirmLiveOrder != "SUBMIT-1-USDC-BTC-USDC-BUY" {
			log.Fatal("live order refused: pass -confirm-live-order=SUBMIT-1-USDC-BTC-USDC-BUY after reviewing a fresh preview")
		}
		policy, policyErr := coinbase.EvaluateDailyBuyPolicy(*journalPath, *killSwitchPath, time.Now())
		if policyErr != nil {
			log.Fatalf("live order policy: %v", policyErr)
		}
		if !policy.Allowed {
			log.Fatalf("live order refused by policy: %s", policy.Reason)
		}
	}

	var credentials coinbase.Credentials
	var err error
	switch *source {
	case "env":
		credentials, err = coinbase.CredentialsFromEnv()
	case "vault":
		credentials, err = (coinbase.VaultKVv2{
			Address: os.Getenv("VAULT_ADDR"),
			Token:   os.Getenv("VAULT_TOKEN"),
		}).ReadCredentials(context.Background(), *mount, *path)
	default:
		log.Fatalf("unsupported credential source %q; use vault or env", *source)
	}
	if err != nil {
		log.Fatalf("load Coinbase credentials: %v", err)
	}

	var response []byte
	switch *resource {
	case "permissions":
		response, err = coinbase.CheckAdvancedTradePermissions(context.Background(), client, credentials)
	case "accounts":
		response, err = coinbase.ListAdvancedTradeAccounts(context.Background(), client, credentials)
	case "order-status":
		response, err = coinbase.GetAdvancedTradeOrder(context.Background(), client, credentials, *orderID)
		if err == nil {
			summary, summaryErr := coinbase.SummarizeOrderStatus(response)
			if summaryErr != nil {
				log.Fatalf("summarize Coinbase order status: %v", summaryErr)
			}
			response, summaryErr = json.Marshal(summary)
			if summaryErr != nil {
				log.Fatalf("encode Coinbase order summary: %v", summaryErr)
			}
		}
	case "journal-order-status":
		if *journalPath == "" {
			log.Fatal("record trade journal: -journal-path is required")
		}
		response, err = coinbase.GetAdvancedTradeOrder(context.Background(), client, credentials, *orderID)
		if err == nil {
			entry, journalErr := coinbase.AppendOrderStatusToJournal(*journalPath, response, time.Now())
			if journalErr != nil {
				log.Fatalf("record trade journal: %v", journalErr)
			}
			fmt.Printf("journaled order %s with hash %s\n", entry.OrderID, entry.Hash)
			summary, summaryErr := coinbase.SummarizeOrderStatus(response)
			if summaryErr != nil {
				log.Fatalf("summarize Coinbase order status: %v", summaryErr)
			}
			response, summaryErr = json.Marshal(summary)
			if summaryErr != nil {
				log.Fatalf("encode Coinbase order summary: %v", summaryErr)
			}
		}
	case "preview-order":
		response, err = coinbase.PreviewOrder(context.Background(), client, credentials, intent)
	case "live-order":
		response, err = coinbase.CreateApprovedOneUSDCBTCBuy(context.Background(), client, credentials, intent, *previewID, *approvalPhrase)
	default:
		log.Fatalf("unsupported resource %q", *resource)
	}
	if err != nil {
		log.Fatalf("read Coinbase Advanced Trade data: %v", err)
	}
	fmt.Println(string(response))
}
