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

func TestAnalyzeAccount_InvalidAddress(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, _ *core.Account) (*core.Account, error) {
			t.Fatal("RPC should not be called for invalid address")
			return nil, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeAccount(pool), map[string]any{
		"address": "invalid",
	})
	assert.True(t, result.IsError)
}

func TestAnalyzeAccount_AccountError(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, _ *core.Account) (*core.Account, error) {
			return nil, fmt.Errorf("node unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeAccount(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	assert.True(t, result.IsError)
}

func TestAnalyzeAccount_ResourceError(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
			return &core.Account{Address: in.Address, Balance: 1000000}, nil
		},
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return nil, fmt.Errorf("resource fetch failed")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeAccount(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	assert.True(t, result.IsError)
}

func TestAnalyzeAccount_BasicAccount(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
			return &core.Account{
				Address: in.Address,
				Balance: 12450500000,
			}, nil
		},
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{
				EnergyUsed:   1000,
				EnergyLimit:  50000,
				NetUsed:      200,
				NetLimit:     2400,
				FreeNetUsed:  100,
				FreeNetLimit: 600,
			}, nil
		},
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			return &core.DelegatedResourceAccountIndex{}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeAccount(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", data["address"])
	assert.Equal(t, "12450.500000", data["balance_trx"])
	assert.Equal(t, float64(12450500000), data["balance_sun"])
	assert.Equal(t, "Normal", data["account_type"])

	// Resources
	res := data["resources"].(map[string]any)
	assert.Equal(t, float64(1000), res["energy_used"])
	assert.Equal(t, float64(50000), res["energy_limit"])
	assert.Equal(t, float64(200), res["bandwidth_used"])
	assert.Equal(t, float64(2400), res["bandwidth_limit"])

	// No staking, voting, delegations, or permissions
	assert.Nil(t, data["staking"])
	assert.Nil(t, data["voting"])
	assert.Nil(t, data["delegations"])
	assert.Nil(t, data["permissions"])
}

func TestAnalyzeAccount_FullAccount(t *testing.T) {
	ownerAddr := []byte{0x41, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14}
	srAddr := []byte{0x41, 0x14, 0x13, 0x12, 0x11, 0x10, 0x0f, 0x0e, 0x0d, 0x0c, 0x0b, 0x0a, 0x09, 0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}

	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
			return &core.Account{
				Address:     in.Address,
				AccountName: []byte("my-wallet"),
				Balance:     5000000000,
				FrozenV2: []*core.Account_FreezeV2{
					{Type: core.ResourceCode_ENERGY, Amount: 3000000000},
					{Type: core.ResourceCode_BANDWIDTH, Amount: 1000000000},
				},
				UnfrozenV2: []*core.Account_UnFreezeV2{
					{Type: core.ResourceCode_ENERGY, UnfreezeAmount: 500000000, UnfreezeExpireTime: 1710864000},
				},
				Votes: []*core.Vote{
					{VoteAddress: srAddr, VoteCount: 4000},
				},
				OwnerPermission: &core.Permission{
					Threshold: 1,
					Keys:      []*core.Key{{Address: ownerAddr, Weight: 1}},
				},
				ActivePermission: []*core.Permission{
					{
						Id:             2,
						PermissionName: "active",
						Threshold:      2,
						Keys: []*core.Key{
							{Address: ownerAddr, Weight: 1},
							{Address: srAddr, Weight: 1},
						},
					},
				},
			}, nil
		},
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{
				EnergyLimit: 500000,
				NetLimit:    2400,
			}, nil
		},
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			return &core.DelegatedResourceAccountIndex{
				ToAccounts:   [][]byte{srAddr},
				FromAccounts: [][]byte{srAddr},
			}, nil
		},
		GetDelegatedResourceV2Func: func(_ context.Context, _ *api.DelegatedResourceMessage) (*api.DelegatedResourceList, error) {
			return &api.DelegatedResourceList{
				DelegatedResource: []*core.DelegatedResource{
					{
						From:                   ownerAddr,
						To:                     srAddr,
						FrozenBalanceForEnergy: 1000000000,
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeAccount(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "my-wallet", data["account_name"])

	// Staking
	staking := data["staking"].(map[string]any)
	assert.Equal(t, float64(3000000000), staking["frozen_energy_sun"])
	assert.Equal(t, float64(1000000000), staking["frozen_bandwidth_sun"])
	assert.Equal(t, float64(4000), staking["tron_power"]) // (3B + 1B) / 1M = 4000
	assert.Equal(t, float64(500000000), staking["pending_unfreeze_sun"])
	assert.Equal(t, float64(1), staking["pending_unfreeze_count"])

	// Voting
	voting := data["voting"].(map[string]any)
	assert.Equal(t, float64(4000), voting["total_votes"])
	votedFor := voting["voted_for"].([]any)
	require.Len(t, votedFor, 1)
	assert.Equal(t, float64(4000), votedFor[0].(map[string]any)["votes"])

	// Delegations
	delegations := data["delegations"].(map[string]any)
	assert.Equal(t, float64(1000000000), delegations["energy_delegated_out_sun"])
	assert.Equal(t, float64(1000000000), delegations["energy_received_sun"])

	// Permissions
	perms := data["permissions"].(map[string]any)
	assert.Equal(t, true, perms["is_multi_sig"])

	owner := perms["owner"].(map[string]any)
	assert.Equal(t, float64(1), owner["threshold"])
	ownerKeys := owner["keys"].([]any)
	require.Len(t, ownerKeys, 1)

	activePerms := perms["active_permissions"].([]any)
	require.Len(t, activePerms, 1)
	ap := activePerms[0].(map[string]any)
	assert.Equal(t, float64(2), ap["threshold"])
	assert.Equal(t, "active", ap["name"])
}

func TestAnalyzeAccount_WitnessAccount(t *testing.T) {
	ownerKey := []byte{0x41, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14}
	witnessKey := []byte{0x41, 0x14, 0x13, 0x12, 0x11, 0x10, 0x0f, 0x0e, 0x0d, 0x0c, 0x0b, 0x0a, 0x09, 0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}

	// Capture the account address from GetAccount to use in ListWitnesses
	var capturedAddr []byte
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
			capturedAddr = in.Address
			return &core.Account{
				Address:     in.Address,
				AccountName: []byte("CryptoChain"),
				Balance:     68658963,
				IsWitness:   true,
				OwnerPermission: &core.Permission{
					Threshold: 1,
					Keys:      []*core.Key{{Address: ownerKey, Weight: 1}},
				},
				WitnessPermission: &core.Permission{
					Threshold: 1,
					Keys:      []*core.Key{{Address: witnessKey, Weight: 1}},
				},
			}, nil
		},
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{}, nil
		},
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			return &core.DelegatedResourceAccountIndex{}, nil
		},
		ListWitnessesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.WitnessList, error) {
			return &api.WitnessList{
				Witnesses: []*core.Witness{
					{
						Address:        capturedAddr,
						VoteCount:      5000000,
						Url:            "https://cryptochain.network",
						TotalProduced:  120000,
						TotalMissed:    50,
						LatestBlockNum: 81000000,
						IsJobs:         true,
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeAccount(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["is_witness"])
	assert.Equal(t, "CryptoChain", data["account_name"])

	// Witness info
	witness := data["witness"].(map[string]any)
	assert.Equal(t, float64(5000000), witness["total_votes"])
	assert.Equal(t, float64(120000), witness["total_produced"])
	assert.Equal(t, float64(50), witness["total_missed"])
	assert.Equal(t, true, witness["is_active"])
	assert.Equal(t, "https://cryptochain.network", witness["url"])
	assert.Equal(t, float64(81000000), witness["latest_block_num"])

	// Permissions — witness key differs from owner key
	perms := data["permissions"].(map[string]any)
	assert.Equal(t, true, perms["witness_key_differs"])
}

func TestAnalyzeAccount_WitnessListErrorIsNonFatal(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
			return &core.Account{
				Address:   in.Address,
				IsWitness: true,
				Balance:   1000000,
			}, nil
		},
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{}, nil
		},
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			return &core.DelegatedResourceAccountIndex{}, nil
		},
		ListWitnessesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.WitnessList, error) {
			return nil, fmt.Errorf("witness RPC unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeAccount(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError, "witness error should be non-fatal")

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["is_witness"])
	assert.Nil(t, data["witness"]) // no witness section when RPC fails
}

func TestAnalyzeAccount_OwnerMultiSig(t *testing.T) {
	addr1 := []byte{0x41, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14}
	addr2 := []byte{0x41, 0x14, 0x13, 0x12, 0x11, 0x10, 0x0f, 0x0e, 0x0d, 0x0c, 0x0b, 0x0a, 0x09, 0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
			return &core.Account{
				Address: in.Address,
				OwnerPermission: &core.Permission{
					Threshold: 2,
					Keys: []*core.Key{
						{Address: addr1, Weight: 1},
						{Address: addr2, Weight: 1},
					},
				},
			}, nil
		},
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{}, nil
		},
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			return &core.DelegatedResourceAccountIndex{}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeAccount(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	require.False(t, result.IsError)

	data := parseJSONResult(t, result)
	perms := data["permissions"].(map[string]any)
	assert.Equal(t, true, perms["is_multi_sig"], "owner with threshold > 1 should be multi-sig")

	owner := perms["owner"].(map[string]any)
	assert.Equal(t, float64(2), owner["threshold"])
	ownerKeys := owner["keys"].([]any)
	require.Len(t, ownerKeys, 2)
}

func TestAnalyzeAccount_NilDelegationList(t *testing.T) {
	// Test that nil elements in delegation list slices don't panic
	result := buildDelegationSummary(
		[]*api.DelegatedResourceList{nil, nil},
		[]*api.DelegatedResourceList{nil},
	)
	assert.Empty(t, result)
}

func TestAnalyzeAccount_DelegationErrorIsNonFatal(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
			return &core.Account{Address: in.Address, Balance: 1000000}, nil
		},
		GetAccountResourceFunc: func(_ context.Context, _ *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{}, nil
		},
		GetDelegatedResourceAccountIndexV2Func: func(_ context.Context, _ *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
			return nil, fmt.Errorf("delegation RPC unavailable")
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleAnalyzeAccount(pool), map[string]any{
		"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	// Delegation errors are non-fatal — tool should still return account data
	require.False(t, result.IsError, "delegation error should be non-fatal")

	data := parseJSONResult(t, result)
	assert.Equal(t, "1.000000", data["balance_trx"])
	assert.Nil(t, data["delegations"])
}
