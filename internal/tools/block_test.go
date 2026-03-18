package tools

import (
	"context"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
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
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
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
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}
