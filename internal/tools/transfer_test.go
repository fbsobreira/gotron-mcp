package tools

import (
	"testing"
)

func TestTransferTRX_InvalidFromAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "invalid",
		"to":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount": "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestTransferTRX_InvalidToAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "invalid",
		"amount": "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid to address")
	}
}

func TestTransferTRX_InvalidAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount": "abc",
	})
	if !result.IsError {
		t.Error("expected error for invalid amount")
	}
}

func TestTransferTRX_ZeroAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount": "0",
	})
	if !result.IsError {
		t.Error("expected error for zero amount")
	}
}

func TestTransferTRC20_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRC20(pool), map[string]any{
		"from":             "invalid",
		"to":               "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestTransferTRC20_InvalidContractAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRC20(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "invalid",
		"amount":           "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid contract address")
	}
}
