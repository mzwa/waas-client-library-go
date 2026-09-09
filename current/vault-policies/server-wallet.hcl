# Replace "secret" if this KV v2 mount has a different name.
# Attach this only to the isolated backend that manages server wallets.
path "secret/data/coinbase/server-wallet/live" {
  capabilities = ["read"]
}
