package coinbase

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// ApprovalPhrase returns the exact, non-secret phrase an operator must supply
// before a future live-order implementation may submit this intent. It binds an
// approval to the product, side, and requested size; it must not be reused for
// a different intent.
func (p MarketOrderPreview) ApprovalPhrase() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	canonical := strings.Join([]string{
		strings.TrimSpace(p.ProductID),
		p.Side,
		strings.TrimSpace(p.BaseSize),
		strings.TrimSpace(p.QuoteSize),
	}, "\n")
	digest := sha256.Sum256([]byte(canonical))
	return "APPROVE-LIVE-ORDER:" + hex.EncodeToString(digest[:]), nil
}

// RequireLiveOrderApproval is the mandatory guard for any future live-order
// submission code. This repository intentionally does not yet implement order
// submission; only PreviewOrder can call Coinbase.
func RequireLiveOrderApproval(intent MarketOrderPreview, suppliedPhrase string) error {
	expectedPhrase, err := intent.ApprovalPhrase()
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(suppliedPhrase)), []byte(expectedPhrase)) != 1 {
		return fmt.Errorf("live order refused: explicit approval phrase for this exact intent is required")
	}
	return nil
}
