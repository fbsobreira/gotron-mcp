package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/approval"
	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/policy"
	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func newTestSignSetup(t *testing.T, mock *mockWalletServer) (*wallet.Manager, *nodepool.Pool) {
	t.Helper()
	wm, err := wallet.NewManager(t.TempDir(), "test-pass")
	require.NoError(t, err)
	wm.SetKeystoreFactory(keystore.ForPathLight)
	t.Cleanup(func() { wm.Close() })
	pool := newMockPool(t, mock)
	return wm, pool
}

func buildTestTxHex(t *testing.T) string {
	t.Helper()
	tx := &core.Transaction{
		RawData: &core.TransactionRaw{
			Contract: []*core.Transaction_Contract{{}},
		},
	}
	txBytes, err := proto.Marshal(tx)
	require.NoError(t, err, "failed to marshal test tx")
	return hex.EncodeToString(txBytes)
}

// --- sign_transaction tests ---

func TestSignTransaction_EmptyHex(t *testing.T) {
	wm, _ := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(wm), map[string]any{
		"transaction_hex": "",
		"wallet":          "test",
	})
	assert.True(t, result.IsError, "expected error for empty transaction_hex")
}

func TestSignTransaction_TooLong(t *testing.T) {
	wm, _ := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(wm), map[string]any{
		"transaction_hex": strings.Repeat("aa", maxHexInputLen+1),
		"wallet":          "test",
	})
	assert.True(t, result.IsError, "expected error for too-long transaction_hex")
}

func TestSignTransaction_EmptyWallet(t *testing.T) {
	wm, _ := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(wm), map[string]any{
		"transaction_hex": "0a0208",
		"wallet":          "",
	})
	assert.True(t, result.IsError, "expected error for empty wallet")
}

func TestSignTransaction_WalletNotFound(t *testing.T) {
	wm, _ := newTestSignSetup(t, &mockWalletServer{})
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignTransaction(wm), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "nonexistent",
	})
	assert.True(t, result.IsError, "expected error for wallet not found")
	found := false
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			if strings.Contains(tc.Text, "sign_transaction:") {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "expected error text containing 'sign_transaction:', got: %+v", result.Content)
}

func TestSignTransaction_InvalidHex(t *testing.T) {
	wm, _ := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(wm), map[string]any{
		"transaction_hex": "not-hex",
		"wallet":          "test",
	})
	assert.True(t, result.IsError, "expected error for invalid hex")
}

func TestSignTransaction_Success(t *testing.T) {
	t.Run("by name", func(t *testing.T) {
		wm, _ := newTestSignSetup(t, &mockWalletServer{})

		// Create a wallet
		_, err := wm.CreateWallet("test-signer")
		require.NoError(t, err, "CreateWallet")

		txHex := buildTestTxHex(t)
		result := callTool(t, handleSignTransaction(wm), map[string]any{
			"transaction_hex": txHex,
			"wallet":          "test-signer",
		})
		require.False(t, result.IsError, "expected success, got error: %v", result.Content)

		data := parseJSONResult(t, result)
		signedHex, ok := data["signed_transaction_hex"].(string)
		assert.True(t, ok && signedHex != "", "signed_transaction_hex should be a non-empty string, got %v", data["signed_transaction_hex"])
		assert.Equal(t, "test-signer", data["wallet"])
	})

	t.Run("by address", func(t *testing.T) {
		wm, _ := newTestSignSetup(t, &mockWalletServer{})

		// Create a wallet and use the returned address to sign
		addr, err := wm.CreateWallet("addr-signer")
		require.NoError(t, err, "CreateWallet")

		txHex := buildTestTxHex(t)
		result := callTool(t, handleSignTransaction(wm), map[string]any{
			"transaction_hex": txHex,
			"wallet":          addr,
		})
		require.False(t, result.IsError, "expected success, got error: %v", result.Content)

		data := parseJSONResult(t, result)
		signedHex, ok := data["signed_transaction_hex"].(string)
		assert.True(t, ok && signedHex != "", "signed_transaction_hex should be a non-empty string, got %v", data["signed_transaction_hex"])
		assert.Equal(t, addr, data["wallet"])
	})
}

// --- sign_and_broadcast tests ---

func TestSignAndBroadcast_InvalidHex(t *testing.T) {
	wm, pool := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignAndBroadcast(pool, wm, nil), map[string]any{
		"transaction_hex": "not-hex",
		"wallet":          "test",
	})
	assert.True(t, result.IsError, "expected error for invalid hex")
}

func TestSignAndBroadcast_WalletNotFound(t *testing.T) {
	wm, pool := newTestSignSetup(t, &mockWalletServer{})
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndBroadcast(pool, wm, nil), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "nonexistent",
	})
	assert.True(t, result.IsError, "expected error for wallet not found")
}

func TestSignAndBroadcast_Success(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{
				Result:  true,
				Code:    api.Return_SUCCESS,
				Message: []byte("ok"),
			}, nil
		},
	}
	wm, pool := newTestSignSetup(t, mock)

	_, err := wm.CreateWallet("broadcast-signer")
	require.NoError(t, err, "CreateWallet")

	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndBroadcast(pool, wm, nil), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "broadcast-signer",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["success"])
	txid, ok := data["txid"].(string)
	assert.True(t, ok && txid != "", "txid should be a non-empty string")
}

// --- sign_and_confirm tests ---

func TestSignAndConfirm_Success(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{
				Result:  true,
				Code:    api.Return_SUCCESS,
				Message: []byte("ok"),
			}, nil
		},
		GetTransactionInfoByIdFunc: func(_ context.Context, msg *api.BytesMessage) (*core.TransactionInfo, error) {
			return &core.TransactionInfo{
				Id:          msg.Value,
				BlockNumber: 12345678,
				Fee:         100000,
				Receipt: &core.ResourceReceipt{
					EnergyUsageTotal: 50000,
					NetUsage:         300,
				},
			}, nil
		},
	}
	wm, pool := newTestSignSetup(t, mock)

	_, err := wm.CreateWallet("confirm-signer")
	require.NoError(t, err, "CreateWallet")

	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndConfirm(pool, wm, nil), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "confirm-signer",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["success"])
	assert.Equal(t, true, data["confirmed"])
	assert.Equal(t, float64(12345678), data["block_number"].(float64))
	txid, ok := data["txid"].(string)
	assert.True(t, ok && txid != "", "txid should be a non-empty string")
}

// --- broadcast_transaction tests (kept as-is) ---

func TestBroadcastTransaction_EmptyHex(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleBroadcastTransaction(pool), map[string]any{
		"signed_transaction_hex": "",
	})
	assert.True(t, result.IsError, "expected error for empty hex")
}

func TestBroadcastTransaction_TooLong(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleBroadcastTransaction(pool), map[string]any{
		"signed_transaction_hex": strings.Repeat("aa", maxHexInputLen+1),
	})
	assert.True(t, result.IsError, "expected error for too-long hex")
}

func TestBroadcastTransaction_InvalidHex(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleBroadcastTransaction(pool), map[string]any{
		"signed_transaction_hex": "zzzz",
	})
	assert.True(t, result.IsError, "expected error for invalid hex")
}

func TestBroadcastTransaction_InvalidProto(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleBroadcastTransaction(pool), map[string]any{
		"signed_transaction_hex": "0f",
	})
	require.True(t, result.IsError, "expected error for invalid protobuf payload")
	// Verify the error is from proto parse or broadcast — either way it's an error
	found := false
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			if strings.Contains(tc.Text, "failed to parse transaction") || strings.Contains(tc.Text, "broadcast_transaction") {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "expected parse or broadcast error, got: %+v", result.Content)
}

func TestSignAndBroadcast_BroadcastFails(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return nil, fmt.Errorf("node unavailable")
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("test-wallet")
	require.NoError(t, err, "CreateWallet")
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndBroadcast(pool, wm, nil), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "test-wallet",
	})
	assert.True(t, result.IsError, "expected error when broadcast fails")
}

func TestSignAndBroadcast_BroadcastRejected(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{
				Result:  false,
				Code:    api.Return_BANDWITH_ERROR,
				Message: []byte("not enough bandwidth"),
			}, fmt.Errorf("result error: not enough bandwidth")
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("test-wallet")
	require.NoError(t, err, "CreateWallet")
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndBroadcast(pool, wm, nil), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "test-wallet",
	})
	// SDK BroadcastCtx returns error when Result is false
	assert.True(t, result.IsError, "expected error when broadcast is rejected")
}

func TestSignAndConfirm_ContextCancelled(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{Result: true, Code: api.Return_SUCCESS}, nil
		},
		GetTransactionInfoByIdFunc: func(_ context.Context, _ *api.BytesMessage) (*core.TransactionInfo, error) {
			return nil, fmt.Errorf("transaction not found")
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("test-wallet")
	require.NoError(t, err, "CreateWallet")
	txHex := buildTestTxHex(t)

	// Use a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"transaction_hex": txHex,
		"wallet":          "test-wallet",
	}
	handler := handleSignAndConfirm(pool, wm, nil)
	result, goErr := handler(ctx, req)
	require.NoError(t, goErr, "handler returned Go error")
	assert.True(t, result.IsError, "expected error for cancelled context")
}

func TestSignAndConfirm_RPCError(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{Result: true, Code: api.Return_SUCCESS}, nil
		},
		GetTransactionInfoByIdFunc: func(_ context.Context, _ *api.BytesMessage) (*core.TransactionInfo, error) {
			return nil, fmt.Errorf("rpc connection refused")
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("test-wallet")
	require.NoError(t, err, "CreateWallet")
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndConfirm(pool, wm, nil), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "test-wallet",
	})
	assert.True(t, result.IsError, "expected error for RPC failure")
}

func TestBroadcastTransaction_Success(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{
				Result:  true,
				Code:    api.Return_SUCCESS,
				Message: []byte("ok"),
			}, nil
		},
	}
	pool := newMockPool(t, mock)

	tx := &core.Transaction{
		RawData: &core.TransactionRaw{
			Timestamp: 1700000000,
		},
	}
	txBytes, err := proto.Marshal(tx)
	require.NoError(t, err, "failed to marshal tx")

	result := callTool(t, handleBroadcastTransaction(pool), map[string]any{
		"signed_transaction_hex": hex.EncodeToString(txBytes),
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["success"])
	assert.NotEmpty(t, data["transaction_id"], "transaction_id should not be empty")
}

// --- get_wallet_policy tests ---

func TestGetWalletPolicy_NoEngine(t *testing.T) {
	handler := handleGetWalletPolicy(nil, nil)
	result := callTool(t, handler, map[string]any{
		"wallet": "my-wallet",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "my-wallet", data["wallet"])
	assert.Equal(t, false, data["policy_enabled"])
	assert.Contains(t, data["message"], "No policy engine configured")
}

func TestGetWalletPolicy_NoPolicyForWallet(t *testing.T) {
	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{},
	}
	store, err := policy.NewStore(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	pe := policy.NewEngine(cfg, store)
	handler := handleGetWalletPolicy(pe, nil)
	result := callTool(t, handler, map[string]any{
		"wallet": "unknown-wallet",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "unknown-wallet", data["wallet"])
	assert.Equal(t, true, data["policy_enabled"])
	assert.Equal(t, false, data["has_policy"])
	assert.Contains(t, data["message"], "No policy configured for this wallet")
}

func TestGetWalletPolicy_WithPolicy(t *testing.T) {
	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"hot-wallet": {
				PerTxLimitTRX: 100,
				Whitelist:     []string{"TXyz1234567890abcdefghijklmnopqrst"},
				TokenLimits: map[string]*policy.TokenLimit{
					"USDT_CONTRACT": {
						Decimals:        6,
						PerTxLimitUnits: 50,
						DailyLimitUnits: 500,
					},
				},
			},
		},
	}
	store, err := policy.NewStore(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	pe := policy.NewEngine(cfg, store)
	handler := handleGetWalletPolicy(pe, nil)
	result := callTool(t, handler, map[string]any{
		"wallet": "hot-wallet",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "hot-wallet", data["wallet"])
	assert.Equal(t, true, data["policy_enabled"])
	assert.Equal(t, true, data["has_policy"])
	assert.Equal(t, float64(100), data["per_tx_limit_trx"])

	whitelist, ok := data["whitelist"].([]any)
	require.True(t, ok, "expected whitelist to be a list")
	assert.Contains(t, whitelist, "TXyz1234567890abcdefghijklmnopqrst")

	tokenLimits, ok := data["token_limits"].(map[string]any)
	require.True(t, ok, "expected token_limits to be a map")
	usdtLimit, ok := tokenLimits["USDT_CONTRACT"].(map[string]any)
	require.True(t, ok, "expected USDT_CONTRACT entry in token_limits")
	assert.Equal(t, float64(50), usdtLimit["per_tx_limit_units"])
	assert.Equal(t, float64(500), usdtLimit["daily_limit_units"])
}

// --- Policy denied tests ---

// buildTransferTxHex builds a valid TransferContract transaction hex for policy tests.
func buildTransferTxHex(t *testing.T, from, to string, amountSUN int64) string {
	t.Helper()
	transfer := &core.TransferContract{
		OwnerAddress: mustDecodeAddr(from),
		ToAddress:    mustDecodeAddr(to),
		Amount:       amountSUN,
	}
	paramAny, err := anypb.New(transfer)
	require.NoError(t, err, "failed to create Any")
	tx := &core.Transaction{
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
			Expiration: time.Now().Add(time.Minute).UnixMilli(),
		},
	}
	txBytes, err := proto.Marshal(tx)
	require.NoError(t, err, "failed to marshal transfer tx")
	return hex.EncodeToString(txBytes)
}

func newTestPolicyEngine(t *testing.T, cfg *policy.Config) *policy.Engine {
	t.Helper()
	store, err := policy.NewStore(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return policy.NewEngine(cfg, store)
}

func TestSignAndBroadcast_PolicyDenied_ReturnsHint(t *testing.T) {
	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"hot-wallet": {
				TokenLimits: map[string]*policy.TokenLimit{
					"TRX": {Decimals: 6, PerTxLimitUnits: 1}, // 1 TRX per-tx limit
				},
			},
		},
	}
	pe := newTestPolicyEngine(t, cfg)

	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("hot-wallet")
	require.NoError(t, err)

	// 5 TRX exceeds the 1 TRX per-tx limit
	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		5_000_000,
	)

	result := callTool(t, handleSignAndBroadcast(pool, wm, pe), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "hot-wallet",
	})
	require.False(t, result.IsError, "expected JSON result, not tool error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "policy_denied", data["status"], "expected policy_denied status")
	hint, ok := data["hint"].(string)
	assert.True(t, ok, "expected hint field to be a string")
	assert.Contains(t, hint, "request_limit_override", "hint should mention request_limit_override")
}

// mockApprover is a simple approval.Approver for testing.
type mockApprover struct {
	approved bool
}

func (m *mockApprover) RequestApproval(_ context.Context, _ approval.Request) (approval.Result, error) {
	return approval.Result{Approved: m.approved}, nil
}

func TestGetWalletPolicy_ApproverConfigured(t *testing.T) {
	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"test-wallet": {
				TokenLimits: map[string]*policy.TokenLimit{
					"TRX": {Decimals: 6, PerTxLimitUnits: 10},
				},
			},
		},
	}
	pe := newTestPolicyEngine(t, cfg)
	pe.SetApprover(&mockApprover{approved: true})

	handler := handleGetWalletPolicy(pe, nil)
	result := callTool(t, handler, map[string]any{
		"wallet": "test-wallet",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["approver_configured"], "approver_configured should be true")
}

// --- request_limit_override tests ---

func TestRequestLimitOverride_MissingReason(t *testing.T) {
	cfg := &policy.Config{Enabled: true, Wallets: map[string]*policy.WalletPolicy{
		"test": {TokenLimits: map[string]*policy.TokenLimit{
			"TRX": {Decimals: 6, PerTxLimitUnits: 1},
		}},
	}}
	pe := newTestPolicyEngine(t, cfg)

	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("test")
	require.NoError(t, err)

	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		5_000_000,
	)

	result := callTool(t, handleRequestLimitOverride(pool, wm, pe), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "test",
		"reason":          "",
	})
	assert.True(t, result.IsError, "expected error for missing reason")
}

func TestRequestLimitOverride_InvalidTx(t *testing.T) {
	cfg := &policy.Config{Enabled: true, Wallets: map[string]*policy.WalletPolicy{
		"test": {TokenLimits: map[string]*policy.TokenLimit{
			"TRX": {Decimals: 6, PerTxLimitUnits: 1},
		}},
	}}
	pe := newTestPolicyEngine(t, cfg)

	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("test")
	require.NoError(t, err)

	result := callTool(t, handleRequestLimitOverride(pool, wm, pe), map[string]any{
		"transaction_hex": "not-valid-hex",
		"wallet":          "test",
		"reason":          "emergency payout",
	})
	assert.True(t, result.IsError, "expected error for invalid transaction hex")
}

func TestRequestLimitOverride_MissingWallet(t *testing.T) {
	cfg := &policy.Config{Enabled: true, Wallets: map[string]*policy.WalletPolicy{
		"test": {TokenLimits: map[string]*policy.TokenLimit{
			"TRX": {Decimals: 6, PerTxLimitUnits: 1},
		}},
	}}
	pe := newTestPolicyEngine(t, cfg)

	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)

	result := callTool(t, handleRequestLimitOverride(pool, wm, pe), map[string]any{
		"transaction_hex": "aabb",
		"wallet":          "",
		"reason":          "emergency payout",
	})
	assert.True(t, result.IsError, "expected error for missing wallet")
}

// --- Approval flow tests ---

// buildTransferTxExt creates a mock TransactionExtention for CreateTransaction2.
func buildTransferTxExt(t *testing.T) *api.TransactionExtention {
	t.Helper()
	tx := &core.Transaction{
		RawData: &core.TransactionRaw{
			Contract: []*core.Transaction_Contract{
				{
					Type:      core.Transaction_Contract_TransferContract,
					Parameter: &anypb.Any{},
				},
			},
			Expiration: time.Now().Add(time.Minute).UnixMilli(),
		},
	}
	return &api.TransactionExtention{
		Transaction: tx,
		Result:      &api.Return{Result: true, Code: api.Return_SUCCESS},
	}
}

func TestSignAndBroadcast_ApprovalFlow_Approved(t *testing.T) {
	mock := &mockWalletServer{
		CreateTransaction2Func: func(_ context.Context, tc *core.TransferContract) (*api.TransactionExtention, error) {
			// Verify rebuilt TX has correct fields from intent
			require.NotNil(t, tc)
			assert.Equal(t, int64(10_000_000), tc.Amount, "rebuilt TX should preserve original amount")
			assert.NotEmpty(t, tc.OwnerAddress, "rebuilt TX should have from address")
			assert.NotEmpty(t, tc.ToAddress, "rebuilt TX should have to address")
			return buildTransferTxExt(t), nil
		},
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{Result: true, Code: api.Return_SUCCESS, Message: []byte("ok")}, nil
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("hot-wallet")
	require.NoError(t, err)

	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"hot-wallet": {
				ApprovalRequiredAboveTRX: 5,
			},
		},
	}
	pe := newTestPolicyEngine(t, cfg)
	pe.SetApprover(&mockApprover{approved: true})

	// 10 TRX exceeds the 5 TRX approval threshold
	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		10_000_000,
	)

	result := callTool(t, handleSignAndBroadcast(pool, wm, pe), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "hot-wallet",
		"reason":          "test approval flow",
	})
	// Should get past approval + rebuild + sign + broadcast
	require.False(t, result.IsError, "expected success after approval, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["success"], "expected broadcast success")
	txid, ok := data["txid"].(string)
	assert.True(t, ok && txid != "", "expected non-empty txid")
}

func TestSignAndBroadcast_ApprovalFlow_Rejected(t *testing.T) {
	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("hot-wallet")
	require.NoError(t, err)

	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"hot-wallet": {
				ApprovalRequiredAboveTRX: 5,
			},
		},
	}
	pe := newTestPolicyEngine(t, cfg)
	pe.SetApprover(&mockApprover{approved: false})

	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		10_000_000,
	)

	result := callTool(t, handleSignAndBroadcast(pool, wm, pe), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "hot-wallet",
		"reason":          "test rejection",
	})
	require.False(t, result.IsError, "expected JSON result, not tool error")

	data := parseJSONResult(t, result)
	assert.Equal(t, "approval_rejected", data["status"], "expected approval_rejected status")
}

func TestSignAndBroadcast_NoApprover_ReturnsHint(t *testing.T) {
	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("hot-wallet")
	require.NoError(t, err)

	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"hot-wallet": {
				ApprovalRequiredAboveTRX: 5,
			},
		},
	}
	pe := newTestPolicyEngine(t, cfg)
	// No approver set

	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		10_000_000,
	)

	result := callTool(t, handleSignAndBroadcast(pool, wm, pe), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "hot-wallet",
	})
	require.False(t, result.IsError, "expected JSON result, not tool error")

	data := parseJSONResult(t, result)
	assert.Equal(t, "approval_required", data["status"], "expected approval_required status")
	hint, ok := data["hint"].(string)
	assert.True(t, ok, "expected hint field to be a string")
	assert.Contains(t, hint, "No approval backend configured")
}

func TestSignAndConfirm_PolicyDenied_ReturnsHint(t *testing.T) {
	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"hot-wallet": {
				TokenLimits: map[string]*policy.TokenLimit{
					"TRX": {Decimals: 6, PerTxLimitUnits: 1}, // 1 TRX per-tx limit
				},
			},
		},
	}
	pe := newTestPolicyEngine(t, cfg)

	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("hot-wallet")
	require.NoError(t, err)

	// 5 TRX exceeds the 1 TRX per-tx limit
	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		5_000_000,
	)

	result := callTool(t, handleSignAndConfirm(pool, wm, pe), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "hot-wallet",
	})
	require.False(t, result.IsError, "expected JSON result, not tool error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "policy_denied", data["status"], "expected policy_denied status")
	hint, ok := data["hint"].(string)
	assert.True(t, ok, "expected hint field to be a string")
	assert.Contains(t, hint, "request_limit_override", "hint should mention request_limit_override")
}

func TestRequestLimitOverride_WhitelistDenied(t *testing.T) {
	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"hot-wallet": {
				TokenLimits: map[string]*policy.TokenLimit{
					"TRX": {Decimals: 6, PerTxLimitUnits: 1},
				},
				Whitelist: []string{"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"}, // only this address whitelisted
			},
		},
	}
	pe := newTestPolicyEngine(t, cfg)
	pe.SetApprover(&mockApprover{approved: true})

	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("hot-wallet")
	require.NoError(t, err)

	// Send to a non-whitelisted address
	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", // not in whitelist
		5_000_000,
	)

	result := callTool(t, handleRequestLimitOverride(pool, wm, pe), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "hot-wallet",
		"reason":          "emergency payout",
	})
	assert.True(t, result.IsError, "expected error for whitelist denial")
	found := false
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			if strings.Contains(tc.Text, "not in the whitelist") {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "expected whitelist denial error, got: %+v", result.Content)
}

func TestRequestLimitOverride_NoApprover(t *testing.T) {
	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"hot-wallet": {
				TokenLimits: map[string]*policy.TokenLimit{
					"TRX": {Decimals: 6, PerTxLimitUnits: 1},
				},
			},
		},
	}
	pe := newTestPolicyEngine(t, cfg)
	// No approver set

	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("hot-wallet")
	require.NoError(t, err)

	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		5_000_000,
	)

	result := callTool(t, handleRequestLimitOverride(pool, wm, pe), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "hot-wallet",
		"reason":          "emergency payout",
	})
	assert.True(t, result.IsError, "expected error when no approver configured")
	found := false
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			if strings.Contains(tc.Text, "no approval backend configured") {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "expected no-approver error, got: %+v", result.Content)
}

func TestGetWalletPolicy_WithRemainingBudget(t *testing.T) {
	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"hot-wallet": {
				TokenLimits: map[string]*policy.TokenLimit{
					"TRX": {Decimals: 6, PerTxLimitUnits: 100, DailyLimitUnits: 500},
				},
			},
		},
	}
	pe := newTestPolicyEngine(t, cfg)

	// Spend 50 TRX (50_000_000 SUN) via Check to reserve some budget
	intent := &policy.Intent{
		WalletName:  "hot-wallet",
		Action:      "TransferContract",
		FromAddr:    "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		ToAddr:      "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		AmountSUN:   50_000_000,
		TokenID:     "TRX",
		TokenAmount: 50_000_000,
	}
	result, err := pe.Check(context.Background(), intent)
	require.NoError(t, err)
	require.True(t, result.Allowed, "expected check to pass for 50 TRX within limits")

	handler := handleGetWalletPolicy(pe, nil)
	toolResult := callTool(t, handler, map[string]any{
		"wallet": "hot-wallet",
	})
	require.False(t, toolResult.IsError, "expected success, got error: %v", toolResult.Content)

	data := parseJSONResult(t, toolResult)
	remaining, ok := data["remaining_today"].(map[string]any)
	require.True(t, ok, "expected remaining_today to be a map, got %T", data["remaining_today"])

	spentToday, ok := remaining["TRX_spent_today"].(float64)
	require.True(t, ok, "expected TRX_spent_today to be a float64")
	assert.InDelta(t, 50.0, spentToday, 0.01, "expected 50 TRX spent")

	remainingToday, ok := remaining["TRX_remaining_today"].(float64)
	require.True(t, ok, "expected TRX_remaining_today to be a float64")
	assert.InDelta(t, 450.0, remainingToday, 0.01, "expected 450 TRX remaining")
}

func TestRebuildTransaction_ValidTransfer(t *testing.T) {
	mock := &mockWalletServer{
		CreateTransaction2Func: func(_ context.Context, tc *core.TransferContract) (*api.TransactionExtention, error) {
			return buildTransferTxExt(t), nil
		},
	}
	pool := newMockPool(t, mock)

	intent := &policy.Intent{
		Action:    "TransferContract",
		FromAddr:  "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		ToAddr:    "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		AmountSUN: 1_000_000,
	}

	tx, err := rebuildTransaction(context.Background(), pool, intent)
	require.NoError(t, err)
	require.NotNil(t, tx, "expected a non-nil transaction")
	require.NotNil(t, tx.RawData, "expected non-nil RawData")
}

func TestRebuildTransaction_UnsupportedAction(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})

	intent := &policy.Intent{
		Action:   "TriggerSmartContract",
		FromAddr: "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		ToAddr:   "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	}

	_, err := rebuildTransaction(context.Background(), pool, intent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot rebuild TriggerSmartContract")
}

func TestRebuildTransaction_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})

	intent := &policy.Intent{
		Action:   "TransferContract",
		FromAddr: "not-a-valid-address",
		ToAddr:   "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	}

	_, err := rebuildTransaction(context.Background(), pool, intent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid from address")
}
