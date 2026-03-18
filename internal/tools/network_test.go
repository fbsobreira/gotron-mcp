package tools

import (
	"context"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

func TestGetTransaction_Success(t *testing.T) {
	txID := "0000000000000000000000000000000000000000000000000000000000000001"
	txIDBytes := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}

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
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}

func TestGetTransaction_MissingID(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetTransaction(pool), map[string]any{})
	if !result.IsError {
		t.Error("expected error for missing transaction_id")
	}
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
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}
