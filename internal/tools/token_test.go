package tools

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"testing"

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
// with ABI-encoded results for balance, decimals, name, symbol, and totalSupply queries.
func mockTRC20Server(balance *big.Int, decimals int64, name, symbol string) *mockWalletServer {
	return &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			// Detect which method is being called by data prefix (function selector)
			data := in.Data
			if len(data) < 4 {
				return nil, fmt.Errorf("unexpected short calldata: %x", data)
			}

			selector := data[:4]
			var result []byte
			switch {
			case bytes.Equal(selector, []byte{0x70, 0xa0, 0x82, 0x31}): // balanceOf(address)
				result = abiEncodeUint256(balance)
			case bytes.Equal(selector, []byte{0x31, 0x3c, 0xe5, 0x67}): // decimals()
				result = abiEncodeUint256(big.NewInt(decimals))
			case bytes.Equal(selector, []byte{0x06, 0xfd, 0xde, 0x03}): // name()
				result = abiEncodeString(name)
			case bytes.Equal(selector, []byte{0x95, 0xd8, 0x9b, 0x41}): // symbol()
				result = abiEncodeString(symbol)
			case bytes.Equal(selector, []byte{0x18, 0x16, 0x0d, 0xdd}): // totalSupply()
				// Use a value above 2^53 to catch JSON float64 precision loss
				supply, _ := new(big.Int).SetString("100000000000000000000000000", 10) // 100M tokens with 18 decimals
				result = abiEncodeUint256(supply)
			default:
				return nil, fmt.Errorf("unexpected selector: %x", selector)
			}

			return &api.TransactionExtention{
				ConstantResult: [][]byte{result},
				Result:         &api.Return{Result: true},
			}, nil
		},
	}
}

// mockTRC20EstimateServer creates a mock that handles decimals() via
// TriggerConstantContract and transfer energy estimation via EstimateEnergy.
func mockTRC20EstimateServer(decimals int64, energyRequired int64) *mockWalletServer {
	return &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			data := in.Data
			if len(data) < 4 {
				return nil, fmt.Errorf("unexpected short calldata: %x", data)
			}

			selector := data[:4]
			var result []byte
			switch {
			case bytes.Equal(selector, []byte{0x31, 0x3c, 0xe5, 0x67}): // decimals()
				result = abiEncodeUint256(big.NewInt(decimals))
			default:
				return nil, fmt.Errorf("unexpected selector: %x", selector)
			}

			return &api.TransactionExtention{
				ConstantResult: [][]byte{result},
				Result:         &api.Return{Result: true},
			}, nil
		},
		EstimateEnergyFunc: func(_ context.Context, in *core.TriggerSmartContract) (*api.EstimateEnergyMessage, error) {
			// Verify the transfer selector (0xa9059cbb) is in the calldata
			if len(in.Data) < 4 || !bytes.Equal(in.Data[:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) {
				return nil, fmt.Errorf("expected transfer selector a9059cbb, got %x", in.Data[:4])
			}
			return &api.EstimateEnergyMessage{
				Result:         &api.Return{Result: true},
				EnergyRequired: energyRequired,
			}, nil
		},
	}
}

func TestEstimateTRC20Energy_Success(t *testing.T) {
	mock := mockTRC20EstimateServer(6, 29000)
	pool := newMockPool(t, mock)
	result := callTool(t, handleEstimateTRC20Energy(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	data := parseJSONResult(t, result)
	if data["estimated_energy"] != float64(29000) {
		t.Errorf("estimated_energy = %v, want 29000", data["estimated_energy"])
	}
	if data["from"] != "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF" {
		t.Errorf("from = %v, want TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", data["from"])
	}
}

func TestEstimateTRC20Energy_InvalidTo(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleEstimateTRC20Energy(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "bad",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid to address")
	}
}

func TestEstimateTRC20Energy_InvalidContract(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleEstimateTRC20Energy(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "bad",
		"amount":           "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid contract address")
	}
}

func TestEstimateTRC20Energy_InvalidFrom(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleEstimateTRC20Energy(pool, nil), map[string]any{
		"from":             "bad",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestEstimateTRC20Energy_InvalidAmount(t *testing.T) {
	mock := mockTRC20EstimateServer(6, 29000)
	pool := newMockPool(t, mock)
	result := callTool(t, handleEstimateTRC20Energy(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "not-a-number",
	})
	if !result.IsError {
		t.Error("expected error for invalid amount")
	}
}

func TestEstimateTRC20Energy_ZeroAmount(t *testing.T) {
	mock := mockTRC20EstimateServer(6, 29000)
	pool := newMockPool(t, mock)
	result := callTool(t, handleEstimateTRC20Energy(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "0",
	})
	if !result.IsError {
		t.Error("expected error for zero amount")
	}
}

func TestGetTRC20Balance_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTRC20Balance(pool, nil), map[string]any{
		"address":          "",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	if !result.IsError {
		t.Error("expected error for empty address")
	}
}

func TestGetTRC20Balance_InvalidContract(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTRC20Balance(pool, nil), map[string]any{
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

	result := callTool(t, handleGetTRC20Balance(pool, nil), map[string]any{
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
	// 1_000_000 raw with 6 decimals = "1" (SDK trims trailing zeros)
	if data["balance"] != "1" {
		t.Errorf("balance = %v, want 1", data["balance"])
	}
}

func TestGetTRC20TokenInfo_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTRC20TokenInfo(pool, nil), map[string]any{
		"contract_address": "bad",
	})
	if !result.IsError {
		t.Error("expected error for invalid contract address")
	}
}

func TestGetTRC20TokenInfo_Success(t *testing.T) {
	mock := mockTRC20Server(big.NewInt(0), 18, "TestToken", "TTK")
	pool := newMockPool(t, mock)

	result := callTool(t, handleGetTRC20TokenInfo(pool, nil), map[string]any{
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
	if data["total_supply"] != "100000000000000000000000000" {
		t.Errorf("total_supply = %v, want 100000000000000000000000000", data["total_supply"])
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

	result := callTool(t, handleGetTRC20TokenInfo(pool, nil), map[string]any{
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	if !result.IsError {
		t.Error("expected error when name() call fails")
	}
	// Verify it's a tool error (not a Go error) with non-empty text
	found := false
	for _, c := range result.Content {
		tc, ok := c.(mcp.TextContent)
		if !ok {
			continue
		}
		found = true
		if tc.Text == "" {
			t.Error("error message should not be empty")
		}
	}
	if !found {
		t.Fatal("expected at least one mcp.TextContent in error result")
	}
}
