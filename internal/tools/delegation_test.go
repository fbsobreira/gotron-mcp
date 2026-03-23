package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

func TestGetAccountPermissions_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetAccountPermissions(pool), map[string]any{
		"address": "invalid",
	})
	if !result.IsError {
		t.Error("expected error for invalid address")
	}
}

func TestGetAccountPermissions_Error(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, _ *core.Account) (*core.Account, error) {
			return nil, fmt.Errorf("node unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetAccountPermissions(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if !result.IsError {
		t.Error("expected error when GetAccount fails")
	}
}

func TestGetAccountPermissions_Success(t *testing.T) {
	ownerAddr := []byte{0x41, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14}
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
			return &core.Account{
				Address: in.Address,
				OwnerPermission: &core.Permission{
					Id:             0,
					PermissionName: "owner",
					Threshold:      1,
					Type:           core.Permission_Owner,
					Keys: []*core.Key{
						{Address: ownerAddr, Weight: 1},
					},
				},
				ActivePermission: []*core.Permission{
					{
						Id:             2,
						PermissionName: "active",
						Threshold:      2,
						Type:           core.Permission_Active,
						Operations:     []byte{0xff, 0x0f},
						Keys: []*core.Key{
							{Address: ownerAddr, Weight: 1},
						},
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetAccountPermissions(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["address"] != "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF" {
		t.Errorf("address = %v", data["address"])
	}

	owner, ok := data["owner"].(map[string]any)
	if !ok {
		t.Fatal("expected owner permission")
	}
	if owner["threshold"] != float64(1) {
		t.Errorf("owner threshold = %v, want 1", owner["threshold"])
	}

	active, ok := data["active"].([]any)
	if !ok || len(active) != 1 {
		t.Fatalf("expected 1 active permission, got %v", data["active"])
	}
	ap := active[0].(map[string]any)
	if ap["threshold"] != float64(2) {
		t.Errorf("active threshold = %v, want 2", ap["threshold"])
	}

	if _, ok := data["witness"]; ok {
		t.Error("witness should not be present when nil")
	}
}

func TestGetAccountPermissions_NoPermissions(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
			return &core.Account{Address: in.Address}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetAccountPermissions(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if _, ok := data["owner"]; ok {
		t.Error("owner should not be present when nil")
	}
}

func TestGetDelegatedResources_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetDelegatedResources(pool), map[string]any{
		"address": "bad",
	})
	if !result.IsError {
		t.Error("expected error for invalid address")
	}
}

func TestGetDelegatedResources_Success(t *testing.T) {
	fromAddr := []byte{0x41, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14}
	toAddr := []byte{0x41, 0x14, 0x13, 0x12, 0x11, 0x10, 0x0f, 0x0e, 0x0d, 0x0c, 0x0b, 0x0a, 0x09, 0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}

	mock := &mockWalletServer{
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			return &core.DelegatedResourceAccountIndex{
				Account:    fromAddr,
				ToAccounts: [][]byte{toAddr},
			}, nil
		},
		GetDelegatedResourceV2Func: func(_ context.Context, _ *api.DelegatedResourceMessage) (*api.DelegatedResourceList, error) {
			return &api.DelegatedResourceList{
				DelegatedResource: []*core.DelegatedResource{
					{
						From:                   fromAddr,
						To:                     toAddr,
						FrozenBalanceForEnergy: 5000000,
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetDelegatedResources(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	delegated := data["delegated_to"].([]any)
	if len(delegated) != 1 {
		t.Fatalf("expected 1 delegation, got %d", len(delegated))
	}
	d := delegated[0].(map[string]any)
	if d["energy"] != float64(5000000) {
		t.Errorf("energy = %v, want 5000000", d["energy"])
	}
}

func TestGetDelegatedResources_Empty(t *testing.T) {
	mock := &mockWalletServer{
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			return &core.DelegatedResourceAccountIndex{}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetDelegatedResources(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	delegated := data["delegated_to"].([]any)
	if len(delegated) != 0 {
		t.Errorf("expected empty delegated_to, got %d", len(delegated))
	}
	received := data["received_from"].([]any)
	if len(received) != 0 {
		t.Errorf("expected empty received_from, got %d", len(received))
	}
}

func TestGetDelegatableAmount_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetDelegatableAmount(pool), map[string]any{
		"address":  "bad",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for invalid address")
	}
}

func TestGetDelegatableAmount_InvalidResource(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleGetDelegatableAmount(pool), map[string]any{
		"address":  "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"resource": "INVALID",
	})
	if !result.IsError {
		t.Error("expected error for invalid resource")
	}
}

func TestGetDelegatableAmount_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetCanDelegatedMaxSizeFunc: func(_ context.Context, _ *api.CanDelegatedMaxSizeRequestMessage) (*api.CanDelegatedMaxSizeResponseMessage, error) {
			return &api.CanDelegatedMaxSizeResponseMessage{
				MaxSize: 5000000000,
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetDelegatableAmount(pool), map[string]any{
		"address":  "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"resource": "ENERGY",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["resource"] != "ENERGY" {
		t.Errorf("resource = %v, want ENERGY", data["resource"])
	}
	if data["max_delegatable_sun"] != float64(5000000000) {
		t.Errorf("max_delegatable_sun = %v, want 5000000000", data["max_delegatable_sun"])
	}
	if data["max_delegatable_trx"] != "5000.000000" {
		t.Errorf("max_delegatable_trx = %v, want 5000.000000", data["max_delegatable_trx"])
	}
}

func TestGetDelegatableAmount_Error(t *testing.T) {
	mock := &mockWalletServer{
		GetCanDelegatedMaxSizeFunc: func(_ context.Context, _ *api.CanDelegatedMaxSizeRequestMessage) (*api.CanDelegatedMaxSizeResponseMessage, error) {
			return nil, fmt.Errorf("node unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetDelegatableAmount(pool), map[string]any{
		"address":  "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"resource": "BANDWIDTH",
	})
	if !result.IsError {
		t.Error("expected error when GetCanDelegatedMaxSize fails")
	}
}
