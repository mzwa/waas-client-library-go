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
//
// Deprecated: live order code must use ApprovalPhraseForPreview instead, so an
// approval also refers to a specific Coinbase preview.
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

// ApprovalPhraseForPreview returns the exact, non-secret phrase required to
// approve an order represented by a particular Coinbase preview. The preview
// ID prevents a confirmation for one quote from being reused after price or
// fee conditions have changed.
func (p MarketOrderPreview) ApprovalPhraseForPreview(previewID string) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	previewID = strings.TrimSpace(previewID)
	if !isUUID(previewID) {
		return "", fmt.Errorf("a UUID Coinbase preview ID is required for live order approval")
	}
	canonical := strings.Join([]string{
		previewID,
		strings.TrimSpace(p.ProductID),
		p.Side,
		strings.TrimSpace(p.BaseSize),
		strings.TrimSpace(p.QuoteSize),
	}, "\n")
	digest := sha256.Sum256([]byte(canonical))
	return "APPROVE-LIVE-ORDER:" + hex.EncodeToString(digest[:]), nil
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') && !(character >= 'A' && character <= 'F') {
			return false
		}
	}
	return true
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

// RequireLiveOrderApprovalForPreview is the mandatory guard for future live
// order submission. Call it immediately before sending a create-order request,
// using the preview ID returned by Coinbase and an operator-supplied phrase.
func RequireLiveOrderApprovalForPreview(intent MarketOrderPreview, previewID, suppliedPhrase string) error {
	expectedPhrase, err := intent.ApprovalPhraseForPreview(previewID)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(suppliedPhrase)), []byte(expectedPhrase)) != 1 {
		return fmt.Errorf("live order refused: explicit approval phrase for this exact Coinbase preview is required")
	}
	return nil
}
