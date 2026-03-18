package tools

import (
	"context"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetAccount_Success(t *testing.T) {
	addr, _ := address.Base58ToAddress("TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF")
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, _ *core.Account) (*core.Account, error) {
			return &core.Account{
				Address:     addr.Bytes(),
				Balance:     5_000_000,
				AccountName: []byte("testaccount"),
				CreateTime:  1700000000,
			}, nil
		},
	}
	pool := newMockPool(t, mock)

	result := callTool(t, handleGetAccount(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}

func TestGetAccount_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetAccount(pool), map[string]any{
		"address": "invalid",
	})
	if !result.IsError {
		t.Error("expected error for invalid address")
	}
}

func TestGetAccount_GRPCError(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, _ *core.Account) (*core.Account, error) {
			return nil, status.Error(codes.NotFound, "account not found")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetAccount(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if !result.IsError {
		t.Error("expected error for gRPC failure")
	}
}

func TestGetAccountResources_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{
				FreeNetUsed:  100,
				FreeNetLimit: 5000,
				EnergyUsed:   200,
				EnergyLimit:  10000,
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetAccountResources(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}

func TestGetAccountResources_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetAccountResources(pool), map[string]any{
		"address": "",
	})
	if !result.IsError {
		t.Error("expected error for empty address")
	}
}
