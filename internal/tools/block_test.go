package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestGetBlock_Latest(t *testing.T) {
	mock := &mockWalletServer{
		GetNowBlock2Func: func(_ context.Context, _ *api.EmptyMessage) (*api.BlockExtention, error) {
			return &api.BlockExtention{
				Blockid: []byte{0x01, 0x02},
				BlockHeader: &core.BlockHeader{
					RawData: &core.BlockHeaderRaw{
						Number:    12345,
						Timestamp: 1700000000,
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBlock(pool), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
}

func TestGetBlock_ByNumber(t *testing.T) {
	mock := &mockWalletServer{
		GetBlockByNum2Func: func(_ context.Context, in *api.NumberMessage) (*api.BlockExtention, error) {
			return &api.BlockExtention{
				Blockid: []byte{0x03, 0x04},
				BlockHeader: &core.BlockHeader{
					RawData: &core.BlockHeaderRaw{
						Number:    in.Num,
						Timestamp: 1700000000,
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBlock(pool), map[string]any{
		"block_number": float64(100),
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
}

func makeBlockWithTransactions(txCount int) *api.BlockExtention {
	txs := make([]*api.TransactionExtention, txCount)
	for i := range txCount {
		contractType := core.Transaction_Contract_TransferContract
		if i%2 == 1 {
			contractType = core.Transaction_Contract_TriggerSmartContract
		}
		txs[i] = &api.TransactionExtention{
			Txid: []byte{byte(i)},
			Transaction: &core.Transaction{
				RawData: &core.TransactionRaw{
					Contract: []*core.Transaction_Contract{
						{
							Type:      contractType,
							Parameter: &anypb.Any{},
						},
					},
				},
			},
		}
	}
	return &api.BlockExtention{
		Blockid:      []byte{0xaa, 0xbb},
		Transactions: txs,
		BlockHeader: &core.BlockHeader{
			RawData: &core.BlockHeaderRaw{
				Number:    500,
				Timestamp: 1700000000,
			},
		},
	}
}

func TestGetBlock_WithTransactions(t *testing.T) {
	block := makeBlockWithTransactions(10)
	mock := &mockWalletServer{
		GetBlockByNum2Func: func(_ context.Context, _ *api.NumberMessage) (*api.BlockExtention, error) {
			return block, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBlock(pool), map[string]any{
		"block_number":         float64(500),
		"include_transactions": true,
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, float64(10), data["transaction_count"], "transaction_count")
	txs, ok := data["transactions"].([]any)
	require.True(t, ok, "expected transactions array")
	assert.Len(t, txs, 10, "transactions length")
}

func TestGetBlock_WithTypeFilter(t *testing.T) {
	block := makeBlockWithTransactions(10) // 5 Transfer, 5 TriggerSmart
	mock := &mockWalletServer{
		GetBlockByNum2Func: func(_ context.Context, _ *api.NumberMessage) (*api.BlockExtention, error) {
			return block, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBlock(pool), map[string]any{
		"block_number":         float64(500),
		"include_transactions": true,
		"transaction_type":     "TransferContract",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	filtered := int(data["transactions_filtered"].(float64))
	assert.Equal(t, 5, filtered, "transactions_filtered")
}

func TestGetBlock_WithPagination(t *testing.T) {
	block := makeBlockWithTransactions(10)
	mock := &mockWalletServer{
		GetBlockByNum2Func: func(_ context.Context, _ *api.NumberMessage) (*api.BlockExtention, error) {
			return block, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBlock(pool), map[string]any{
		"block_number":         float64(500),
		"include_transactions": true,
		"limit":                float64(3),
		"offset":               float64(2),
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	returned := int(data["transactions_returned"].(float64))
	assert.Equal(t, 3, returned, "transactions_returned")
	assert.Equal(t, true, data["has_more"], "expected has_more = true")
}

func TestGetBlock_LatestError(t *testing.T) {
	mock := &mockWalletServer{
		GetNowBlock2Func: func(_ context.Context, _ *api.EmptyMessage) (*api.BlockExtention, error) {
			return nil, fmt.Errorf("node unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBlock(pool), map[string]any{})
	assert.True(t, result.IsError, "expected error when latest block fails")
}

func TestGetBlock_ByNumberError(t *testing.T) {
	mock := &mockWalletServer{
		GetBlockByNum2Func: func(_ context.Context, _ *api.NumberMessage) (*api.BlockExtention, error) {
			return nil, fmt.Errorf("block not found")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBlock(pool), map[string]any{
		"block_number": float64(999999),
	})
	assert.True(t, result.IsError, "expected error when block number fails")
}
