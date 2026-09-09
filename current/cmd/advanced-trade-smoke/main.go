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
	flag.Parse()

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

	permissions, err := coinbase.CheckAdvancedTradePermissions(context.Background(), nil, credentials)
	if err != nil {
		log.Fatalf("validate Coinbase API key: %v", err)
	}
	fmt.Println(string(permissions))
}
