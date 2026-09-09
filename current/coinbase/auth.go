// Package coinbase implements the minimal, server-side authentication needed
// for the current Coinbase App REST API.
package coinbase

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
)

// Credentials are the ECDSA Coinbase App API credentials. Keep them only in
// process memory and obtain them from a secret manager at runtime.
type Credentials struct {
	KeyID     string
	KeySecret string
}

// CredentialsFromEnv loads Coinbase credentials injected into the process
// environment by a secret manager such as SOPS or systemd credentials.
func CredentialsFromEnv() (Credentials, error) {
	keySecret := strings.TrimSpace(os.Getenv("COINBASE_KEY_SECRET"))
	keySecret = strings.Trim(keySecret, `"'`)
	keySecret = strings.ReplaceAll(keySecret, `\n`, "\n")
	credentials := Credentials{
		KeyID:     os.Getenv("COINBASE_KEY_ID"),
		KeySecret: keySecret,
	}
	if err := credentials.Validate(); err != nil {
		return Credentials{}, err
	}
	return credentials, nil
}

// Validate reports whether the required credential fields were supplied.
func (c Credentials) Validate() error {
	if strings.TrimSpace(c.KeyID) == "" {
		return fmt.Errorf("coinbase key_id is required")
	}
	if strings.TrimSpace(c.KeySecret) == "" {
		return fmt.Errorf("coinbase key_secret is required")
	}
	return nil
}

// BuildRESTJWT creates a request-bound JWT for Coinbase App REST APIs.
// The requestHost must not include a scheme and requestPath must start with /.
func BuildRESTJWT(credentials Credentials, method, requestHost, requestPath string, now time.Time) (string, error) {
	if err := credentials.Validate(); err != nil {
		return "", err
	}
	if method == "" || requestHost == "" || !strings.HasPrefix(requestPath, "/") {
		return "", fmt.Errorf("method, host, and absolute request path are required")
	}

	privateKey, err := parseECPrivateKey(credentials.KeySecret)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate JWT nonce: %w", err)
	}

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.ES256, Key: privateKey},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", credentials.KeyID).WithHeader("nonce", hex.EncodeToString(nonce)),
	)
	if err != nil {
		return "", fmt.Errorf("create JWT signer: %w", err)
	}

	claims := struct {
		Issuer  string `json:"iss"`
		Subject string `json:"sub"`
		URI     string `json:"uri"`
		*jwt.Claims
	}{
		Issuer:  "coinbase-cloud",
		Subject: credentials.KeyID,
		URI:     fmt.Sprintf("%s %s%s", strings.ToUpper(method), requestHost, requestPath),
		Claims: &jwt.Claims{
			NotBefore: jwt.NewNumericDate(now),
			Expiry:    jwt.NewNumericDate(now.Add(2 * time.Minute)),
		},
	}

	token, err := jwt.Signed(signer).Claims(claims).CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("serialize JWT: %w", err)
	}
	return token, nil
}

func parseECPrivateKey(encoded string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(encoded))
	if block == nil {
		return nil, fmt.Errorf("decode Coinbase ECDSA private key: expected PEM")
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse Coinbase ECDSA private key: %w", err)
	}
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("parse Coinbase ECDSA private key: expected EC key")
	}
	return ecKey, nil
}
