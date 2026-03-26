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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.Len(t, got, tt.want)
		})
	}
}

func TestStringifyValue(t *testing.T) {
	// Test with basic types that don't require address import
	got := stringifyValue("hello")
	assert.Equal(t, "hello", got)

	got = stringifyValue(42)
	assert.Equal(t, 42, got)

	got = stringifyValue(nil)
	assert.Nil(t, got)
}

func TestStringifyValue_BigInt(t *testing.T) {
	// Non-nil *big.Int
	val := big.NewInt(12345)
	got := stringifyValue(val)
	assert.Equal(t, "12345", got)

	// Nil *big.Int
	var nilBig *big.Int
	got = stringifyValue(nilBig)
	assert.Equal(t, "0", got)
}

func TestStringifyValue_Address(t *testing.T) {
	addr := address.BytesToAddress([]byte{0x41, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	got := stringifyValue(addr)
	s, ok := got.(string)
	require.True(t, ok, "expected string, got %T", got)
	assert.NotEmpty(t, s, "address string should not be empty")
}

func TestStringifyValue_AddressSlice(t *testing.T) {
	addrs := []address.Address{
		address.BytesToAddress([]byte{0x41, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}),
		address.BytesToAddress([]byte{0x41, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}),
	}
	got := stringifyValue(addrs)
	strs, ok := got.([]string)
	require.True(t, ok, "expected []string, got %T", got)
	assert.Len(t, strs, 2, "expected 2 addresses")
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
			assert.Equal(t, tt.want, got)
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
	assert.True(t, result.IsError, "expected error for empty contract address")
}

func TestDecodeABIOutput_InvalidHex(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleDecodeABIOutput(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "balanceOf(address)",
		"data":             "not-hex",
	})
	assert.True(t, result.IsError, "expected error for invalid hex data")
}

func TestDecodeABIOutput_EmptyData(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleDecodeABIOutput(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "balanceOf(address)",
		"data":             "",
	})
	assert.True(t, result.IsError, "expected error for empty data")
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
	require.True(t, result.IsError, "expected ABI fetch error from mock")
	// Verify the error is NOT about invalid hex — hex parsing with 0x prefix succeeded
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			assert.False(t, strings.Contains(tc.Text, "invalid hex"), "0x prefix should be stripped — got invalid hex error")
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
	require.False(t, result.IsError, "expected success (revert decode), got error: %v", result.Content)
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
	assert.True(t, result.IsError, "expected error for data exceeding max length")
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
	assert.True(t, result.IsError, "expected error for invalid from address")
}

func TestTriggerConstantContract_InvalidContractAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "bad",
		"method":           "totalSupply()",
	})
	assert.True(t, result.IsError, "expected error for invalid contract address")
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
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "totalSupply()", data["method"], "method")
	assert.NotNil(t, data["result_hex"], "result_hex should not be nil")
	assert.NotEqual(t, "", data["result_hex"], "result_hex should not be empty")
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
	assert.True(t, result.IsError, "expected error when TriggerConstantContract fails")
}

func TestGetContractABI_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetContractABI(pool), map[string]any{
		"contract_address": "bad",
	})
	assert.True(t, result.IsError, "expected error for invalid contract address")
}

func TestGetContractABI_Success(t *testing.T) {
	mock := mockContractServer()
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetContractABI(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	abi, ok := data["abi"].([]any)
	require.True(t, ok, "expected abi array")
	assert.Len(t, abi, 2, "expected 2 ABI entries")
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
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.NotNil(t, data["note"], "expected note about missing ABI")
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
	assert.True(t, result.IsError, "expected error when GetContract fails")
}

func TestListContractMethods_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleListContractMethods(pool), map[string]any{
		"contract_address": "bad",
	})
	assert.True(t, result.IsError, "expected error for invalid address")
}

func TestListContractMethods_Success(t *testing.T) {
	mock := mockContractServer()
	pool := newMockPool(t, mock)
	result := callTool(t, handleListContractMethods(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, float64(2), data["count"], "count")
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
	assert.True(t, result.IsError, "expected error when GetContract fails")
}

func TestEstimateEnergy_InvalidFromAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleEstimateEnergy(pool), map[string]any{
		"from":             "bad",
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	assert.True(t, result.IsError, "expected error for invalid from address")
}

func TestEstimateEnergy_InvalidContractAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleEstimateEnergy(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "bad",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	assert.True(t, result.IsError, "expected error for invalid contract address")
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
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, float64(30000), data["estimated_energy"], "estimated_energy")
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
	assert.True(t, result.IsError, "expected error when estimation fails")
}

func TestTriggerContract_InvalidFromAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "bad",
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	assert.True(t, result.IsError, "expected error for invalid from address")
}

func TestTriggerContract_InvalidContractAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "bad",
		"method":           "transfer(address,uint256)",
		"params":           `[{"address":"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"},{"uint256":"1000"}]`,
	})
	assert.True(t, result.IsError, "expected error for invalid contract address")
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
	assert.True(t, result.IsError, "expected error for fee_limit > 15000")
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			assert.Contains(t, tc.Text, "fee_limit must be between 0 and 15000")
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
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "TriggerSmartContract", data["type"], "type")
	assert.NotNil(t, data["transaction_hex"], "transaction_hex should not be nil")
	assert.NotEqual(t, "", data["transaction_hex"], "transaction_hex should not be empty")
	assert.NotNil(t, data["txid"], "txid should not be nil")
	assert.NotEqual(t, "", data["txid"], "txid should not be empty")
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
	assert.True(t, result.IsError, "expected error when TriggerContract fails")
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
			require.False(t, result.IsError, "expected success, got error: %v", result.Content)
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
			require.False(t, result.IsError, "expected success, got error: %v", result.Content)
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
			require.False(t, result.IsError, "expected success, got error: %v", result.Content)
		})
	}
}

func TestTriggerConstantContract_PrePackedData(t *testing.T) {
	mock := mockContractServer()
	mock.TriggerConstantContractFunc = func(_ context.Context, ct *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		assert.NotEmpty(t, ct.Data, "expected pre-packed data to be set on TriggerSmartContract")
		return &api.TransactionExtention{
			ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(42))},
			Result:         &api.Return{Result: true},
		}, nil
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"data":             "0x70a082310000000000000000000000005cbdd86a2fa8dc4bddd8a8f69dba48572eec07fb",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
	data := extractJSON(t, result)
	assert.NotNil(t, data["result_hex"], "result_hex should not be nil")
	assert.NotEqual(t, "", data["result_hex"], "result_hex should not be empty")
}

func TestTriggerConstantContract_DataRequiresNoMethod(t *testing.T) {
	mock := mockContractServer()
	mock.TriggerConstantContractFunc = func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		return &api.TransactionExtention{
			ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(1))},
			Result:         &api.Return{Result: true},
		}, nil
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"data":             "70a08231",
	})
	require.False(t, result.IsError, "expected success with data and no method, got error: %v", result.Content)
}

func TestTriggerConstantContract_MethodRequiredWithoutData(t *testing.T) {
	pool := newMockPool(t, mockContractServer())
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	assert.True(t, result.IsError, "expected error when neither method nor data is provided")
}

func TestTriggerConstantContract_CallValue(t *testing.T) {
	mock := mockContractServer()
	mock.TriggerConstantContractFunc = func(_ context.Context, ct *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		assert.Equal(t, int64(1_000_000), ct.CallValue, "CallValue")
		return &api.TransactionExtention{
			ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(1))},
			Result:         &api.Return{Result: true},
		}, nil
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "deposit()",
		"call_value":       float64(1_000_000),
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
}

func TestTriggerConstantContract_InvalidDataHex(t *testing.T) {
	pool := newMockPool(t, mockContractServer())
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"data":             "not-valid-hex",
	})
	assert.True(t, result.IsError, "expected error for invalid data hex")
}

func TestTriggerContract_PrePackedData(t *testing.T) {
	mock := &mockWalletServer{
		TriggerContractFunc: func(_ context.Context, ct *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			assert.NotEmpty(t, ct.Data, "expected pre-packed data to be set")
			return &api.TransactionExtention{
				Transaction: &core.Transaction{RawData: &core.TransactionRaw{}},
				Txid:        []byte("mock-txid-1234567890123456"),
				Result:      &api.Return{Result: true},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
		"data":             "0xa9059cbb0000000000000000000000005cbdd86a2fa8dc4bddd8a8f69dba48572eec07fb00000000000000000000000000000000000000000000000000000000000f4240",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
}

func TestTriggerContract_MethodRequiredWithoutData(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
	})
	assert.True(t, result.IsError, "expected error when neither method nor data is provided")
}

func TestTriggerConstantContract_TokenValue(t *testing.T) {
	mock := mockContractServer()
	mock.TriggerConstantContractFunc = func(_ context.Context, ct *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		assert.Equal(t, int64(1000001), ct.TokenId, "TokenId")
		assert.Equal(t, int64(500), ct.CallTokenValue, "CallTokenValue")
		return &api.TransactionExtention{
			ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(1))},
			Result:         &api.Return{Result: true},
		}, nil
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "deposit()",
		"token_id":         "1000001",
		"token_value":      float64(500),
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
}

func TestTriggerConstantContract_TokenValueWithoutID(t *testing.T) {
	pool := newMockPool(t, mockContractServer())
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "deposit()",
		"token_value":      float64(500),
	})
	assert.True(t, result.IsError, "expected error for token_value without token_id")
}

func TestTriggerConstantContract_TokenIDWithoutValue(t *testing.T) {
	pool := newMockPool(t, mockContractServer())
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "deposit()",
		"token_id":         "1000001",
	})
	assert.True(t, result.IsError, "expected error for token_id without token_value")
}

func TestTriggerConstantContract_NegativeCallValue(t *testing.T) {
	pool := newMockPool(t, mockContractServer())
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "deposit()",
		"call_value":       float64(-1),
	})
	assert.True(t, result.IsError, "expected error for negative call_value")
}

func TestTriggerConstantContract_NegativeTokenValue(t *testing.T) {
	pool := newMockPool(t, mockContractServer())
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "deposit()",
		"token_id":         "1000001",
		"token_value":      float64(-1),
	})
	assert.True(t, result.IsError, "expected error for negative token_value")
}

func TestTriggerConstantContract_DataAndMethodIgnoresMethod(t *testing.T) {
	mock := mockContractServer()
	mock.TriggerConstantContractFunc = func(_ context.Context, ct *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		// When data is provided, the SDK receives raw bytes, not method+params
		assert.NotEmpty(t, ct.Data, "expected pre-packed data")
		return &api.TransactionExtention{
			ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(1))},
			Result:         &api.Return{Result: true},
		}, nil
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"method":           "balanceOf(address)",
		"data":             "70a08231",
	})
	require.False(t, result.IsError, "expected success when both data and method are provided, got error: %v", result.Content)
}

func TestTriggerConstantContract_OversizedData(t *testing.T) {
	pool := newMockPool(t, mockContractServer())
	// Generate data hex > 1 MiB decoded (each "ab" = 1 byte)
	bigData := strings.Repeat("ab", (1<<20)+1)
	result := callTool(t, handleTriggerConstantContract(pool), map[string]any{
		"contract_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"data":             bigData,
	})
	assert.True(t, result.IsError, "expected error for oversized data")
}

func TestTriggerContract_NegativeCallValue(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
		"method":           "transfer(address,uint256)",
		"params":           `["TJRabPrwbZy45sbavfcjinPJC18kjpRTv8", "1000"]`,
		"call_value":       float64(-1),
	})
	assert.True(t, result.IsError, "expected error for negative call_value")
}

func TestTriggerContract_OversizedData(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	bigData := strings.Repeat("ab", (1<<20)+1)
	result := callTool(t, handleTriggerContract(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
		"data":             bigData,
	})
	assert.True(t, result.IsError, "expected error for oversized data")
}
