package tools

import (
	"testing"
)

func TestFreezeBalance_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleFreezeBalance(pool), map[string]any{
		"from":     "invalid",
		"amount":   "100",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for invalid address")
	}
}

func TestFreezeBalance_InvalidAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleFreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "abc",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for invalid amount")
	}
}

func TestFreezeBalance_ZeroAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleFreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "0",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for zero amount")
	}
}

func TestUnfreezeBalance_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleUnfreezeBalance(pool), map[string]any{
		"from":     "",
		"amount":   "100",
		"resource": "BANDWIDTH",
	})
	if !result.IsError {
		t.Error("expected error for empty address")
	}
}

func TestUnfreezeBalance_ZeroAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleUnfreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "0",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for zero amount")
	}
}
