package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAccountPermissions_InvalidAddress(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, _ *core.Account) (*core.Account, error) {
			t.Fatal("RPC should not be called for invalid address")
			return nil, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetAccountPermissions(pool), map[string]any{
		"address": "invalid",
	})
	assert.True(t, result.IsError, "expected error for invalid address")
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
	assert.True(t, result.IsError, "expected error when GetAccount fails")
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
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", data["address"])

	// Owner permission
	owner, ok := data["owner"].(map[string]any)
	require.True(t, ok, "expected owner permission")
	assert.Equal(t, float64(1), owner["threshold"])
	assert.Equal(t, "owner", owner["name"])
	assert.Equal(t, "Owner", owner["type"])
	keys := owner["keys"].([]any)
	require.Len(t, keys, 1)
	key := keys[0].(map[string]any)
	assert.Equal(t, float64(1), key["weight"])
	assert.NotEmpty(t, key["address"])

	// Active permission
	active, ok := data["active"].([]any)
	require.True(t, ok)
	require.Len(t, active, 1)
	ap := active[0].(map[string]any)
	assert.Equal(t, float64(2), ap["threshold"])
	assert.Equal(t, "active", ap["name"])
	assert.Equal(t, "ff0f", ap["operations"])
	aKeys := ap["keys"].([]any)
	require.Len(t, aKeys, 1)
	assert.Equal(t, float64(1), aKeys[0].(map[string]any)["weight"])

	// No witness
	assert.Nil(t, data["witness"], "witness should not be present when nil")
}

func TestGetAccountPermissions_WithWitness(t *testing.T) {
	witnessAddr := []byte{0x41, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14}
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
			return &core.Account{
				Address: in.Address,
				OwnerPermission: &core.Permission{
					Id:        0,
					Threshold: 1,
					Type:      core.Permission_Owner,
					Keys:      []*core.Key{{Address: witnessAddr, Weight: 1}},
				},
				WitnessPermission: &core.Permission{
					Id:             1,
					PermissionName: "witness",
					Threshold:      1,
					Type:           core.Permission_Witness,
					Keys:           []*core.Key{{Address: witnessAddr, Weight: 3}},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetAccountPermissions(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	witness, ok := data["witness"].(map[string]any)
	require.True(t, ok, "expected witness permission")
	assert.Equal(t, "witness", witness["name"])
	assert.Equal(t, "Witness", witness["type"])
	assert.Equal(t, float64(1), witness["threshold"])
	keys := witness["keys"].([]any)
	require.Len(t, keys, 1)
	assert.Equal(t, float64(3), keys[0].(map[string]any)["weight"])
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
	require.False(t, result.IsError)

	data := parseJSONResult(t, result)
	assert.Nil(t, data["owner"])
	assert.Nil(t, data["witness"])
	assert.Nil(t, data["active"])
}

func TestGetDelegatedResources_InvalidAddress(t *testing.T) {
	mock := &mockWalletServer{
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			t.Fatal("RPC should not be called for invalid address")
			return nil, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetDelegatedResources(pool), map[string]any{
		"address": "bad",
	})
	assert.True(t, result.IsError, "expected error for invalid address")
}

func TestGetDelegatedResources_Success(t *testing.T) {
	fromAddr := []byte{0x41, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14}
	toAddr := []byte{0x41, 0x14, 0x13, 0x12, 0x11, 0x10, 0x0f, 0x0e, 0x0d, 0x0c, 0x0b, 0x0a, 0x09, 0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}

	mock := &mockWalletServer{
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			return &core.DelegatedResourceAccountIndex{
				Account:      fromAddr,
				ToAccounts:   [][]byte{toAddr},
				FromAccounts: [][]byte{toAddr},
			}, nil
		},
		GetDelegatedResourceV2Func: func(_ context.Context, _ *api.DelegatedResourceMessage) (*api.DelegatedResourceList, error) {
			return &api.DelegatedResourceList{
				DelegatedResource: []*core.DelegatedResource{
					{
						From:                      fromAddr,
						To:                        toAddr,
						FrozenBalanceForEnergy:    5000000,
						FrozenBalanceForBandwidth: 2000000,
						ExpireTimeForEnergy:       1710864000,
						ExpireTimeForBandwidth:    1710950400,
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetDelegatedResources(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)

	// delegated_to
	delegated := data["delegated_to"].([]any)
	require.Len(t, delegated, 1)
	d := delegated[0].(map[string]any)
	assert.Equal(t, float64(5000000), d["energy_sun"])
	assert.Equal(t, "5.000000", d["energy_trx"])
	assert.Equal(t, float64(2000000), d["bandwidth_sun"])
	assert.Equal(t, "2.000000", d["bandwidth_trx"])
	assert.Equal(t, float64(1710864000), d["expire_time_energy"])
	assert.Equal(t, float64(1710950400), d["expire_time_bandwidth"])
	assert.NotEmpty(t, d["from"])
	assert.NotEmpty(t, d["to"])

	// received_from
	received := data["received_from"].([]any)
	require.Len(t, received, 1)
	r := received[0].(map[string]any)
	assert.Equal(t, float64(5000000), r["energy_sun"])
	assert.Equal(t, float64(2000000), r["bandwidth_sun"])
}

func TestGetDelegatedResources_IndexError(t *testing.T) {
	mock := &mockWalletServer{
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			return nil, fmt.Errorf("node unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetDelegatedResources(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	assert.True(t, result.IsError, "expected error when index lookup fails")
}

func TestGetDelegatedResources_ResourceError(t *testing.T) {
	toAddr := []byte{0x41, 0x14, 0x13, 0x12, 0x11, 0x10, 0x0f, 0x0e, 0x0d, 0x0c, 0x0b, 0x0a, 0x09, 0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}
	callCount := 0
	mock := &mockWalletServer{
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			callCount++
			if callCount == 1 {
				return &core.DelegatedResourceAccountIndex{}, nil
			}
			return &core.DelegatedResourceAccountIndex{
				FromAccounts: [][]byte{toAddr},
			}, nil
		},
		GetDelegatedResourceV2Func: func(_ context.Context, _ *api.DelegatedResourceMessage) (*api.DelegatedResourceList, error) {
			return nil, fmt.Errorf("rpc error")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetDelegatedResources(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	assert.True(t, result.IsError, "expected error when resource lookup fails")
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
	require.False(t, result.IsError)

	data := parseJSONResult(t, result)
	assert.Len(t, data["delegated_to"].([]any), 0)
	assert.Len(t, data["received_from"].([]any), 0)
}

func TestGetDelegatableAmount_InvalidAddress(t *testing.T) {
	mock := &mockWalletServer{
		GetCanDelegatedMaxSizeFunc: func(_ context.Context, _ *api.CanDelegatedMaxSizeRequestMessage) (*api.CanDelegatedMaxSizeResponseMessage, error) {
			t.Fatal("RPC should not be called for invalid address")
			return nil, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetDelegatableAmount(pool), map[string]any{
		"address":  "bad",
		"resource": "ENERGY",
	})
	assert.True(t, result.IsError, "expected error for invalid address")
}

func TestGetDelegatableAmount_InvalidResource(t *testing.T) {
	mock := &mockWalletServer{
		GetCanDelegatedMaxSizeFunc: func(_ context.Context, _ *api.CanDelegatedMaxSizeRequestMessage) (*api.CanDelegatedMaxSizeResponseMessage, error) {
			t.Fatal("RPC should not be called for invalid resource")
			return nil, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleGetDelegatableAmount(pool), map[string]any{
		"address":  "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"resource": "INVALID",
	})
	assert.True(t, result.IsError, "expected error for invalid resource")
}

func TestGetDelegatableAmount_Success(t *testing.T) {
	tests := []struct {
		name     string
		resource string
	}{
		{"ENERGY", "ENERGY"},
		{"BANDWIDTH", "BANDWIDTH"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockWalletServer{
				GetCanDelegatedMaxSizeFunc: func(_ context.Context, in *api.CanDelegatedMaxSizeRequestMessage) (*api.CanDelegatedMaxSizeResponseMessage, error) {
					return &api.CanDelegatedMaxSizeResponseMessage{
						MaxSize: 5000000000,
					}, nil
				},
			}
			pool := newMockPool(t, mock)
			result := callTool(t, handleGetDelegatableAmount(pool), map[string]any{
				"address":  "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
				"resource": tt.resource,
			})
			require.False(t, result.IsError, "expected success, got error: %v", result.Content)

			data := parseJSONResult(t, result)
			assert.Equal(t, tt.resource, data["resource"])
			assert.Equal(t, float64(5000000000), data["max_delegatable_sun"])
			assert.Equal(t, "5000.000000", data["max_delegatable_trx"])
		})
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
	assert.True(t, result.IsError, "expected error when GetCanDelegatedMaxSize fails")
}
