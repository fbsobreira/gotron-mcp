package tools

import (
	"testing"
)

func TestValidateAddress_Base58(t *testing.T) {
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}

func TestValidateAddress_Hex(t *testing.T) {
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "41B4A428AB7092C2F1395F376CE297033B3BB446B4",
	})
	if result.IsError {
		t.Fatalf("expected success for hex address, got error: %v", result.Content)
	}
}

func TestValidateAddress_Invalid(t *testing.T) {
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "notanaddress",
	})
	// Should return success with is_valid: false, not a tool error
	if result.IsError {
		t.Fatalf("expected success (with is_valid false), got tool error: %v", result.Content)
	}
}

func TestValidateAddress_Empty(t *testing.T) {
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "",
	})
	if !result.IsError {
		t.Error("expected error for empty address")
	}
}
