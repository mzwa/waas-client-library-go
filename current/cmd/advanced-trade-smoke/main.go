// advanced-trade-smoke validates a Vault-stored Coinbase App API key with a
// read-only permission lookup. It does not create orders or move funds.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/coinbase/waas-client-library-go/current/coinbase"
)

func main() {
	source := flag.String("source", "vault", "credential source: vault or env")
	mount := flag.String("vault-mount", "secret", "Vault KV v2 mount")
	path := flag.String("vault-path", "coinbase/advanced-trade/live", "Vault secret path")
	resource := flag.String("resource", "permissions", "resource: permissions, accounts, public-products, public-product, or preview-order")
	productID := flag.String("product-id", "BTC-USD", "public product ID used with -resource=public-product")
	side := flag.String("side", "BUY", "order side used with -resource=preview-order: BUY or SELL")
	baseSize := flag.String("base-size", "", "base amount used with -resource=preview-order; set exactly one size")
	quoteSize := flag.String("quote-size", "", "quote amount used with -resource=preview-order; set exactly one size")
	flag.Parse()
	if *resource == "public-products" {
		response, err := coinbase.ListPublicProducts(context.Background(), nil)
		if err != nil {
			log.Fatalf("read Coinbase public products: %v", err)
		}
		fmt.Println(string(response))
		return
	}
	if *resource == "public-product" {
		response, err := coinbase.GetPublicProduct(context.Background(), nil, *productID)
		if err != nil {
			log.Fatalf("read Coinbase public product: %v", err)
		}
		fmt.Println(string(response))
		return
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
		response, err = coinbase.CheckAdvancedTradePermissions(context.Background(), nil, credentials)
	case "accounts":
		response, err = coinbase.ListAdvancedTradeAccounts(context.Background(), nil, credentials)
	case "preview-order":
		response, err = coinbase.PreviewOrder(context.Background(), nil, credentials, coinbase.MarketOrderPreview{
			ProductID: *productID,
			Side:      *side,
			BaseSize:  *baseSize,
			QuoteSize: *quoteSize,
		})
	default:
		log.Fatalf("unsupported resource %q", *resource)
	}
	if err != nil {
		log.Fatalf("read Coinbase Advanced Trade data: %v", err)
	}
	fmt.Println(string(response))
}
