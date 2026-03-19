package tools

import (
	"context"
	"math/big"
	"testing"

	"fmt"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
)

// abiEncodeUint256 returns a 32-byte ABI-encoded big.Int.
func abiEncodeUint256(v *big.Int) []byte {
	b := v.Bytes()
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded
}

// abiEncodeString returns ABI-encoded string (offset + length + data).
func abiEncodeString(s string) []byte {
	// offset (32 bytes) pointing to 0x20
	out := make([]byte, 32)
	out[31] = 0x20
	// length
	lenBytes := make([]byte, 32)
	lenBytes[31] = byte(len(s))
	out = append(out, lenBytes...)
	// data padded to 32 bytes
	data := make([]byte, 32)
	copy(data, []byte(s))
	out = append(out, data...)
	return out
}

// mockTRC20Server creates a mock that responds to TriggerConstantContract
// with ABI-encoded results for balance, decimals, name, and symbol queries.
func mockTRC20Server(balance *big.Int, decimals int64, name, symbol string) *mockWalletServer {
	return &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			// Detect which method is being called by data prefix (function selector)
			data := in.Data
			if len(data) < 4 {
				return &api.TransactionExtention{
					ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(0))},
					Result:         &api.Return{Result: true},
				}, nil
			}

			// Function selectors:
			// balanceOf(address)     = 0x70a08231
			// decimals()             = 0x313ce567
			// name()                 = 0x06fdde03
			// symbol()               = 0x95d89b41
			selector := data[:4]
			var result []byte
			switch {
			case selector[0] == 0x70 && selector[1] == 0xa0: // balanceOf
				result = abiEncodeUint256(balance)
			case selector[0] == 0x31 && selector[1] == 0x3c: // decimals
				result = abiEncodeUint256(big.NewInt(decimals))
			case selector[0] == 0x06 && selector[1] == 0xfd: // name
				result = abiEncodeString(name)
			case selector[0] == 0x95 && selector[1] == 0xd8: // symbol
				result = abiEncodeString(symbol)
			default:
				result = abiEncodeUint256(big.NewInt(0))
			}

			return &api.TransactionExtention{
				ConstantResult: [][]byte{result},
				Result:         &api.Return{Result: true},
			}, nil
		},
	}
}

func TestGetTRC20Balance_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTRC20Balance(pool), map[string]any{
		"address":          "",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	if !result.IsError {
		t.Error("expected error for empty address")
	}
}

func TestGetTRC20Balance_InvalidContract(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTRC20Balance(pool), map[string]any{
		"address":          "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "invalid",
	})
	if !result.IsError {
		t.Error("expected error for invalid contract address")
	}
}

func TestGetTRC20Balance_Success(t *testing.T) {
	balance := big.NewInt(1_000_000) // 1.0 USDT (6 decimals)
	mock := mockTRC20Server(balance, 6, "Tether USD", "USDT")
	pool := newMockPool(t, mock)

	result := callTool(t, handleGetTRC20Balance(pool), map[string]any{
		"address":          "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["decimals"] != float64(6) {
		t.Errorf("decimals = %v, want 6", data["decimals"])
	}
	if data["balance_raw"] != "1000000" {
		t.Errorf("balance_raw = %v, want 1000000", data["balance_raw"])
	}
}

func TestGetTRC20TokenInfo_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTRC20TokenInfo(pool), map[string]any{
		"contract_address": "bad",
	})
	if !result.IsError {
		t.Error("expected error for invalid contract address")
	}
}

func TestGetTRC20TokenInfo_Success(t *testing.T) {
	mock := mockTRC20Server(big.NewInt(0), 18, "TestToken", "TTK")
	pool := newMockPool(t, mock)

	result := callTool(t, handleGetTRC20TokenInfo(pool), map[string]any{
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["name"] != "TestToken" {
		t.Errorf("name = %v, want TestToken", data["name"])
	}
	if data["symbol"] != "TTK" {
		t.Errorf("symbol = %v, want TTK", data["symbol"])
	}
	if data["decimals"] != float64(18) {
		t.Errorf("decimals = %v, want 18", data["decimals"])
	}
}

func TestGetTRC20TokenInfo_NameError(t *testing.T) {
	// Mock that returns error for TriggerConstantContract (simulates name() failure)
	mock := &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return nil, fmt.Errorf("mock: contract call failed")
		},
	}
	pool := newMockPool(t, mock)

	result := callTool(t, handleGetTRC20TokenInfo(pool), map[string]any{
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	if !result.IsError {
		t.Error("expected error when name() call fails")
	}
	// Verify it's a tool error (not a Go error)
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			if tc.Text == "" {
				t.Error("error message should not be empty")
			}
		}
	}
}
