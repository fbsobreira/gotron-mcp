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
	// Return decimals (6) on first call, allowance (1000000000 = 1000 tokens) on second
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

func TestRevokeApproval_InvalidFeeLimit(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleRevokeApproval(pool, nil), map[string]any{
		"owner":            "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"spender":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"fee_limit":        float64(20000),
	})
	assert.True(t, result.IsError)
}

func TestRevokeApproval_Success(t *testing.T) {
	mock := &mockWalletServer{
		TriggerContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
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
