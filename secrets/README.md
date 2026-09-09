# Encrypted Coinbase secrets

This directory contains only SOPS-encrypted secret files. Never commit a
plaintext `.env`, JSON key download, PEM file, or age private identity here.

## One-time server setup

Run these commands on the deployment server. Keep the generated identity file
owned by root and do not copy it into the repository.

```bash
sudo install -d -m 700 /etc/sops/age
sudo age-keygen -o /etc/sops/age/keys.txt
sudo chmod 600 /etc/sops/age/keys.txt
sudo age-keygen -y /etc/sops/age/keys.txt
```

The final command prints an `age1...` **public recipient**. It is safe to share
with the developer who encrypts the secrets file.

## Encrypt the Coinbase key

1. `.sops.yaml` is already configured for the deployment server's public age
   recipient. Do not modify it to add a private identity.
2. Create a temporary local file called `advanced-trade.env` containing:

   ```text
   COINBASE_KEY_ID=organizations/.../apiKeys/...
   COINBASE_KEY_SECRET="-----BEGIN EC PRIVATE KEY-----\n...\n-----END EC PRIVATE KEY-----\n"
   ```

3. Encrypt it, review that `git diff` contains ciphertext only, then securely
   remove the plaintext file:

   ```bash
   sops --encrypt --input-type dotenv --output-type dotenv \
     advanced-trade.env > secrets/coinbase/advanced-trade.enc.env
   shred -u advanced-trade.env
   ```

4. Commit only `secrets/coinbase/advanced-trade.enc.env` and `.sops.yaml`.

## Run the read-only check on the server

```bash
sudo env SOPS_AGE_KEY_FILE=/etc/sops/age/keys.txt \
sops exec-env secrets/coinbase/advanced-trade.enc.env \
  'go run ./current/cmd/advanced-trade-smoke -source=env'
```

The command calls Coinbase's read-only key-permissions endpoint; it does not
place orders or move funds.
