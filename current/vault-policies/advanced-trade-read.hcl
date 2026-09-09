# Replace "secret" if this KV v2 mount has a different name.
# Attach this policy only to the read-only credential-validation workload.
path "secret/data/coinbase/advanced-trade/live" {
  capabilities = ["read"]
}
