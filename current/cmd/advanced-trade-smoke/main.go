// advanced-trade-smoke validates a Vault-stored Coinbase App API key with a
// authenticated Coinbase integration checker. Live execution is limited to an
// explicitly approved BTC-USDC buy of no more than 1 USDC.
package main

import (
	"context"
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
	resource := flag.String("resource", "permissions", "resource: permissions, accounts, order-status, journal-order-status, verify-journal, public-products, public-product, preview-order, approval-phrase, or live-order")
	productID := flag.String("product-id", "BTC-USD", "public product ID used with -resource=public-product")
	side := flag.String("side", "BUY", "order side used with -resource=preview-order: BUY or SELL")
	baseSize := flag.String("base-size", "", "base amount used with -resource=preview-order; set exactly one size")
	quoteSize := flag.String("quote-size", "", "quote amount used with -resource=preview-order; set exactly one size")
	previewID := flag.String("preview-id", "", "Coinbase preview UUID required for approval-phrase or live-order")
	approvalPhrase := flag.String("approval-phrase", "", "exact phrase required for -resource=live-order")
	confirmLiveOrder := flag.String("confirm-live-order", "", "must equal SUBMIT-1-USDC-BTC-USDC-BUY to submit a live order")
	orderID := flag.String("order-id", "", "Coinbase order UUID required for order-status or journal-order-status")
	journalPath := flag.String("journal-path", "", "local JSONL journal path required for journal-order-status or verify-journal")
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
	if *resource == "live-order" && *confirmLiveOrder != "SUBMIT-1-USDC-BTC-USDC-BUY" {
		log.Fatal("live order refused: pass -confirm-live-order=SUBMIT-1-USDC-BTC-USDC-BUY after reviewing a fresh preview")
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
