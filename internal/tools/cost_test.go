package tools

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeTransferCost_InvalidFrom(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			t.Fatal("RPC should not be called for invalid address")
			return nil, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeTransferCost(pool, nil), map[string]any{
		"from":   "invalid",
		"to":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount": "100",
	})
	assert.True(t, result.IsError)
}

func TestAnalyzeTransferCost_InvalidTo(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleAnalyzeTransferCost(pool, nil), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "bad",
		"amount": "100",
	})
	assert.True(t, result.IsError)
}

func TestAnalyzeTransferCost_InvalidContract(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleAnalyzeTransferCost(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
		"contract_address": "bad",
	})
	assert.True(t, result.IsError)
}

func TestAnalyzeTransferCost_ResourceError(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return nil, fmt.Errorf("node unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeTransferCost(pool, nil), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount": "100",
	})
	assert.True(t, result.IsError)
}

func TestAnalyzeTransferCost_TRXTransfer(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{
				EnergyLimit:  50000,
				EnergyUsed:   1000,
				NetLimit:     2400,
				NetUsed:      200,
				FreeNetLimit: 600,
				FreeNetUsed:  100,
			}, nil
		},
		GetEnergyPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:420"}, nil
		},
		GetBandwidthPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:1000"}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeTransferCost(pool, nil), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount": "100",
	})
	require.False(t, result.IsError, "got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "TRX", data["transfer_type"])
	assert.Equal(t, float64(0), data["energy_required"])
	assert.Equal(t, float64(267), data["bandwidth_required"])

	// Should have enough bandwidth (2400-200 + 600-100 = 2700 > 267)
	assert.Equal(t, float64(2700), data["account_bandwidth_available"])
	assert.Equal(t, "0.000000", data["total_estimated_cost_trx"])

	breakdown := data["cost_breakdown"].([]any)
	require.Len(t, breakdown, 1) // only bandwidth (no energy for TRX)
	bw := breakdown[0].(map[string]any)
	assert.Equal(t, "bandwidth", bw["resource"])
	assert.Equal(t, "use_staked", bw["method"])
}

func TestAnalyzeTransferCost_TRXTransfer_InsufficientBandwidth(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{
				NetLimit:     100,
				NetUsed:      100,
				FreeNetLimit: 100,
				FreeNetUsed:  100,
			}, nil
		},
		GetEnergyPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:420"}, nil
		},
		GetBandwidthPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:1000"}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeTransferCost(pool, nil), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount": "100",
	})
	require.False(t, result.IsError)

	data := parseJSONResult(t, result)
	assert.Equal(t, float64(0), data["account_bandwidth_available"])

	// Should burn TRX for bandwidth: 267 * 1000 = 267000 SUN
	breakdown := data["cost_breakdown"].([]any)
	require.Len(t, breakdown, 1)
	bw := breakdown[0].(map[string]any)
	assert.Equal(t, "burn_trx", bw["method"])
	assert.Equal(t, float64(267000), bw["cost_sun"])
}

func TestAnalyzeTransferCost_TRC20Transfer(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{
				EnergyLimit:  50000,
				EnergyUsed:   1000,
				NetLimit:     2400,
				FreeNetLimit: 600,
			}, nil
		},
		GetEnergyPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:420"}, nil
		},
		GetBandwidthPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:1000"}, nil
		},
		// Decimals call
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(6))},
				Result:         &api.Return{Result: true},
			}, nil
		},
		// EstimateEnergy
		EstimateEnergyFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.EstimateEnergyMessage, error) {
			return &api.EstimateEnergyMessage{
				EnergyRequired: 29000,
				Result:         &api.Return{Result: true},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeTransferCost(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	require.False(t, result.IsError, "got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "TRC20", data["transfer_type"])
	assert.Equal(t, float64(29000), data["energy_required"])
	assert.Equal(t, float64(345), data["bandwidth_required"])
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", data["contract_address"])

	// Has enough energy (50000-1000=49000 > 29000), enough bandwidth
	breakdown := data["cost_breakdown"].([]any)
	require.Len(t, breakdown, 2) // energy + bandwidth
	energy := breakdown[0].(map[string]any)
	assert.Equal(t, "energy", energy["resource"])
	assert.Equal(t, "use_staked", energy["method"])

	assert.Equal(t, "0.000000", data["total_estimated_cost_trx"])
}

func TestAnalyzeTransferCost_TRC20_InsufficientEnergy(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{
				EnergyLimit:  10000,
				EnergyUsed:   5000,
				NetLimit:     2400,
				FreeNetLimit: 600,
			}, nil
		},
		GetEnergyPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:420"}, nil
		},
		GetBandwidthPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:1000"}, nil
		},
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(6))},
				Result:         &api.Return{Result: true},
			}, nil
		},
		EstimateEnergyFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.EstimateEnergyMessage, error) {
			return &api.EstimateEnergyMessage{
				EnergyRequired: 29000,
				Result:         &api.Return{Result: true},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeTransferCost(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	require.False(t, result.IsError)

	data := parseJSONResult(t, result)
	// Available: 10000-5000=5000, need 29000 → deficit 24000
	breakdown := data["cost_breakdown"].([]any)
	energy := breakdown[0].(map[string]any)
	assert.Equal(t, "burn_trx", energy["method"])
	assert.Equal(t, float64(24000), energy["energy_deficit"])
	// 24000 * 420 = 10080000 SUN
	assert.Equal(t, float64(10080000), energy["cost_sun"])

	totalCost := data["total_estimated_cost_sun"].(float64)
	assert.Greater(t, totalCost, float64(0))
}

func TestAnalyzeTransferCost_TRC20_EnergyEstimationFails(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{
				EnergyLimit:  50000,
				NetLimit:     2400,
				FreeNetLimit: 600,
			}, nil
		},
		GetEnergyPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:420"}, nil
		},
		GetBandwidthPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:1000"}, nil
		},
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(6))},
				Result:         &api.Return{Result: true},
			}, nil
		},
		EstimateEnergyFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.EstimateEnergyMessage, error) {
			return nil, fmt.Errorf("estimation not supported")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeTransferCost(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	// Energy estimation failure is non-fatal
	require.False(t, result.IsError)

	data := parseJSONResult(t, result)
	assert.Equal(t, "TRC20", data["transfer_type"])
	assert.Equal(t, float64(0), data["energy_required"]) // 0 when estimation fails
	assert.Contains(t, data["warning"], "Energy estimation unavailable")
}

func TestBuildCostResult(t *testing.T) {
	res := &api.AccountResourceMessage{
		EnergyLimit:  100000,
		EnergyUsed:   0,
		NetLimit:     5000,
		NetUsed:      0,
		FreeNetLimit: 600,
		FreeNetUsed:  0,
	}
	result := buildCostResult("TRX", "Tfrom", "Tto", "", 0, 267, res, 420, 1000, true)
	assert.Equal(t, "TRX", result["transfer_type"])
	assert.Equal(t, int64(0), result["total_estimated_cost_sun"])
	assert.Equal(t, int64(5600), result["account_bandwidth_available"])
}
