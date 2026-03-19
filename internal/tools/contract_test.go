package tools

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
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

func TestStringifyValue_BigInt(t *testing.T) {
	// Non-nil *big.Int
	val := big.NewInt(12345)
	got := stringifyValue(val)
	if got != "12345" {
		t.Errorf("*big.Int: got %v, want 12345", got)
	}

	// Nil *big.Int
	var nilBig *big.Int
	got = stringifyValue(nilBig)
	if got != "0" {
		t.Errorf("nil *big.Int: got %v, want 0", got)
	}
}

func TestStringifyValue_Address(t *testing.T) {
	addr := address.HexToAddress("410000000000000000000000000000000000000001")
	got := stringifyValue(addr)
	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected string, got %T", got)
	}
	if s == "" {
		t.Error("address string should not be empty")
	}
}

func TestStringifyValue_AddressSlice(t *testing.T) {
	addrs := []address.Address{
		address.HexToAddress("410000000000000000000000000000000000000001"),
		address.HexToAddress("410000000000000000000000000000000000000002"),
	}
	got := stringifyValue(addrs)
	strs, ok := got.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", got)
	}
	if len(strs) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(strs))
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

func TestDecodeABIOutput_MaxLength(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	// Data exceeding 1MB limit (each hex char = 0.5 bytes, so 2MB hex string = 1MB data)
	longData := strings.Repeat("aa", (1<<20)+1)
	result := callTool(t, handleDecodeABIOutput(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "balanceOf(address)",
		"data":             longData,
	})
	if !result.IsError {
		t.Error("expected error for data exceeding max length")
	}
}

// mockContractServer creates a mock that returns a SmartContract with a simple ABI.
func mockContractServer() *mockWalletServer {
	return &mockWalletServer{
		GetContractFunc: func(_ context.Context, _ *api.BytesMessage) (*core.SmartContract, error) {
			return &core.SmartContract{
				Abi: &core.SmartContract_ABI{
					Entrys: []*core.SmartContract_ABI_Entry{
						{
							Name: "totalSupply",
							Type: core.SmartContract_ABI_Entry_Function,
							Outputs: []*core.SmartContract_ABI_Entry_Param{
								{Name: "", Type: "uint256"},
							},
							StateMutability: core.SmartContract_ABI_Entry_View,
						},
						{
							Name: "transfer",
							Type: core.SmartContract_ABI_Entry_Function,
							Inputs: []*core.SmartContract_ABI_Entry_Param{
								{Name: "to", Type: "address"},
								{Name: "value", Type: "uint256"},
							},
							Outputs: []*core.SmartContract_ABI_Entry_Param{
								{Name: "", Type: "bool"},
							},
							StateMutability: core.SmartContract_ABI_Entry_Nonpayable,
						},
					},
				},
			}, nil
		},
	}
}

func TestTriggerConstantContract_InvalidFromAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"from":             "invalid-addr",
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "totalSupply()",
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestTriggerConstantContract_InvalidContractAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "bad",
		"method":           "totalSupply()",
	})
	if !result.IsError {
		t.Error("expected error for invalid contract address")
	}
}

func TestTriggerConstantContract_Success(t *testing.T) {
	mock := mockContractServer()
	mock.TriggerConstantContractFunc = func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		return &api.TransactionExtention{
			ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(1000000))},
			Result:         &api.Return{Result: true},
		}, nil
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "totalSupply()",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["method"] != "totalSupply()" {
		t.Errorf("method = %v, want totalSupply()", data["method"])
	}
	if data["result_hex"] == nil || data["result_hex"] == "" {
		t.Error("result_hex should not be empty")
	}
}

func TestTriggerConstantContract_Error(t *testing.T) {
	mock := &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return nil, fmt.Errorf("contract execution failed")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "totalSupply()",
	})
	if !result.IsError {
		t.Error("expected error when TriggerConstantContract fails")
	}
}

func TestGetContractABI_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetContractABI(pool), map[string]any{
		"contract_address": "bad",
	})
	if !result.IsError {
		t.Error("expected error for invalid contract address")
	}
}

func TestGetContractABI_Success(t *testing.T) {
	mock := mockContractServer()
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetContractABI(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	abi, ok := data["abi"].([]any)
	if !ok {
		t.Fatal("expected abi array")
	}
	if len(abi) != 2 {
		t.Errorf("expected 2 ABI entries, got %d", len(abi))
	}
}

func TestGetContractABI_EmptyABI(t *testing.T) {
	mock := &mockWalletServer{
		GetContractFunc: func(_ context.Context, _ *api.BytesMessage) (*core.SmartContract, error) {
			return &core.SmartContract{
				Abi: &core.SmartContract_ABI{},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetContractABI(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["note"] == nil {
		t.Error("expected note about missing ABI")
	}
}

func TestGetContractABI_Error(t *testing.T) {
	mock := &mockWalletServer{
		GetContractFunc: func(_ context.Context, _ *api.BytesMessage) (*core.SmartContract, error) {
			return nil, fmt.Errorf("contract not found")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetContractABI(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if !result.IsError {
		t.Error("expected error when GetContract fails")
	}
}

func TestListContractMethods_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleListContractMethods(pool), map[string]any{
		"contract_address": "bad",
	})
	if !result.IsError {
		t.Error("expected error for invalid address")
	}
}

func TestListContractMethods_Success(t *testing.T) {
	mock := mockContractServer()
	pool := newMockPool(t, mock)
	result := callTool(t, handleListContractMethods(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["count"] != float64(2) {
		t.Errorf("count = %v, want 2", data["count"])
	}
}

func TestListContractMethods_Error(t *testing.T) {
	mock := &mockWalletServer{
		GetContractFunc: func(_ context.Context, _ *api.BytesMessage) (*core.SmartContract, error) {
			return nil, fmt.Errorf("contract not found")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleListContractMethods(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if !result.IsError {
		t.Error("expected error when GetContract fails")
	}
}

func TestEstimateEnergy_InvalidFromAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleEstimateEnergy(pool), map[string]any{
		"from":             "bad",
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestEstimateEnergy_InvalidContractAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleEstimateEnergy(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "bad",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	if !result.IsError {
		t.Error("expected error for invalid contract address")
	}
}

func TestEstimateEnergy_Success(t *testing.T) {
	mock := &mockWalletServer{
		EstimateEnergyFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.EstimateEnergyMessage, error) {
			return &api.EstimateEnergyMessage{
				Result:         &api.Return{Result: true, Code: 0},
				EnergyRequired: 30000,
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleEstimateEnergy(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["estimated_energy"] != float64(30000) {
		t.Errorf("estimated_energy = %v, want 30000", data["estimated_energy"])
	}
}

func TestEstimateEnergy_Error(t *testing.T) {
	mock := &mockWalletServer{
		EstimateEnergyFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.EstimateEnergyMessage, error) {
			return nil, fmt.Errorf("estimation failed")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleEstimateEnergy(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	if !result.IsError {
		t.Error("expected error when estimation fails")
	}
}

func TestTriggerContract_InvalidFromAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "bad",
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestTriggerContract_InvalidContractAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "bad",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	if !result.IsError {
		t.Error("expected error for invalid contract address")
	}
}

func TestTriggerContract_InvalidFeeLimit(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"method":           "transfer(address,uint256)",
		"params":           `[]`,
		"fee_limit":        float64(20000),
	})
	if !result.IsError {
		t.Error("expected error for fee_limit > 15000")
	}
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			if !strings.Contains(tc.Text, "fee_limit must be between 0 and 15000") {
				t.Errorf("expected fee_limit validation error, got: %s", tc.Text)
			}
		}
	}
}

func TestTriggerContract_Success(t *testing.T) {
	mock := &mockWalletServer{
		TriggerContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Result: &api.Return{Result: true, Code: 0},
				Txid:   []byte{0x10, 0x20, 0x30},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["type"] != "TriggerSmartContract" {
		t.Errorf("type = %v, want TriggerSmartContract", data["type"])
	}
	if data["transaction_hex"] == nil || data["transaction_hex"] == "" {
		t.Error("transaction_hex should not be empty")
	}
	if data["txid"] == nil || data["txid"] == "" {
		t.Error("txid should not be empty")
	}
}

func TestTriggerContract_Error(t *testing.T) {
	mock := &mockWalletServer{
		TriggerContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return nil, fmt.Errorf("contract call failed")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	if !result.IsError {
		t.Error("expected error when TriggerContract fails")
	}
}

// Plain-value parameter format tests — types inferred from method signature.
// These verify that the SDK's abi.LoadFromJSONWithMethod() works end-to-end
// through the MCP tool handlers after the gotron-sdk upgrade.

func TestTriggerConstantContract_PlainParams(t *testing.T) {
	mock := mockContractServer()
	mock.TriggerConstantContractFunc = func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		return &api.TransactionExtention{
			ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(1000000))},
			Result:         &api.Return{Result: true},
		}, nil
	}
	pool := newMockPool(t, mock)

	tests := []struct {
		name   string
		params string
	}{
		{"plain-value format", `["TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"]`},
		{"typed-object format", `[{"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
				"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
				"method":           "balanceOf(address)",
				"params":           tt.params,
			})
			if result.IsError {
				t.Fatalf("expected success, got error: %v", result.Content)
			}
		})
	}
}

func TestEstimateEnergy_PlainParams(t *testing.T) {
	mock := &mockWalletServer{
		EstimateEnergyFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.EstimateEnergyMessage, error) {
			return &api.EstimateEnergyMessage{
				Result:         &api.Return{Result: true},
				EnergyRequired: 31895,
			}, nil
		},
	}
	pool := newMockPool(t, mock)

	tests := []struct {
		name   string
		params string
	}{
		{"plain-value format", `["TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", "1000000"]`},
		{"typed-object format", `[{"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"}, {"uint256": "1000000"}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callTool(t, handleEstimateEnergy(pool), map[string]any{
				"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
				"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
				"method":           "transfer(address,uint256)",
				"params":           tt.params,
			})
			if result.IsError {
				t.Fatalf("expected success, got error: %v", result.Content)
			}
		})
	}
}

func TestTriggerContract_PlainParams(t *testing.T) {
	mock := &mockWalletServer{
		TriggerContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Result:      &api.Return{Result: true},
				Txid:        make([]byte, 32),
				Transaction: &core.Transaction{RawData: &core.TransactionRaw{}},
			}, nil
		},
	}
	pool := newMockPool(t, mock)

	tests := []struct {
		name   string
		params string
	}{
		{"plain-value format", `["TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", "1000000"]`},
		{"typed-object format", `[{"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"}, {"uint256": "1000000"}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callTool(t, handleTriggerContract(pool), map[string]any{
				"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
				"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
				"method":           "transfer(address,uint256)",
				"params":           tt.params,
				"fee_limit":        float64(100),
			})
			if result.IsError {
				t.Fatalf("expected success, got error: %v", result.Content)
			}
		})
	}
}
