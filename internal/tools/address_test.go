package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAddress_Base58(t *testing.T) {
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
}

func TestValidateAddress_Hex(t *testing.T) {
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "41B4A428AB7092C2F1395F376CE297033B3BB446B4",
	})
	require.False(t, result.IsError, "expected success for hex address, got error: %v", result.Content)
}

func TestValidateAddress_Invalid(t *testing.T) {
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "notanaddress",
	})
	// Should return success with is_valid: false, not a tool error
	require.False(t, result.IsError, "expected success (with is_valid false), got tool error: %v", result.Content)
}

func TestValidateAddress_Empty(t *testing.T) {
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "",
	})
	assert.True(t, result.IsError, "expected error for empty address")
}

func TestValidateAddress_Ethereum0x(t *testing.T) {
	// Known: TJRabPrwbZy45sbavfcjinPJC18kjpRTv8 = 0x5cbdd86a2fa8dc4bddd8a8f69dba48572eec07fb (20-byte ETH)
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0x5cbdd86a2fa8dc4bddd8a8f69dba48572eec07fb",
	})
	require.False(t, result.IsError, "expected success for ETH address, got error: %v", result.Content)
	data := extractJSON(t, result)
	assert.Equal(t, "ethereum", data["input_format"], "input_format")
	assert.Equal(t, true, data["is_valid"], "is_valid")
	assert.Equal(t, "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8", data["base58"], "base58")
}

func TestValidateAddress_Ethereum0x_Invalid(t *testing.T) {
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0xZZZZ",
	})
	require.False(t, result.IsError, "expected success (with is_valid false), got tool error")
	data := extractJSON(t, result)
	assert.Equal(t, false, data["is_valid"], "is_valid")
}

func TestValidateAddress_Ethereum0x_WrongLength(t *testing.T) {
	// 19 bytes — too short for Ethereum address, treated as hex
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0x5cbdd86a2fa8dc4bddd8a8f69dba48572eec07",
	})
	require.False(t, result.IsError, "expected success (with is_valid false), got tool error")
	data := extractJSON(t, result)
	assert.Equal(t, false, data["is_valid"], "is_valid for wrong-length ETH addr")
}

func TestValidateAddress_Ethereum0x41(t *testing.T) {
	// A valid 20-byte Ethereum address that starts with 0x41
	// Should be detected as Ethereum (20 bytes), not TRON hex
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0x4100000000000000000000000000000000000001",
	})
	require.False(t, result.IsError, "expected success for 0x41... ETH address, got tool error")
	data := extractJSON(t, result)
	assert.Equal(t, "ethereum", data["input_format"], "input_format (20-byte 0x41... should be ETH)")
	assert.Equal(t, true, data["is_valid"], "is_valid")
}

func TestValidateAddress_0x41_TronHex21Bytes(t *testing.T) {
	// 21-byte 0x-prefixed TRON hex address (starts with 41)
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0x41B4A428AB7092C2F1395F376CE297033B3BB446B4",
	})
	require.False(t, result.IsError, "expected success for 0x41... TRON hex address")
	data := extractJSON(t, result)
	assert.Equal(t, "hex", data["input_format"], "input_format (21-byte 0x41... should be TRON hex)")
	assert.Equal(t, true, data["is_valid"], "is_valid")
}

func TestValidateAddress_0X_Uppercase(t *testing.T) {
	// Uppercase 0X prefix should work the same as 0x
	result := callTool(t, handleValidateAddress(), map[string]any{
		"address": "0X5cbdd86a2fa8dc4bddd8a8f69dba48572eec07fb",
	})
	require.False(t, result.IsError, "expected success for 0X-prefixed ETH address")
	data := extractJSON(t, result)
	assert.Equal(t, "ethereum", data["input_format"], "input_format")
}
