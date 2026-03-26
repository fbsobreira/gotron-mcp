package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestGetTransaction_Success(t *testing.T) {
	txID := "0000000000000000000000000000000000000000000000000000000000000001"
	txIDBytes, err := hex.DecodeString(txID)
	require.NoError(t, err, "failed to decode txID")

	mock := &mockWalletServer{
		GetTransactionByIdFunc: func(_ context.Context, _ *api.BytesMessage) (*core.Transaction, error) {
			return &core.Transaction{
				RawData: &core.TransactionRaw{
					Contract: []*core.Transaction_Contract{
						{Type: core.Transaction_Contract_TransferContract},
					},
				},
			}, nil
		},
		GetTransactionInfoByIdFunc: func(_ context.Context, in *api.BytesMessage) (*core.TransactionInfo, error) {
			return &core.TransactionInfo{
				Id:             txIDBytes,
				BlockNumber:    12345,
				BlockTimeStamp: 1700000000,
				Fee:            1000,
				Receipt: &core.ResourceReceipt{
					EnergyUsage:      100,
					EnergyFee:        200,
					NetUsage:         300,
					EnergyUsageTotal: 400,
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetTransaction(pool), map[string]any{
		"transaction_id": txID,
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
}

func TestGetTransaction_ContractData(t *testing.T) {
	txID := "0000000000000000000000000000000000000000000000000000000000000002"
	txIDBytes, err := hex.DecodeString(txID)
	require.NoError(t, err, "failed to decode txID")

	// Build a real TransferContract proto parameter
	transfer := &core.TransferContract{
		OwnerAddress: mustDecodeAddr("TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"),
		ToAddress:    mustDecodeAddr("TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"),
		Amount:       5_000_000, // 5 TRX
	}
	paramAny, err := anypb.New(transfer)
	require.NoError(t, err, "failed to create Any")

	mock := &mockWalletServer{
		GetTransactionByIdFunc: func(_ context.Context, _ *api.BytesMessage) (*core.Transaction, error) {
			return &core.Transaction{
				RawData: &core.TransactionRaw{
					Contract: []*core.Transaction_Contract{
						{
							Type: core.Transaction_Contract_TransferContract,
							Parameter: &anypb.Any{
								TypeUrl: paramAny.TypeUrl,
								Value:   paramAny.Value,
							},
						},
					},
				},
			}, nil
		},
		GetTransactionInfoByIdFunc: func(_ context.Context, _ *api.BytesMessage) (*core.TransactionInfo, error) {
			return &core.TransactionInfo{
				Id:          txIDBytes,
				BlockNumber: 99999,
				Fee:         500,
				Receipt:     &core.ResourceReceipt{},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetTransaction(pool), map[string]any{
		"transaction_id": txID,
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "TransferContract", data["contract_type"], "contract_type")
	contractData, ok := data["contract_data"].(map[string]any)
	require.True(t, ok, "expected contract_data map in response")
	assert.Equal(t, "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", contractData["owner_address"], "owner_address")
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", contractData["to_address"], "to_address")
	assert.Equal(t, "5.000000", contractData["amount"], "amount")
}

// mustDecodeAddr decodes a base58 TRON address to bytes for test fixtures.
func mustDecodeAddr(addr string) []byte {
	a, err := address.Base58ToAddress(addr)
	if err != nil {
		panic(fmt.Sprintf("invalid test address %q: %v", addr, err))
	}
	return a.Bytes()
}

func TestGetTransaction_MissingID(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTransaction(pool), map[string]any{})
	assert.True(t, result.IsError, "expected error for missing transaction_id")
}

func TestGetNetwork_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetNowBlock2Func: func(_ context.Context, _ *api.EmptyMessage) (*api.BlockExtention, error) {
			return &api.BlockExtention{
				BlockHeader: &core.BlockHeader{
					RawData: &core.BlockHeaderRaw{
						Number:    99999,
						Timestamp: 1700000000,
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetNetwork(pool, "mainnet", "mock:50051"), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
}

func TestGetChainParameters_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetNodeInfoFunc: func(_ context.Context, _ *api.EmptyMessage) (*core.NodeInfo, error) {
			return &core.NodeInfo{
				BeginSyncNum:  1000,
				Block:         "block-hash-123",
				SolidityBlock: "solidity-block-456",
			}, nil
		},
		GetEnergyPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "100:420"}, nil
		},
		GetBandwidthPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "200:1000"}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetChainParameters(pool), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	nodeInfo, ok := data["node_info"].(map[string]any)
	require.True(t, ok, "expected node_info map")
	assert.Equal(t, float64(1000), nodeInfo["begin_sync_num"], "begin_sync_num")
}

func TestGetChainParameters_Error(t *testing.T) {
	mock := &mockWalletServer{
		GetNodeInfoFunc: func(_ context.Context, _ *api.EmptyMessage) (*core.NodeInfo, error) {
			return nil, fmt.Errorf("node info unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetChainParameters(pool), map[string]any{})
	assert.True(t, result.IsError, "expected error when GetNodeInfo fails")
}

func TestGetEnergyPrices_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetEnergyPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "100:420"}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetEnergyPrices(pool), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
}

func TestGetEnergyPrices_Error(t *testing.T) {
	mock := &mockWalletServer{
		GetEnergyPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return nil, fmt.Errorf("energy prices unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetEnergyPrices(pool), map[string]any{})
	assert.True(t, result.IsError, "expected error when GetEnergyPrices fails")
}

func TestGetBandwidthPrices_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetBandwidthPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "200:1000"}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBandwidthPrices(pool), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
}

func TestGetBandwidthPrices_Error(t *testing.T) {
	mock := &mockWalletServer{
		GetBandwidthPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return nil, fmt.Errorf("bandwidth prices unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBandwidthPrices(pool), map[string]any{})
	assert.True(t, result.IsError, "expected error when GetBandwidthPrices fails")
}

func TestGetPendingTransactions_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetPendingSizeFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.NumberMessage, error) {
			return &api.NumberMessage{Num: 3}, nil
		},
		GetTransactionListFromPendFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.TransactionIdList, error) {
			return &api.TransactionIdList{TxId: []string{"tx1", "tx2", "tx3"}}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetPendingTransactions(pool), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
	data := parseJSONResult(t, result)
	assert.Equal(t, float64(3), data["pool_size"], "pool_size")
	ids, ok := data["transaction_ids"].([]any)
	require.True(t, ok, "expected transaction_ids array")
	assert.Len(t, ids, 3, "transaction_ids length")
}

func TestGetPendingTransactions_EmptyPool(t *testing.T) {
	mock := &mockWalletServer{
		GetPendingSizeFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.NumberMessage, error) {
			return &api.NumberMessage{Num: 0}, nil
		},
		GetTransactionListFromPendFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.TransactionIdList, error) {
			return &api.TransactionIdList{}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetPendingTransactions(pool), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
	data := parseJSONResult(t, result)
	assert.Equal(t, float64(0), data["pool_size"], "pool_size")
	// Verify transaction_ids is [] not null
	ids, ok := data["transaction_ids"].([]any)
	require.True(t, ok, "expected transaction_ids to be an array, not null")
	assert.Len(t, ids, 0, "transaction_ids length")
}

func TestGetPendingTransactions_SizeError(t *testing.T) {
	mock := &mockWalletServer{
		GetPendingSizeFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.NumberMessage, error) {
			return nil, fmt.Errorf("pending size unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetPendingTransactions(pool), map[string]any{})
	assert.True(t, result.IsError, "expected error when GetPendingSize fails")
}

func TestGetPendingTransactions_ListError(t *testing.T) {
	mock := &mockWalletServer{
		GetPendingSizeFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.NumberMessage, error) {
			return &api.NumberMessage{Num: 3}, nil
		},
		GetTransactionListFromPendFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.TransactionIdList, error) {
			return nil, fmt.Errorf("list pending unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetPendingTransactions(pool), map[string]any{})
	assert.True(t, result.IsError, "expected error when GetTransactionListFromPending fails")
}

func TestIsTransactionPending_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetTransactionFromPendingFunc: func(_ context.Context, in *api.BytesMessage) (*core.Transaction, error) {
			return &core.Transaction{RawData: &core.TransactionRaw{}}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleIsTransactionPending(pool), map[string]any{
		"transaction_id": "0000000000000000000000000000000000000000000000000000000000000001",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["pending"], "pending")
}

func TestIsTransactionPending_NotFound(t *testing.T) {
	mock := &mockWalletServer{
		GetTransactionFromPendingFunc: func(_ context.Context, _ *api.BytesMessage) (*core.Transaction, error) {
			return &core.Transaction{}, nil // empty tx = not found
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleIsTransactionPending(pool), map[string]any{
		"transaction_id": "0000000000000000000000000000000000000000000000000000000000000001",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
	data := parseJSONResult(t, result)
	assert.Equal(t, false, data["pending"], "pending")
}

func TestIsTransactionPending_MissingID(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleIsTransactionPending(pool), map[string]any{})
	assert.True(t, result.IsError, "expected error for missing transaction_id")
}

func TestGetPendingByAddress_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetTransactionListFromPendFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.TransactionIdList, error) {
			return &api.TransactionIdList{TxId: []string{}}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetPendingByAddress(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
	data := parseJSONResult(t, result)
	assert.Equal(t, "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", data["address"], "address")
	assert.Equal(t, float64(0), data["count"], "count")
}

func TestGetPendingByAddress_WithTransactions(t *testing.T) {
	ownerAddr := mustDecodeAddr("TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF")
	transfer := &core.TransferContract{
		OwnerAddress: ownerAddr,
		ToAddress:    mustDecodeAddr("TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"),
		Amount:       1_000_000,
	}
	paramAny, err := anypb.New(transfer)
	require.NoError(t, err, "failed to create Any")

	mock := &mockWalletServer{
		GetTransactionListFromPendFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.TransactionIdList, error) {
			return &api.TransactionIdList{TxId: []string{"abc123"}}, nil
		},
		GetTransactionFromPendingFunc: func(_ context.Context, _ *api.BytesMessage) (*core.Transaction, error) {
			return &core.Transaction{
				RawData: &core.TransactionRaw{
					Contract: []*core.Transaction_Contract{
						{
							Type:      core.Transaction_Contract_TransferContract,
							Parameter: &anypb.Any{TypeUrl: paramAny.TypeUrl, Value: paramAny.Value},
						},
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetPendingByAddress(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
	data := parseJSONResult(t, result)
	assert.Equal(t, float64(1), data["count"], "count")
	txs, ok := data["transactions"].([]any)
	require.True(t, ok && len(txs) > 0, "expected non-empty transactions array")
	tx0 := txs[0].(map[string]any)
	assert.Equal(t, "TransferContract", tx0["contract_type"], "contract_type")
	assert.NotNil(t, tx0["transaction_id"], "expected transaction_id in entry")
	assert.NotEqual(t, "", tx0["transaction_id"], "expected transaction_id in entry")
	cd, ok := tx0["contract_data"].(map[string]any)
	require.True(t, ok, "expected contract_data map in entry")
	assert.Equal(t, "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", cd["owner_address"], "owner_address")
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", cd["to_address"], "to_address")
	assert.Equal(t, "1.000000", cd["amount"], "amount")
}

func TestGetPendingByAddress_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetPendingByAddress(pool), map[string]any{
		"address": "invalid",
	})
	assert.True(t, result.IsError, "expected error for invalid address")
}

func TestNormalizeResult(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"SUCESS", "SUCCESS"},
		{"SUCCESS", "SUCCESS"},
		{"FAILURE", "FAILURE"},
		{"", ""},
		{"SUCESS_SUCESS", "SUCCESS_SUCCESS"},
	}
	for _, tt := range tests {
		got := normalizeResult(tt.in)
		assert.Equal(t, tt.want, got, "normalizeResult(%q)", tt.in)
	}
}
