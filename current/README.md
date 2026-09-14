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

## Preview a prospective market order

Order previews are the only order-related operation currently implemented.
They use `POST /api/v3/brokerage/orders/preview`, which Coinbase documents as a
`view`-permission endpoint. A preview can report validation errors, expected
fees, and a `preview_id`; it cannot submit an order.

Use exactly one of `-quote-size` or `-base-size`:

```bash
go run ./current/cmd/advanced-trade-smoke \
  -source=env \
  -resource=preview-order \
  -product-id=BTC-USD \
  -side=BUY \
  -quote-size=10.00
```

## Narrow live-order path

The only live action implemented is a market **BUY** of `BTC-USDC` using a
quote size of at most **1 USDC**. It cannot sell, transfer funds, cancel
orders, use another market, or spend more than 1 USDC. A live request binds to
the exact `preview_id`; that UUID is also used as Coinbase's `client_order_id`,
so retrying the same preview is idempotent.

The operator must first obtain a fresh preview, then generate its exact
approval phrase. The final command requires both that phrase and a literal
confirmation flag. Do not run the final command unless you intentionally want
to submit that exact order:

```bash
# Produce a new preview and retain its preview_id from the JSON response.
go run ./current/cmd/advanced-trade-smoke -source=env \
  -resource=preview-order -product-id=BTC-USDC -side=BUY -quote-size=1.00

# Generate the phrase for that exact preview ID; type or paste it into the
# final command yourself. It is not a credential.
go run ./current/cmd/advanced-trade-smoke -resource=approval-phrase \
  -product-id=BTC-USDC -side=BUY -quote-size=1.00 \
  -preview-id=<preview-id>

# This is the only command that can place an order.
go run ./current/cmd/advanced-trade-smoke -source=env -resource=live-order \
  -product-id=BTC-USDC -side=BUY -quote-size=1.00 \
  -preview-id=<preview-id> -approval-phrase=<approval-phrase> \
  -confirm-live-order=SUBMIT-1-USDC-BTC-USDC-BUY
```

## Reconcile and journal a completed order

`order-status` is a read-only Coinbase lookup and prints only a sanitized
execution summary; it excludes account, user, and portfolio identifiers.
`journal-order-status` first performs that same read-only lookup, then appends
selected trade metadata to a local JSONL journal with 0600 permissions. Each
journal entry includes the previous entry hash, so `verify-journal` detects
alteration or removal. It is tamper-evident rather than physically immutable;
keep its directory restricted to the trading service account.

```bash
# Read Coinbase's final order record. This makes no account changes.
go run ./current/cmd/advanced-trade-smoke -source=env \
  -resource=order-status -order-id=<order-id>

# Record only selected final order metadata in a local hash-chained journal.
go run ./current/cmd/advanced-trade-smoke -source=env \
  -resource=journal-order-status -order-id=<order-id> \
  -journal-path=/var/lib/coinbase-trading/trades.jsonl

# Verify every journal-chain link later.
go run ./current/cmd/advanced-trade-smoke \
  -resource=verify-journal \
  -journal-path=/var/lib/coinbase-trading/trades.jsonl
```

## Paper trading and live risk policy

The current policy permits at most one `BTC-USDC` BUY, up to 1 USDC, per
`Africa/Johannesburg` calendar day. The same journal used for reconciliation is
the source of truth. Therefore `-journal-path` is mandatory for both
`paper-buy` and `live-order`. A file at
`/var/lib/coinbase-trading/DISABLED` is a manual kill switch: while it exists,
the live path refuses every order. Paper simulation never uses credentials or
submits an order; it uses public price data and deliberately excludes a guessed
fee estimate.

```bash
# Simulate a 1-USDC buy and report whether today's live policy permits it.
go run ./current/cmd/advanced-trade-smoke \
  -resource=paper-buy -quote-size=1.00 \
  -journal-path=/var/lib/coinbase-trading/trades.jsonl

# Disable all future live orders immediately (does not affect paper trading).
sudo install -d -m 700 /var/lib/coinbase-trading
sudo touch /var/lib/coinbase-trading/DISABLED

# Re-enable only after manual review.
sudo rm /var/lib/coinbase-trading/DISABLED
```

## Next stages

1. Add paper-trading strategies and a daily budget policy before widening the
   deliberately narrow 1-USDC live-order path.
2. Add CDP Server Wallet support using the separate `wallet_secret`.
3. Add the Embedded Wallet OAuth/custom-auth service without exposing backend
   secrets to clients.
