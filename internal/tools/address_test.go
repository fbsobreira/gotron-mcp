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

func TestValidateAddress_Ethereum0x(t *testing.T) {
	// Known: TJRabPrwbZy45sbavfcjinPJC18kjpRTv8 = 0x5cbdd86a2fa8dc4bddd8a8f69dba48572eec07fb (20-byte ETH)
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0x5cbdd86a2fa8dc4bddd8a8f69dba48572eec07fb",
	})
	if result.IsError {
		t.Fatalf("expected success for ETH address, got error: %v", result.Content)
	}
	data := extractJSON(t, result)
	if data["input_format"] != "ethereum" {
		t.Errorf("input_format = %v, want ethereum", data["input_format"])
	}
	if data["is_valid"] != true {
		t.Errorf("is_valid = %v, want true", data["is_valid"])
	}
	if data["base58"] != "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8" {
		t.Errorf("base58 = %v, want TJRabPrwbZy45sbavfcjinPJC18kjpRTv8", data["base58"])
	}
}

func TestValidateAddress_Ethereum0x_Invalid(t *testing.T) {
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0xZZZZ",
	})
	if result.IsError {
		t.Fatal("expected success (with is_valid false), got tool error")
	}
	data := extractJSON(t, result)
	if data["is_valid"] != false {
		t.Errorf("is_valid = %v, want false", data["is_valid"])
	}
}

func TestValidateAddress_Ethereum0x_WrongLength(t *testing.T) {
	// 19 bytes — too short for Ethereum address, treated as hex
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0x5cbdd86a2fa8dc4bddd8a8f69dba48572eec07",
	})
	if result.IsError {
		t.Fatal("expected success (with is_valid false), got tool error")
	}
	data := extractJSON(t, result)
	if data["is_valid"] != false {
		t.Errorf("is_valid = %v, want false for wrong-length ETH addr", data["is_valid"])
	}
}

func TestValidateAddress_Ethereum0x41(t *testing.T) {
	// A valid 20-byte Ethereum address that starts with 0x41
	// Should be detected as Ethereum (20 bytes), not TRON hex
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0x4100000000000000000000000000000000000001",
	})
	if result.IsError {
		t.Fatal("expected success for 0x41... ETH address, got tool error")
	}
	data := extractJSON(t, result)
	if data["input_format"] != "ethereum" {
		t.Errorf("input_format = %v, want ethereum (20-byte 0x41... should be ETH)", data["input_format"])
	}
	if data["is_valid"] != true {
		t.Errorf("is_valid = %v, want true", data["is_valid"])
	}
}

func TestValidateAddress_0x41_TronHex21Bytes(t *testing.T) {
	// 21-byte 0x-prefixed TRON hex address (starts with 41)
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0x41B4A428AB7092C2F1395F376CE297033B3BB446B4",
	})
	if result.IsError {
		t.Fatal("expected success for 0x41... TRON hex address")
	}
	data := extractJSON(t, result)
	if data["input_format"] != "hex" {
		t.Errorf("input_format = %v, want hex (21-byte 0x41... should be TRON hex)", data["input_format"])
	}
	if data["is_valid"] != true {
		t.Errorf("is_valid = %v, want true", data["is_valid"])
	}
}

func TestValidateAddress_0X_Uppercase(t *testing.T) {
	// Uppercase 0X prefix should work the same as 0x
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0X5cbdd86a2fa8dc4bddd8a8f69dba48572eec07fb",
	})
	if result.IsError {
		t.Fatal("expected success for 0X-prefixed ETH address")
	}
	data := extractJSON(t, result)
	if data["input_format"] != "ethereum" {
		t.Errorf("input_format = %v, want ethereum", data["input_format"])
	}
}
