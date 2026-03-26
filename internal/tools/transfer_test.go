package tools

import (
	"context"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestTransferTRX_InvalidFromAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "invalid",
		"to":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount": "100",
	})
	assert.True(t, result.IsError, "expected error for invalid from address")
}

func TestTransferTRX_InvalidToAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "invalid",
		"amount": "100",
	})
	assert.True(t, result.IsError, "expected error for invalid to address")
}

func TestTransferTRX_InvalidAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount": "abc",
	})
	assert.True(t, result.IsError, "expected error for invalid amount")
}

func TestTransferTRX_ZeroAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount": "0",
	})
	assert.True(t, result.IsError, "expected error for zero amount")
}

func TestTransferTRX_Success(t *testing.T) {
	mock := &mockWalletServer{
		CreateTransaction2Func: func(_ context.Context, _ *core.TransferContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Txid: []byte{0x0a, 0x0b, 0x0c},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount": "100.5",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "TransferContract", data["type"])
	assert.Equal(t, "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", data["from"])
	assert.NotEmpty(t, data["transaction_hex"], "transaction_hex should not be empty")
}

func TestTransferTRX_WithMemo(t *testing.T) {
	mock := &mockWalletServer{
		CreateTransaction2Func: func(_ context.Context, _ *core.TransferContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Txid: []byte{0x0a, 0x0b, 0x0c},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{
						Contract: []*core.Transaction_Contract{{}},
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount": "1",
		"memo":   "test payment",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	// Decode the transaction_hex and verify memo was applied
	data := parseJSONResult(t, result)
	txHex := data["transaction_hex"].(string)
	txBytes, err := hex.DecodeString(txHex)
	require.NoError(t, err, "failed to decode transaction_hex")
	var tx core.Transaction
	require.NoError(t, proto.Unmarshal(txBytes, &tx), "failed to unmarshal transaction")
	assert.Equal(t, "test payment", string(tx.RawData.Data))
}

func TestTransferTRX_WithPermissionID(t *testing.T) {
	mock := &mockWalletServer{
		CreateTransaction2Func: func(_ context.Context, _ *core.TransferContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Txid: []byte{0x0a, 0x0b, 0x0c},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{
						Contract: []*core.Transaction_Contract{{}},
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":          "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":            "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":        "1",
		"permission_id": float64(2),
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	txHex := data["transaction_hex"].(string)
	txBytes, err := hex.DecodeString(txHex)
	require.NoError(t, err, "failed to decode transaction_hex")
	var tx core.Transaction
	require.NoError(t, proto.Unmarshal(txBytes, &tx), "failed to unmarshal transaction")
	require.NotEmpty(t, tx.RawData.Contract, "expected at least one contract")
	assert.Equal(t, int32(2), tx.RawData.Contract[0].PermissionId)
}

func TestTransferTRC20_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRC20(pool, nil), map[string]any{
		"from":             "invalid",
		"to":               "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	assert.True(t, result.IsError, "expected error for invalid from address")
}

func TestTransferTRC20_InvalidContractAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRC20(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "invalid",
		"amount":           "100",
	})
	assert.True(t, result.IsError, "expected error for invalid contract address")
}

func TestTransferTRC20_InvalidToAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRC20(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "bad",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	assert.True(t, result.IsError, "expected error for invalid to address")
}

func TestTransferTRC20_InvalidFeeLimit(t *testing.T) {
	// Need a mock that returns decimals to get past that check
	mock := &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(6))},
				Result:         &api.Return{Result: true},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTransferTRC20(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
		"fee_limit":        float64(20000),
	})
	assert.True(t, result.IsError, "expected error for fee_limit > 15000")
}

func TestTransferTRC20_Success(t *testing.T) {
	mock := &mockWalletServer{
		// TRC20GetDecimalsCtx calls TriggerConstantContract
		TriggerConstantContractFunc: func(_ context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(6))},
				Result:         &api.Return{Result: true},
			}, nil
		},
		// TRC20SendCtx calls TriggerContract
		TriggerContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Result: &api.Return{Result: true, Code: 0},
				Txid:   []byte{0x0d, 0x0e, 0x0f},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTransferTRC20(pool, nil), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "TriggerSmartContract", data["type"])
	assert.NotEmpty(t, data["transaction_hex"], "transaction_hex should not be empty")
}
