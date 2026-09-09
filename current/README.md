# Current Coinbase Integration Starter

This directory is intentionally separate from the archived WaaS client code in
the repository root. It contains a read-only Advanced Trade credential smoke
test and a small HashiCorp Vault KV v2 reader.

## Credential layout

Use separate Vault paths and policies. Do not reuse a trading key for wallet
operations.

| Vault path | Required fields | Intended access |
| --- | --- | --- |
| `coinbase/advanced-trade/live` | `key_id`, `key_secret` | Read account and permission data; trading requires a separate, narrowly scoped deployment policy. |
| `coinbase/server-wallet/live` | `key_id`, `key_secret`, `wallet_secret` | Backend-only server-wallet operations. |
| `coinbase/embedded-wallet/live` | Server-side configuration only | Backend support for embedded wallets. Never expose a secret through a browser or mobile app. |

The sample reads only the Advanced Trade path and can call either
`GET /api/v3/brokerage/key_permissions` or `GET /api/v3/brokerage/accounts`.
It cannot place, cancel, or transfer an order.

Example least-privilege Vault policies are in `vault-policies/`. Apply only
`advanced-trade-read.hcl` to the process that runs the smoke test; the
server-wallet and embedded-wallet policies belong to separate workloads.

## Run the read-only smoke test

Set the Vault address and a token that can read only the Advanced Trade secret:

```bash
export VAULT_ADDR="https://vault.example.internal"
export VAULT_TOKEN="..."
go run ./current/cmd/advanced-trade-smoke \
  -vault-mount=secret \
  -vault-path=coinbase/advanced-trade/live
```

The Vault secret must be a KV v2 secret and contain an ECDSA Coinbase App API
key in these fields:

```text
key_id     = organizations/<organization-id>/apiKeys/<key-id>
key_secret = -----BEGIN EC PRIVATE KEY----- ...
```

Use a key restricted to the intended Coinbase portfolio and server IP address.
For this read-only probe, grant only `view`. The program prints the API's
permission response but never prints a secret or JWT.

To list accessible accounts with the same view-only key, add
`-resource=accounts` to the command.

Public product metadata can be read without credentials with
`-resource=public-products`.

For one market instead of the full catalogue, use
`-resource=public-product -product-id=BTC-USD`.

## Next stages

1. Add an approval-gated order service using a different `trade` key.
2. Add CDP Server Wallet support using the separate `wallet_secret`.
3. Add the Embedded Wallet OAuth/custom-auth service without exposing backend
   secrets to clients.
