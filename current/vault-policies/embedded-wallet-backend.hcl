# Replace "secret" if this KV v2 mount has a different name.
# This policy is for an embedded-wallet backend only, never a client app.
path "secret/data/coinbase/embedded-wallet/live" {
  capabilities = ["read"]
}
