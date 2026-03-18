package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestStringifyDecoded(t *testing.T) {
	tests := []struct {
		name string
		in   []interface{}
		want int
	}{
		{"nil slice", nil, 0},
		{"empty", []interface{}{}, 0},
		{"string", []interface{}{"hello"}, 1},
		{"int", []interface{}{42}, 1},
		{"nested", []interface{}{[]interface{}{"a", "b"}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringifyDecoded(tt.in)
			if len(got) != tt.want {
				t.Errorf("len = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestStringifyValue(t *testing.T) {
	// Test with basic types that don't require address import
	got := stringifyValue("hello")
	if got != "hello" {
		t.Errorf("string: got %v, want hello", got)
	}

	got = stringifyValue(42)
	if got != 42 {
		t.Errorf("int: got %v, want 42", got)
	}

	got = stringifyValue(nil)
	if got != nil {
		t.Errorf("nil: got %v, want nil", got)
	}
}

func TestIsEstimateEnergyUnsupported(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"unsupported message", fmt.Errorf("does not support estimate energy"), true},
		{"other error", fmt.Errorf("connection refused"), false},
		{"contains substring", fmt.Errorf("this node does not support estimate energy RPC"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEstimateEnergyUnsupported(tt.err)
			if got != tt.want {
				t.Errorf("isEstimateEnergyUnsupported(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDecodeABIOutput_EmptyAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleDecodeABIOutput(pool), map[string]any{
		"contract_address": "",
		"method":           "balanceOf(address)",
		"data":             "0000",
	})
	if !result.IsError {
		t.Error("expected error for empty contract address")
	}
}

func TestDecodeABIOutput_InvalidHex(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleDecodeABIOutput(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "balanceOf(address)",
		"data":             "not-hex",
	})
	if !result.IsError {
		t.Error("expected error for invalid hex data")
	}
}

func TestDecodeABIOutput_EmptyData(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleDecodeABIOutput(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "balanceOf(address)",
		"data":             "",
	})
	if !result.IsError {
		t.Error("expected error for empty data")
	}
}

func TestDecodeABIOutput_WithOxPrefix(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	// Valid hex with 0x prefix — will fail on ABI decode (no mock) but shouldn't fail on hex parse
	result := callTool(t, handleDecodeABIOutput(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "balanceOf(address)",
		"data":             "0x0000000000000000000000000000000000000000000000000000000000000001",
	})
	// Should error on ABI fetch (mock doesn't implement GetContractABI), not on hex parsing
	if !result.IsError {
		t.Fatal("expected ABI fetch error from mock")
	}
	// Verify the error is NOT about invalid hex — hex parsing with 0x prefix succeeded
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			if strings.Contains(tc.Text, "invalid hex") {
				t.Error("0x prefix should be stripped — got invalid hex error")
			}
		}
	}
}

func TestDecodeABIOutput_RevertReason(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	// Error(string) selector: 08c379a0 + "SafeMath: subtraction overflow"
	result := callTool(t, handleDecodeABIOutput(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "transfer(address,uint256)",
		"data":             "08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000001e536166654d6174683a207375627472616374696f6e206f766572666c6f770000",
	})
	if result.IsError {
		t.Fatalf("expected success (revert decode), got error: %v", result.Content)
	}
}
