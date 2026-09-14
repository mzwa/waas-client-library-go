package coinbase

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TradeJournalEntry is a tamper-evident, append-only local trade record. Hash
// chaining makes later alteration or removal detectable when VerifyTradeJournal
// is run. It intentionally omits credentials, tokens, addresses, and balances.
type TradeJournalEntry struct {
	RecordedAt           string `json:"recorded_at"`
	OrderID              string `json:"order_id"`
	PreviewID            string `json:"preview_id"`
	ProductID            string `json:"product_id"`
	Side                 string `json:"side"`
	Status               string `json:"status"`
	CompletionPercentage string `json:"completion_percentage"`
	FilledSize           string `json:"filled_size"`
	AverageFilledPrice   string `json:"average_filled_price"`
	TotalFees            string `json:"total_fees"`
	PreviousHash         string `json:"previous_hash"`
	Hash                 string `json:"hash"`
}

type coinbaseOrderResponse struct {
	Order struct {
		OrderID              string `json:"order_id"`
		ClientOrderID        string `json:"client_order_id"`
		ProductID            string `json:"product_id"`
		Side                 string `json:"side"`
		Status               string `json:"status"`
		CompletionPercentage string `json:"completion_percentage"`
		FilledSize           string `json:"filled_size"`
		AverageFilledPrice   string `json:"average_filled_price"`
		TotalFees            string `json:"total_fees"`
		FilledValue          string `json:"filled_value"`
		TotalValueAfterFees  string `json:"total_value_after_fees"`
		CreatedTime          string `json:"created_time"`
		LastFillTime         string `json:"last_fill_time"`
		Settled              bool   `json:"settled"`
	} `json:"order"`
}

// OrderSummary is the safe terminal representation of a completed Coinbase
// order. It intentionally excludes user, portfolio, and account identifiers.
type OrderSummary struct {
	OrderID              string `json:"order_id"`
	PreviewID            string `json:"preview_id"`
	ProductID            string `json:"product_id"`
	Side                 string `json:"side"`
	Status               string `json:"status"`
	CompletionPercentage string `json:"completion_percentage"`
	FilledSize           string `json:"filled_size"`
	AverageFilledPrice   string `json:"average_filled_price"`
	FilledValue          string `json:"filled_value"`
	TotalFees            string `json:"total_fees"`
	TotalValueAfterFees  string `json:"total_value_after_fees"`
	CreatedTime          string `json:"created_time"`
	LastFillTime         string `json:"last_fill_time"`
	Settled              bool   `json:"settled"`
}

// SummarizeOrderStatus reduces Coinbase's response to non-sensitive execution
// fields suitable for terminal output and application logs.
func SummarizeOrderStatus(orderStatus []byte) (OrderSummary, error) {
	response, err := decodeOrderStatus(orderStatus)
	if err != nil {
		return OrderSummary{}, err
	}
	return OrderSummary{
		OrderID:              response.Order.OrderID,
		PreviewID:            response.Order.ClientOrderID,
		ProductID:            response.Order.ProductID,
		Side:                 response.Order.Side,
		Status:               response.Order.Status,
		CompletionPercentage: response.Order.CompletionPercentage,
		FilledSize:           response.Order.FilledSize,
		AverageFilledPrice:   response.Order.AverageFilledPrice,
		FilledValue:          response.Order.FilledValue,
		TotalFees:            response.Order.TotalFees,
		TotalValueAfterFees:  response.Order.TotalValueAfterFees,
		CreatedTime:          response.Order.CreatedTime,
		LastFillTime:         response.Order.LastFillTime,
		Settled:              response.Order.Settled,
	}, nil
}

// AppendOrderStatusToJournal extracts a minimal record from a read-only
// Coinbase Get Order response and appends it to a 0600 JSONL journal.
func AppendOrderStatusToJournal(path string, orderStatus []byte, recordedAt time.Time) (TradeJournalEntry, error) {
	response, err := decodeOrderStatus(orderStatus)
	if err != nil {
		return TradeJournalEntry{}, err
	}
	previousHash, _, err := VerifyTradeJournal(path)
	if err != nil {
		return TradeJournalEntry{}, err
	}
	entry := TradeJournalEntry{
		RecordedAt:           recordedAt.UTC().Format(time.RFC3339Nano),
		OrderID:              response.Order.OrderID,
		PreviewID:            response.Order.ClientOrderID,
		ProductID:            response.Order.ProductID,
		Side:                 response.Order.Side,
		Status:               response.Order.Status,
		CompletionPercentage: response.Order.CompletionPercentage,
		FilledSize:           response.Order.FilledSize,
		AverageFilledPrice:   response.Order.AverageFilledPrice,
		TotalFees:            response.Order.TotalFees,
		PreviousHash:         previousHash,
	}
	entry.Hash, err = entry.expectedHash()
	if err != nil {
		return TradeJournalEntry{}, err
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return TradeJournalEntry{}, fmt.Errorf("encode trade journal entry: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return TradeJournalEntry{}, fmt.Errorf("create trade journal directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return TradeJournalEntry{}, fmt.Errorf("open trade journal: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return TradeJournalEntry{}, fmt.Errorf("append trade journal: %w", err)
	}
	return entry, nil
}

func decodeOrderStatus(orderStatus []byte) (coinbaseOrderResponse, error) {
	var response coinbaseOrderResponse
	if err := json.Unmarshal(orderStatus, &response); err != nil {
		return coinbaseOrderResponse{}, fmt.Errorf("decode Coinbase order status: %w", err)
	}
	if !isUUID(response.Order.OrderID) {
		return coinbaseOrderResponse{}, fmt.Errorf("Coinbase order status did not contain a UUID order ID")
	}
	if response.Order.ProductID == "" || response.Order.Side == "" || response.Order.Status == "" {
		return coinbaseOrderResponse{}, fmt.Errorf("Coinbase order status is missing required trade metadata")
	}
	return response, nil
}

// VerifyTradeJournal validates every hash-chain link and returns the last hash
// plus the number of entries. A missing journal is valid and has no entries.
func VerifyTradeJournal(path string) (lastHash string, entries int, err error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("open trade journal: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry TradeJournalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return "", entries, fmt.Errorf("decode trade journal entry %d: %w", entries+1, err)
		}
		if entry.PreviousHash != lastHash {
			return "", entries, fmt.Errorf("trade journal chain mismatch at entry %d", entries+1)
		}
		expected, err := entry.expectedHash()
		if err != nil {
			return "", entries, err
		}
		if entry.Hash != expected {
			return "", entries, fmt.Errorf("trade journal hash mismatch at entry %d", entries+1)
		}
		lastHash = entry.Hash
		entries++
	}
	if err := scanner.Err(); err != nil {
		return "", entries, fmt.Errorf("read trade journal: %w", err)
	}
	return lastHash, entries, nil
}

func (entry TradeJournalEntry) expectedHash() (string, error) {
	entry.Hash = ""
	canonical, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("canonicalize trade journal entry: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}
