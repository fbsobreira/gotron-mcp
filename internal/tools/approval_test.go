package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestGetTRC20Allowance_InvalidOwner(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTRC20Allowance(pool, nil), map[string]any{
		"owner":            "bad",
		"spender":          "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	assert.True(t, result.IsError)
}

func TestGetTRC20Allowance_InvalidSpender(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTRC20Allowance(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "bad",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	assert.True(t, result.IsError)
}

func TestGetTRC20Allowance_InvalidContract(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTRC20Allowance(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "bad",
	})
	assert.True(t, result.IsError)
}

func TestGetTRC20Allowance_RPCError(t *testing.T) {
	mock := &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return nil, fmt.Errorf("node unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetTRC20Allowance(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	assert.True(t, result.IsError)
}

func TestGetTRC20Allowance_Zero(t *testing.T) {
	mock := &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(0))},
				Result:         &api.Return{Result: true},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetTRC20Allowance(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	require.False(t, result.IsError, "got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "0", data["allowance_raw"])
	assert.Equal(t, false, data["is_unlimited"])
	assert.Equal(t, "none", data["risk_level"])
}

func TestGetTRC20Allowance_Limited(t *testing.T) {
	// Return allowance (1000000000 = 1000 tokens) on first call, decimals (6) on second
	callCount := 0
	mock := &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			callCount++
			if callCount == 1 {
				// allowance call
				return &api.TransactionExtention{
					ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(1000000000))},
					Result:         &api.Return{Result: true},
				}, nil
			}
			// decimals call
			return &api.TransactionExtention{
				ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(6))},
				Result:         &api.Return{Result: true},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetTRC20Allowance(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	require.False(t, result.IsError, "got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "1000000000", data["allowance_raw"])
	assert.Equal(t, false, data["is_unlimited"])
	assert.Equal(t, "medium", data["risk_level"])
	assert.Equal(t, "1000", data["allowance_display"])
	assert.Equal(t, 2, callCount, "expected exactly 2 RPC calls (allowance + decimals)")
}

func TestGetTRC20Allowance_Unlimited(t *testing.T) {
	unlimitedBytes, _ := hex.DecodeString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	mock := &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				ConstantResult: [][]byte{unlimitedBytes},
				Result:         &api.Return{Result: true},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetTRC20Allowance(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	require.False(t, result.IsError, "got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["is_unlimited"])
	assert.Equal(t, "high", data["risk_level"])
	assert.Equal(t, "Unlimited", data["allowance_display"])
}

func TestRevokeApproval_InvalidOwner(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleRevokeApproval(pool, nil), map[string]any{
		"owner":            "bad",
		"spender":          "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	assert.True(t, result.IsError)
}

func TestRevokeApproval_InvalidSpender(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleRevokeApproval(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "bad",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	assert.True(t, result.IsError)
}

func TestRevokeApproval_InvalidContract(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleRevokeApproval(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "bad",
	})
	assert.True(t, result.IsError)
}

func TestRevokeApproval_InvalidFeeLimit(t *testing.T) {
	tests := []struct {
		name     string
		feeLimit float64
	}{
		{"above max", 20000},
		{"negative", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := newMockPool(t, &mockWalletServer{})
			result := callTool(t, handleRevokeApproval(pool, nil), map[string]any{
				"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
				"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
				"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
				"fee_limit":        tt.feeLimit,
			})
			assert.True(t, result.IsError)
		})
	}
}

func TestRevokeApproval_Success(t *testing.T) {
	mock := &mockWalletServer{
		TriggerContractFunc: func(_ context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			// Verify the calldata starts with approve(address,uint256) selector: 0x095ea7b3
			data := in.GetData()
			require.True(t, len(data) >= 4, "calldata too short")
			assert.Equal(t, "095ea7b3", hex.EncodeToString(data[:4]), "expected approve selector")

			// Verify amount is 0 (last 32 bytes should be all zeros)
			if len(data) >= 68 {
				amountBytes := data[36:68]
				allZero := true
				for _, b := range amountBytes {
					if b != 0 {
						allZero = false
						break
					}
				}
				assert.True(t, allZero, "expected amount=0 for revoke")
			}

			return &api.TransactionExtention{
				Result: &api.Return{Result: true},
				Txid:   []byte{0x01, 0x02, 0x03},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleRevokeApproval(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	require.False(t, result.IsError, "got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "RevokeApproval", data["type"])
	assert.Equal(t, "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", data["owner"])
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", data["spender"])
	assert.NotEmpty(t, data["transaction_hex"])
	assert.NotEmpty(t, data["txid"])
}

func TestRevokeApproval_BuildError(t *testing.T) {
	mock := &mockWalletServer{
		TriggerContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return nil, fmt.Errorf("contract call failed")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleRevokeApproval(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	assert.True(t, result.IsError)
}

func TestRevokeApproval_WithPermissionID(t *testing.T) {
	mock := &mockWalletServer{
		TriggerContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Result: &api.Return{Result: true},
				Txid:   []byte{0x01, 0x02, 0x03},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{
						Contract: []*core.Transaction_Contract{{}},
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleRevokeApproval(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"permission_id":    float64(2),
	})
	require.False(t, result.IsError, "got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "RevokeApproval", data["type"])
	assert.NotEmpty(t, data["transaction_hex"])

	// Verify permission_id is embedded in the built transaction
	txHex := data["transaction_hex"].(string)
	txBytes, err := hex.DecodeString(txHex)
	require.NoError(t, err)
	var tx core.Transaction
	require.NoError(t, proto.Unmarshal(txBytes, &tx))
	require.NotEmpty(t, tx.RawData.Contract)
	assert.Equal(t, int32(2), tx.RawData.Contract[0].PermissionId)
}

func TestFormatBigIntWithDecimals(t *testing.T) {
	tests := []struct {
		name     string
		amount   *big.Int
		decimals int
		want     string
	}{
		{"zero decimals", big.NewInt(1000), 0, "1000"},
		{"whole number", big.NewInt(1000000), 6, "1"},
		{"with fraction", big.NewInt(1500000), 6, "1.5"},
		{"small fraction", big.NewInt(100), 6, "0.0001"},
		{"zero", big.NewInt(0), 6, "0"},
		{"large number", new(big.Int).Mul(big.NewInt(1000000), big.NewInt(1000000)), 6, "1000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBigIntWithDecimals(tt.amount, tt.decimals)
			assert.Equal(t, tt.want, got)
		})
	}
}
