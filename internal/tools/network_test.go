package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

func TestGetTransaction_Success(t *testing.T) {
	txID := "0000000000000000000000000000000000000000000000000000000000000001"
	txIDBytes, err := hex.DecodeString(txID)
	if err != nil {
		t.Fatalf("failed to decode txID: %v", err)
	}

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
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	nodeInfo, ok := data["node_info"].(map[string]any)
	if !ok {
		t.Fatal("expected node_info map")
	}
	if nodeInfo["begin_sync_num"] != float64(1000) {
		t.Errorf("begin_sync_num = %v, want 1000", nodeInfo["begin_sync_num"])
	}
}

func TestGetChainParameters_Error(t *testing.T) {
	mock := &mockWalletServer{
		GetNodeInfoFunc: func(_ context.Context, _ *api.EmptyMessage) (*core.NodeInfo, error) {
			return nil, fmt.Errorf("node info unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetChainParameters(pool), map[string]any{})
	if !result.IsError {
		t.Error("expected error when GetNodeInfo fails")
	}
}

func TestGetEnergyPrices_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetEnergyPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "100:420"}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetEnergyPrices(pool), map[string]any{})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}

func TestGetEnergyPrices_Error(t *testing.T) {
	mock := &mockWalletServer{
		GetEnergyPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return nil, fmt.Errorf("energy prices unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetEnergyPrices(pool), map[string]any{})
	if !result.IsError {
		t.Error("expected error when GetEnergyPrices fails")
	}
}

func TestGetBandwidthPrices_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetBandwidthPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "200:1000"}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBandwidthPrices(pool), map[string]any{})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}

func TestGetBandwidthPrices_Error(t *testing.T) {
	mock := &mockWalletServer{
		GetBandwidthPricesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return nil, fmt.Errorf("bandwidth prices unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetBandwidthPrices(pool), map[string]any{})
	if !result.IsError {
		t.Error("expected error when GetBandwidthPrices fails")
	}
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
		if got != tt.want {
			t.Errorf("normalizeResult(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
