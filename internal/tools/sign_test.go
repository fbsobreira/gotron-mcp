package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/approval"
	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/policy"
	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/client/transaction"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
	wm.SetPQScryptParams(keystore.LightScryptN, keystore.LightScryptP)
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
	wm, pool := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
		"transaction_hex": "",
		"wallet":          "test",
	})
	assert.True(t, result.IsError, "expected error for empty transaction_hex")
}

func TestSignTransaction_TooLong(t *testing.T) {
	wm, pool := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
		"transaction_hex": strings.Repeat("aa", maxHexInputLen+1),
		"wallet":          "test",
	})
	assert.True(t, result.IsError, "expected error for too-long transaction_hex")
}

func TestSignTransaction_EmptyWallet(t *testing.T) {
	wm, pool := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
		"transaction_hex": "0a0208",
		"wallet":          "",
	})
	assert.True(t, result.IsError, "expected error for empty wallet")
}

func TestSignTransaction_WalletNotFound(t *testing.T) {
	wm, pool := newTestSignSetup(t, &mockWalletServer{})
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
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
	wm, pool := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
		"transaction_hex": "not-hex",
		"wallet":          "test",
	})
	assert.True(t, result.IsError, "expected error for invalid hex")
}

func TestSignTransaction_Success(t *testing.T) {
	t.Run("by name", func(t *testing.T) {
		wm, pool := newTestSignSetup(t, &mockWalletServer{})

		// Create a wallet
		_, err := wm.CreateWallet("test-signer")
		require.NoError(t, err, "CreateWallet")

		txHex := buildTestTxHex(t)
		result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
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
		wm, pool := newTestSignSetup(t, &mockWalletServer{})

		// Create a wallet and use the returned address to sign
		addr, err := wm.CreateWallet("addr-signer")
		require.NoError(t, err, "CreateWallet")

		txHex := buildTestTxHex(t)
		result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
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
	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, nil), map[string]any{
		"transaction_hex": "not-hex",
		"wallet":          "test",
	})
	assert.True(t, result.IsError, "expected error for invalid hex")
}

func TestSignAndBroadcast_WalletNotFound(t *testing.T) {
	wm, pool := newTestSignSetup(t, &mockWalletServer{})
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, nil), map[string]any{
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
	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, nil), map[string]any{
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
	result := callTool(t, handleSignAndConfirm(nil, pool, wm, nil), map[string]any{
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
	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, nil), map[string]any{
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
	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, nil), map[string]any{
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
	handler := handleSignAndConfirm(nil, pool, wm, nil)
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
	result := callTool(t, handleSignAndConfirm(nil, pool, wm, nil), map[string]any{
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

func TestBroadcastTransaction_PQ(t *testing.T) {
	// signedPQHex signs a transfer from a new ML-DSA-44 wallet.
	signedPQHex := func(t *testing.T) (string, string) {
		t.Helper()
		mock := &mockWalletServer{GetChainParametersFunc: mlDsaParams(1)}
		wm, pool := newTestSignSetup(t, mock)
		pqAddr := newPQWallet(t, wm, "pq-wallet")
		mock.GetAccountFunc = accountFor(testAccount(pqAddr, nil))
		result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
			"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 1_000_000, 0),
			"wallet":          "pq-wallet",
		})
		require.False(t, result.IsError, "sign: %s", resultText(result))
		signedHex, ok := parseJSONResult(t, result)["signed_transaction_hex"].(string)
		require.True(t, ok)
		return signedHex, pqAddr
	}

	t.Run("reports the post-quantum signers", func(t *testing.T) {
		signedHex, pqAddr := signedPQHex(t)
		var broadcast *core.Transaction
		pool := newMockPool(t, &mockWalletServer{GetChainParametersFunc: mlDsaParams(1), BroadcastTransactionFunc: okBroadcast(&broadcast)})

		result := callTool(t, handleBroadcastTransaction(pool), map[string]any{"signed_transaction_hex": signedHex})
		require.False(t, result.IsError, "expected success, got: %s", resultText(result))
		require.NotNil(t, broadcast, "transaction must be broadcast")
		signers, ok := parseJSONResult(t, result)["pq_signers"].([]any)
		require.True(t, ok)
		require.Len(t, signers, 1)
		assert.Equal(t, map[string]any{"scheme": "ML_DSA_44", "address": pqAddr}, signers[0])
	})

	t.Run("scheme disabled stops before broadcast", func(t *testing.T) {
		signedHex, _ := signedPQHex(t)
		pool := newMockPool(t, &mockWalletServer{
			GetChainParametersFunc: mlDsaParams(0),
			BroadcastTransactionFunc: func(context.Context, *core.Transaction) (*api.Return, error) {
				t.Errorf("BroadcastTransaction must not be called")
				return &api.Return{Result: true}, nil
			},
		})

		result := callTool(t, handleBroadcastTransaction(pool), map[string]any{"signed_transaction_hex": signedHex})
		require.True(t, result.IsError)
		assert.Contains(t, resultText(result), "broadcast_transaction: transaction carries a post-quantum signature the node does not accept (TIP-899 is live only on Nile")
		assert.Contains(t, resultText(result), "ML_DSA_44")
	})

	t.Run("ECDSA transaction lists no signers and skips the probe", func(t *testing.T) {
		pool := newMockPool(t, &mockWalletServer{
			GetChainParametersFunc: func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
				t.Errorf("GetChainParameters must not be called")
				return nil, nil
			},
			BroadcastTransactionFunc: okBroadcast(nil),
		})
		txBytes, err := proto.Marshal(buildTransferTx(t, testAddr(0x11), testAddr(0x22), 1, 0))
		require.NoError(t, err)

		result := callTool(t, handleBroadcastTransaction(pool), map[string]any{"signed_transaction_hex": hex.EncodeToString(txBytes)})
		require.False(t, result.IsError, "expected success, got: %s", resultText(result))
		assert.Equal(t, []any{}, parseJSONResult(t, result)["pq_signers"])
	})
}

// TestSignWithECDSA_PQCoSignatureDisabled covers the one path where the
// post-sign probe is the only check: an ECDSA wallet co-signing a transaction
// that already carries a post-quantum signature the node refuses.
func TestSignWithECDSA_PQCoSignatureDisabled(t *testing.T) {
	tools := []struct {
		name    string
		handler func(*nodepool.Pool, *wallet.Manager) server.ToolHandlerFunc
	}{
		{"sign_and_broadcast", func(p *nodepool.Pool, wm *wallet.Manager) server.ToolHandlerFunc {
			return handleSignAndBroadcast(nil, p, wm, nil)
		}},
		{"sign_and_confirm", func(p *nodepool.Pool, wm *wallet.Manager) server.ToolHandlerFunc {
			return handleSignAndConfirm(nil, p, wm, nil)
		}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockWalletServer{
				GetChainParametersFunc: chainParams(map[string]int64{"getAllowFnDsa512": 0}),
				BroadcastTransactionFunc: func(context.Context, *core.Transaction) (*api.Return, error) {
					t.Errorf("BroadcastTransaction must not be called")
					return &api.Return{Result: true}, nil
				},
			}
			wm, pool := newTestSignSetup(t, mock)
			_, err := wm.CreateWallet("ecdsa-wallet")
			require.NoError(t, err)

			tx := withPQSig(buildTransferTx(t, testAddr(0x11), testAddr(0x22), 1_000_000, 0), core.PQScheme_FN_DSA_512)
			txBytes, err := proto.Marshal(tx)
			require.NoError(t, err)

			result := callTool(t, tt.handler(pool, wm), map[string]any{
				"transaction_hex": hex.EncodeToString(txBytes),
				"wallet":          "ecdsa-wallet",
			})
			require.True(t, result.IsError, "expected an error, got: %s", resultText(result))
			assert.Contains(t, resultText(result), tt.name+": transaction carries a post-quantum signature the node does not accept")
			assert.Contains(t, resultText(result), "FN_DSA_512")
		})
	}
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

	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, pe), map[string]any{
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

	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, pe), map[string]any{
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

	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, pe), map[string]any{
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

	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, pe), map[string]any{
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

	result := callTool(t, handleSignAndConfirm(nil, pool, wm, pe), map[string]any{
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

	tx, err := rebuildTransaction(context.Background(), pool, intent, 0)
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

	_, err := rebuildTransaction(context.Background(), pool, intent, 0)
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

	_, err := rebuildTransaction(context.Background(), pool, intent, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid from address")
}

// mockBasicSession implements server.ClientSession but NOT SessionWithElicitation.
type mockBasicSession struct{ id string }

func (m *mockBasicSession) SessionID() string { return m.id }
func (m *mockBasicSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return make(chan mcp.JSONRPCNotification, 1)
}
func (m *mockBasicSession) Initialize()       {}
func (m *mockBasicSession) Initialized() bool { return true }

// mockElicitSession implements server.SessionWithElicitation for testing.
type mockElicitSession struct {
	id     string
	result *mcp.ElicitationResult
	err    error
}

func (m *mockElicitSession) SessionID() string { return m.id }
func (m *mockElicitSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return make(chan mcp.JSONRPCNotification, 1)
}
func (m *mockElicitSession) Initialize()       {}
func (m *mockElicitSession) Initialized() bool { return true }
func (m *mockElicitSession) RequestElicitation(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

// --- Elicitation full flow tests ---

// callToolWithCtx invokes a handler with a custom context (for session injection).
func callToolWithCtx(t *testing.T, ctx context.Context, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	result, err := handler(ctx, req)
	if err != nil {
		t.Fatalf("handler returned Go error (not tool error): %v", err)
	}
	return result
}

func TestSignAndBroadcast_Elicitation_Confirmed(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{Result: true, Code: api.Return_SUCCESS, Message: []byte("ok")}, nil
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("elicit-signer")
	require.NoError(t, err)

	mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
	session := &mockElicitSession{
		id: "confirm-session",
		result: &mcp.ElicitationResult{
			ElicitationResponse: mcp.ElicitationResponse{
				Action: mcp.ElicitationResponseActionAccept,
			},
		},
	}
	ctx := mcpSrv.WithContext(context.Background(), session)

	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		1_000_000,
	)

	handler := handleSignAndBroadcast(mcpSrv, pool, wm, nil) // pe=nil triggers elicitation
	result := callToolWithCtx(t, ctx, handler, map[string]any{
		"transaction_hex": txHex,
		"wallet":          "elicit-signer",
	})
	require.False(t, result.IsError, "expected success after user confirmation, got: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["success"])
	assert.NotEmpty(t, data["txid"])
}

func TestSignAndBroadcast_Elicitation_Declined(t *testing.T) {
	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("elicit-signer")
	require.NoError(t, err)

	mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
	session := &mockElicitSession{
		id: "decline-session",
		result: &mcp.ElicitationResult{
			ElicitationResponse: mcp.ElicitationResponse{
				Action: mcp.ElicitationResponseActionDecline,
			},
		},
	}
	ctx := mcpSrv.WithContext(context.Background(), session)

	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		1_000_000,
	)

	handler := handleSignAndBroadcast(mcpSrv, pool, wm, nil)
	result := callToolWithCtx(t, ctx, handler, map[string]any{
		"transaction_hex": txHex,
		"wallet":          "elicit-signer",
	})
	require.False(t, result.IsError, "expected JSON result, not tool error")

	data := parseJSONResult(t, result)
	assert.Equal(t, "cancelled", data["status"])
	assert.Contains(t, data["reason"], "cancelled by user")
}

func TestSignAndBroadcast_Elicitation_NotSupported_Proceeds(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{Result: true, Code: api.Return_SUCCESS, Message: []byte("ok")}, nil
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("elicit-signer")
	require.NoError(t, err)

	// Use a session that does NOT support elicitation — should proceed without confirmation
	mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
	session := &mockBasicSession{id: "no-elicit"}
	ctx := mcpSrv.WithContext(context.Background(), session)

	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		1_000_000,
	)

	handler := handleSignAndBroadcast(mcpSrv, pool, wm, nil)
	result := callToolWithCtx(t, ctx, handler, map[string]any{
		"transaction_hex": txHex,
		"wallet":          "elicit-signer",
	})
	require.False(t, result.IsError, "expected success when elicitation not supported, got: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["success"], "should broadcast normally when elicitation unsupported")
}

func TestSignAndBroadcast_WithPolicy_SkipsElicitation(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{Result: true, Code: api.Return_SUCCESS, Message: []byte("ok")}, nil
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("policy-wallet")
	require.NoError(t, err)

	cfg := &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"policy-wallet": {PerTxLimitTRX: 1000},
		},
	}
	pe := newTestPolicyEngine(t, cfg)

	// Session that would decline — but policy is active so elicitation should be skipped
	mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
	session := &mockElicitSession{
		id: "should-not-be-called",
		result: &mcp.ElicitationResult{
			ElicitationResponse: mcp.ElicitationResponse{
				Action: mcp.ElicitationResponseActionDecline,
			},
		},
	}
	ctx := mcpSrv.WithContext(context.Background(), session)

	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		1_000_000,
	)

	handler := handleSignAndBroadcast(mcpSrv, pool, wm, pe)
	result := callToolWithCtx(t, ctx, handler, map[string]any{
		"transaction_hex": txHex,
		"wallet":          "policy-wallet",
	})
	require.False(t, result.IsError, "expected success — policy active means no elicitation, got: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["success"], "should broadcast without elicitation when policy is active")
}

func TestSignAndConfirm_Elicitation_Declined(t *testing.T) {
	mock := &mockWalletServer{}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("elicit-signer")
	require.NoError(t, err)

	mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
	session := &mockElicitSession{
		id: "decline-confirm",
		result: &mcp.ElicitationResult{
			ElicitationResponse: mcp.ElicitationResponse{
				Action: mcp.ElicitationResponseActionDecline,
			},
		},
	}
	ctx := mcpSrv.WithContext(context.Background(), session)

	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		1_000_000,
	)

	handler := handleSignAndConfirm(mcpSrv, pool, wm, nil)
	result := callToolWithCtx(t, ctx, handler, map[string]any{
		"transaction_hex": txHex,
		"wallet":          "elicit-signer",
	})
	require.False(t, result.IsError, "expected JSON result")

	data := parseJSONResult(t, result)
	assert.Equal(t, "cancelled", data["status"])
	assert.Contains(t, data["reason"], "cancelled by user")
}

func TestSignAndConfirm_Elicitation_Confirmed(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return &api.Return{Result: true, Code: api.Return_SUCCESS, Message: []byte("ok")}, nil
		},
		GetTransactionInfoByIdFunc: func(_ context.Context, msg *api.BytesMessage) (*core.TransactionInfo, error) {
			return &core.TransactionInfo{
				Id:          msg.Value,
				BlockNumber: 99999,
				Fee:         50000,
				Receipt:     &core.ResourceReceipt{EnergyUsageTotal: 10000, NetUsage: 200},
			}, nil
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("elicit-signer")
	require.NoError(t, err)

	mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
	session := &mockElicitSession{
		id: "confirm-session",
		result: &mcp.ElicitationResult{
			ElicitationResponse: mcp.ElicitationResponse{
				Action: mcp.ElicitationResponseActionAccept,
			},
		},
	}
	ctx := mcpSrv.WithContext(context.Background(), session)

	txHex := buildTransferTxHex(t,
		"TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		1_000_000,
	)

	handler := handleSignAndConfirm(mcpSrv, pool, wm, nil)
	result := callToolWithCtx(t, ctx, handler, map[string]any{
		"transaction_hex": txHex,
		"wallet":          "elicit-signer",
	})
	require.False(t, result.IsError, "expected success after confirmation, got: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["confirmed"])
	assert.NotEmpty(t, data["txid"])
}

// --- formatTokenAmount tests ---

func TestFormatTokenAmount(t *testing.T) {
	tests := []struct {
		name     string
		raw      *big.Int
		decimals int
		want     string
	}{
		{"nil", nil, 6, "0"},
		{"zero decimals", big.NewInt(42), 0, "42"},
		{"1 USDT (6 decimals)", big.NewInt(1_000_000), 6, "1"},
		{"10.5 USDT", big.NewInt(10_500_000), 6, "10.5"},
		{"0.000001 USDT", big.NewInt(1), 6, "0.000001"},
		{"large 18-decimal token", new(big.Int).SetUint64(1_000_000_000_000_000_000), 18, "1"},
		{"fractional 18-decimal", big.NewInt(123_456_789_012_345_678), 18, "0.123456789012345678"},
		{"negative decimals", big.NewInt(100), -1, "100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTokenAmount(tt.raw, tt.decimals)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- requestBroadcastConfirmation message building tests ---

func TestRequestBroadcastConfirmation_MessageVariants(t *testing.T) {
	mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
	acceptSession := &mockElicitSession{
		id: "msg-test",
		result: &mcp.ElicitationResult{
			ElicitationResponse: mcp.ElicitationResponse{
				Action: mcp.ElicitationResponseActionAccept,
			},
		},
	}

	tests := []struct {
		name    string
		decoded *transaction.ContractData
	}{
		{
			name: "TransferContract with string amount",
			decoded: &transaction.ContractData{
				Type:   "TransferContract",
				Fields: map[string]any{"owner_address": "TAddr1", "to_address": "TAddr2", "amount": "100.000000"},
			},
		},
		{
			name: "TransferContract with int64 amount (TRC10)",
			decoded: &transaction.ContractData{
				Type:   "TransferAssetContract",
				Fields: map[string]any{"owner_address": "TAddr1", "to_address": "TAddr2", "amount": int64(5000)},
			},
		},
		{
			name: "TransferContract with float64 amount",
			decoded: &transaction.ContractData{
				Type:   "TransferContract",
				Fields: map[string]any{"owner_address": "TAddr1", "to_address": "TAddr2", "amount": float64(42)},
			},
		},
		{
			name: "TriggerSmartContract with contract_address",
			decoded: &transaction.ContractData{
				Type:   "TriggerSmartContract",
				Fields: map[string]any{"owner_address": "TAddr1", "contract_address": "TContract", "data": "abcd", "call_value": "0.000000"},
			},
		},
		{
			name: "TriggerSmartContract with call_value",
			decoded: &transaction.ContractData{
				Type:   "TriggerSmartContract",
				Fields: map[string]any{"owner_address": "TAddr1", "contract_address": "TContract", "data": "abcd", "call_value": "5.000000"},
			},
		},
		{
			name: "minimal decoded data",
			decoded: &transaction.ContractData{
				Type:   "UnknownContract",
				Fields: map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mcpSrv.WithContext(context.Background(), acceptSession)
			err := requestBroadcastConfirmation(ctx, mcpSrv, nil, tt.decoded, "test-wallet", wallet.KeyTypeECDSA, nil)
			assert.NoError(t, err, "all message variants should proceed on Accept")
		})
	}
}

func TestRequestBroadcastConfirmation_ElicitationError(t *testing.T) {
	mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
	errSession := &mockElicitSession{
		id:  "err-session",
		err: fmt.Errorf("unexpected elicitation error"),
	}
	ctx := mcpSrv.WithContext(context.Background(), errSession)

	decoded := &transaction.ContractData{
		Type:   "TransferContract",
		Fields: map[string]any{"owner_address": "TAddr1", "to_address": "TAddr2", "amount": "1.000000"},
	}

	err := requestBroadcastConfirmation(ctx, mcpSrv, nil, decoded, "test-wallet", wallet.KeyTypeECDSA, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
}

// --- requestBroadcastConfirmation unit tests ---

func TestRequestBroadcastConfirmation(t *testing.T) {
	decoded := &transaction.ContractData{
		Type:   "TransferContract",
		Fields: map[string]any{"owner_address": "TAddr1", "to_address": "TAddr2", "amount": 100.0},
	}

	tests := []struct {
		name      string
		decoded   *transaction.ContractData
		srv       *server.MCPServer
		session   server.ClientSession
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "nil decoded skips confirmation",
			decoded: nil,
			srv:     nil,
			session: nil,
			wantErr: false,
		},
		{
			name:    "nil server skips confirmation",
			decoded: decoded,
			srv:     nil,
			session: nil,
			wantErr: false,
		},
		{
			name:    "accept proceeds",
			decoded: decoded,
			srv:     server.NewMCPServer("test", "1.0.0", server.WithElicitation()),
			session: &mockElicitSession{
				id: "accept",
				result: &mcp.ElicitationResult{
					ElicitationResponse: mcp.ElicitationResponse{
						Action: mcp.ElicitationResponseActionAccept,
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "decline cancels",
			decoded: decoded,
			srv:     server.NewMCPServer("test", "1.0.0", server.WithElicitation()),
			session: &mockElicitSession{
				id: "decline",
				result: &mcp.ElicitationResult{
					ElicitationResponse: mcp.ElicitationResponse{
						Action: mcp.ElicitationResponseActionDecline,
					},
				},
			},
			wantErr:   true,
			errSubstr: "cancelled by user",
		},
		{
			name:    "cancel cancels",
			decoded: decoded,
			srv:     server.NewMCPServer("test", "1.0.0", server.WithElicitation()),
			session: &mockElicitSession{
				id: "cancel",
				result: &mcp.ElicitationResult{
					ElicitationResponse: mcp.ElicitationResponse{
						Action: mcp.ElicitationResponseActionCancel,
					},
				},
			},
			wantErr:   true,
			errSubstr: "cancelled by user",
		},
		{
			name:    "session without elicitation support degrades gracefully",
			decoded: decoded,
			srv:     server.NewMCPServer("test", "1.0.0", server.WithElicitation()),
			session: &mockBasicSession{id: "basic"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.srv != nil && tt.session != nil {
				ctx = tt.srv.WithContext(ctx, tt.session)
			}
			err := requestBroadcastConfirmation(ctx, tt.srv, nil, tt.decoded, "test-wallet", wallet.KeyTypeECDSA, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- Post-quantum (TIP-899) sign path tests ---

// txExtFromTransfer builds the TransactionExtention a node returns for
// CreateTransaction2 with the given contract (permission id 0).
func txExtFromTransfer(t *testing.T, tc *core.TransferContract) *api.TransactionExtention {
	t.Helper()
	param, err := anypb.New(tc)
	require.NoError(t, err)
	return &api.TransactionExtention{
		Transaction: &core.Transaction{
			RawData: &core.TransactionRaw{
				Contract: []*core.Transaction_Contract{{
					Type:      core.Transaction_Contract_TransferContract,
					Parameter: param,
				}},
				Expiration: time.Now().Add(time.Minute).UnixMilli(),
			},
		},
		Txid:   []byte("stale-txid-from-node"),
		Result: &api.Return{Result: true, Code: api.Return_SUCCESS},
	}
}

// newPQWallet creates an ML-DSA-44 wallet and returns its address.
func newPQWallet(t *testing.T, wm *wallet.Manager, name string) string {
	t.Helper()
	addr, err := wm.CreateWalletWithKeyType(name, wallet.KeyTypeMLDSA44)
	require.NoError(t, err)
	return addr
}

// pqApprovalPolicy requires approval above 5 TRX and tracks a 500 TRX daily
// budget, so Check reserves spend for name.
func pqApprovalPolicy(name string) *policy.Config {
	return &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			name: {
				TokenLimits: map[string]*policy.TokenLimit{
					"TRX": {Decimals: 6, DailyLimitUnits: 500, ApprovalRequiredAboveUnits: 5},
				},
			},
		},
	}
}

// trxSpentToday reads the reserved TRX daily spend for a wallet.
func trxSpentToday(t *testing.T, pe *policy.Engine, name string) float64 {
	t.Helper()
	spent, ok := pe.GetRemainingBudget(name)["TRX_spent_today"].(float64)
	require.True(t, ok, "expected TRX_spent_today for %s", name)
	return spent
}

func okBroadcast(captured **core.Transaction) func(context.Context, *core.Transaction) (*api.Return, error) {
	return func(_ context.Context, tx *core.Transaction) (*api.Return, error) {
		if captured != nil {
			*captured = tx
		}
		return &api.Return{Result: true, Code: api.Return_SUCCESS, Message: []byte("ok")}, nil
	}
}

func failBroadcast(t *testing.T) func(context.Context, *core.Transaction) (*api.Return, error) {
	return func(context.Context, *core.Transaction) (*api.Return, error) {
		t.Errorf("BroadcastTransaction must not be called")
		return &api.Return{Result: false}, nil
	}
}

func failAccount(t *testing.T) func(context.Context, *core.Account) (*core.Account, error) {
	return func(context.Context, *core.Account) (*core.Account, error) {
		t.Errorf("GetAccount must not be called")
		return &core.Account{}, nil
	}
}

func resultText(result *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestSignAndBroadcast_PQ_Success(t *testing.T) {
	var captured *core.Transaction
	mock := &mockWalletServer{GetChainParametersFunc: mlDsaParams(1), BroadcastTransactionFunc: okBroadcast(&captured)}
	wm, pool := newTestSignSetup(t, mock)
	pqAddr := newPQWallet(t, wm, "pq-wallet")
	mock.GetAccountFunc = accountFor(testAccount(pqAddr, nil))

	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, nil), map[string]any{
		"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 1_000_000, 0),
		"wallet":          "pq-wallet",
	})
	require.False(t, result.IsError, "expected success, got: %s", resultText(result))

	require.NotNil(t, captured, "broadcast must be called")
	require.Len(t, captured.GetPqAuthSig(), 1, "broadcast tx must carry one PQ signature")
	assert.Empty(t, captured.GetSignature(), "PQ wallet must not add an ECDSA signature")

	data := parseJSONResult(t, result)
	assert.Equal(t, "ml_dsa_44", data["key_type"])
	size, ok := data["signed_size_bytes"].(float64)
	require.True(t, ok, "signed_size_bytes must be a number, got %v", data["signed_size_bytes"])
	assert.Equal(t, float64(proto.Size(captured)), size)
	wantTxID, err := computeTxID(captured)
	require.NoError(t, err)
	assert.Equal(t, wantTxID, data["txid"])
}

func TestSignAndConfirm_PQ_Success(t *testing.T) {
	var captured *core.Transaction
	mock := &mockWalletServer{
		GetChainParametersFunc:   mlDsaParams(1),
		BroadcastTransactionFunc: okBroadcast(&captured),
		GetTransactionInfoByIdFunc: func(_ context.Context, msg *api.BytesMessage) (*core.TransactionInfo, error) {
			return &core.TransactionInfo{Id: msg.Value, BlockNumber: 42}, nil
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	pqAddr := newPQWallet(t, wm, "pq-wallet")
	mock.GetAccountFunc = accountFor(testAccount(pqAddr, nil))

	result := callTool(t, handleSignAndConfirm(nil, pool, wm, nil), map[string]any{
		"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 1_000_000, 0),
		"wallet":          pqAddr, // by address
	})
	require.False(t, result.IsError, "expected success, got: %s", resultText(result))
	require.NotNil(t, captured)
	require.Len(t, captured.GetPqAuthSig(), 1)

	data := parseJSONResult(t, result)
	assert.Equal(t, true, data["confirmed"])
	assert.Equal(t, "ml_dsa_44", data["key_type"])
	assert.Equal(t, float64(proto.Size(captured)), data["signed_size_bytes"])
}

// A disabled scheme stops the tool with no approval and no spend left
// reserved. The spend assertion also holds if Check ran after the rollback
// was armed, so it does not prove the ordering;
// TestPQ_PreflightBeforeCheck_NoPricerCall does.
func TestSignAndBroadcast_PQ_SchemeDisabled_NoReservationNoApproval(t *testing.T) {
	mock := &mockWalletServer{
		GetChainParametersFunc:   mlDsaParams(0),
		BroadcastTransactionFunc: failBroadcast(t),
	}
	mock.GetAccountFunc = failAccount(t)
	wm, pool := newTestSignSetup(t, mock)
	pqAddr := newPQWallet(t, wm, "pq-wallet")

	pe := newTestPolicyEngine(t, pqApprovalPolicy("pq-wallet"))
	approver := &countingApprover{approved: true}
	pe.SetApprover(approver)

	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, pe), map[string]any{
		"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 10_000_000, 0),
		"wallet":          "pq-wallet",
	})
	require.True(t, result.IsError, "expected tool error, got: %s", resultText(result))
	assert.Contains(t, resultText(result), "sign_and_broadcast: node does not allow ML-DSA-44 signatures")
	assert.Equal(t, int32(0), approver.calls.Load(), "no approval may be requested")
	assert.InDelta(t, 0.0, trxSpentToday(t, pe, "pq-wallet"), 1e-9, "no daily spend may stay reserved")
}

func TestSignAndConfirm_PQ_SchemeDisabled_NoReservationNoApproval(t *testing.T) {
	mock := &mockWalletServer{
		GetChainParametersFunc:   mlDsaParams(0),
		BroadcastTransactionFunc: failBroadcast(t),
	}
	mock.GetAccountFunc = failAccount(t)
	wm, pool := newTestSignSetup(t, mock)
	pqAddr := newPQWallet(t, wm, "pq-wallet")

	pe := newTestPolicyEngine(t, pqApprovalPolicy("pq-wallet"))
	approver := &countingApprover{approved: true}
	pe.SetApprover(approver)

	result := callTool(t, handleSignAndConfirm(nil, pool, wm, pe), map[string]any{
		"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 10_000_000, 0),
		"wallet":          "pq-wallet",
	})
	require.True(t, result.IsError, "expected tool error, got: %s", resultText(result))
	assert.Contains(t, resultText(result), "sign_and_confirm: node does not allow ML-DSA-44 signatures")
	assert.Equal(t, int32(0), approver.calls.Load(), "no approval may be requested")
	assert.InDelta(t, 0.0, trxSpentToday(t, pe, "pq-wallet"), 1e-9, "no daily spend may stay reserved")
}

func TestRequestLimitOverride_PQ_SchemeDisabled_NoApproval(t *testing.T) {
	mock := &mockWalletServer{
		GetChainParametersFunc:   mlDsaParams(0),
		BroadcastTransactionFunc: failBroadcast(t),
		CreateTransaction2Func: func(context.Context, *core.TransferContract) (*api.TransactionExtention, error) {
			t.Errorf("CreateTransaction2 must not be called")
			return nil, fmt.Errorf("unexpected")
		},
	}
	mock.GetAccountFunc = failAccount(t)
	wm, pool := newTestSignSetup(t, mock)
	pqAddr := newPQWallet(t, wm, "pq-wallet")

	pe := newTestPolicyEngine(t, pqApprovalPolicy("pq-wallet"))
	approver := &countingApprover{approved: true}
	pe.SetApprover(approver)

	result := callTool(t, handleRequestLimitOverride(pool, wm, pe), map[string]any{
		"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 10_000_000, 0),
		"wallet":          "pq-wallet",
		"reason":          "payout",
	})
	require.True(t, result.IsError, "expected tool error, got: %s", resultText(result))
	assert.Contains(t, resultText(result), "request_limit_override: node does not allow ML-DSA-44 signatures")
	assert.Equal(t, int32(0), approver.calls.Load(), "no approval may be requested")
}

func TestRequestLimitOverride_PQ_Success(t *testing.T) {
	var captured *core.Transaction
	mock := &mockWalletServer{
		GetChainParametersFunc:   mlDsaParams(1),
		BroadcastTransactionFunc: okBroadcast(&captured),
		CreateTransaction2Func: func(_ context.Context, tc *core.TransferContract) (*api.TransactionExtention, error) {
			return txExtFromTransfer(t, tc), nil
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	pqAddr := newPQWallet(t, wm, "pq-wallet")
	mock.GetAccountFunc = accountFor(testAccount(pqAddr, nil))

	pe := newTestPolicyEngine(t, pqApprovalPolicy("pq-wallet"))
	pe.SetApprover(&countingApprover{approved: true})

	result := callTool(t, handleRequestLimitOverride(pool, wm, pe), map[string]any{
		"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 10_000_000, 0),
		"wallet":          "pq-wallet",
		"reason":          "payout",
	})
	require.False(t, result.IsError, "expected success, got: %s", resultText(result))
	require.NotNil(t, captured)
	require.Len(t, captured.GetPqAuthSig(), 1)

	data := parseJSONResult(t, result)
	assert.Equal(t, "ml_dsa_44", data["key_type"])
	assert.Equal(t, float64(proto.Size(captured)), data["signed_size_bytes"])
}

// An ECDSA wallet must never trigger the TIP-899 probe or the owner check.
func TestSignAndBroadcast_ECDSA_NoPQProbe(t *testing.T) {
	mock := &mockWalletServer{
		GetChainParametersFunc: func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
			t.Errorf("GetChainParameters must not be called for an ECDSA wallet")
			return nil, fmt.Errorf("unexpected")
		},
		BroadcastTransactionFunc: okBroadcast(nil),
	}
	mock.GetAccountFunc = failAccount(t)
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("ecdsa-wallet")
	require.NoError(t, err)

	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, nil), map[string]any{
		"transaction_hex": buildTransferTxHexPerm(t, testAddr(0x11), testAddr(0x22), 1_000_000, 0),
		"wallet":          "ecdsa-wallet",
	})
	require.False(t, result.IsError, "expected success, got: %s", resultText(result))
	data := parseJSONResult(t, result)
	assert.Equal(t, "ecdsa", data["key_type"])
	assert.Greater(t, data["signed_size_bytes"], float64(0))
}

// The rebuilt transaction after approval must keep the original permission_id,
// and the reported txid must match the signed RawData, not the node's Txid.
func TestSignAndBroadcast_ApprovalFlow_KeepsPermissionID(t *testing.T) {
	var captured *core.Transaction
	mock := &mockWalletServer{
		CreateTransaction2Func: func(_ context.Context, tc *core.TransferContract) (*api.TransactionExtention, error) {
			return txExtFromTransfer(t, tc), nil
		},
		BroadcastTransactionFunc: okBroadcast(&captured),
	}
	wm, pool := newTestSignSetup(t, mock)
	_, err := wm.CreateWallet("hot-wallet")
	require.NoError(t, err)

	pe := newTestPolicyEngine(t, pqApprovalPolicy("hot-wallet"))
	pe.SetApprover(&countingApprover{approved: true})

	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, pe), map[string]any{
		"transaction_hex": buildTransferTxHexPerm(t, testAddr(0x11), testAddr(0x22), 10_000_000, 2),
		"wallet":          "hot-wallet",
	})
	require.False(t, result.IsError, "expected success, got: %s", resultText(result))
	require.NotNil(t, captured)
	assert.Equal(t, int32(2), captured.GetRawData().GetContract()[0].GetPermissionId(), "permission_id must survive the rebuild")

	wantTxID, err := computeTxID(captured)
	require.NoError(t, err)
	assert.Equal(t, wantTxID, parseJSONResult(t, result)["txid"])
}

// After approval and rebuild, a PQ wallet's owner check runs again; a failure
// there happens after Check, so the reserved spend must be rolled back.
func TestPQ_RebuildOwnerRecheck_RollsBackReservation(t *testing.T) {
	handlers := []struct {
		name string
		h    func(*mockWalletServer, *wallet.Manager, *nodepool.Pool, *policy.Engine) server.ToolHandlerFunc
	}{
		{"sign_and_broadcast", func(_ *mockWalletServer, wm *wallet.Manager, p *nodepool.Pool, pe *policy.Engine) server.ToolHandlerFunc {
			return handleSignAndBroadcast(nil, p, wm, pe)
		}},
		{"sign_and_confirm", func(_ *mockWalletServer, wm *wallet.Manager, p *nodepool.Pool, pe *policy.Engine) server.ToolHandlerFunc {
			return handleSignAndConfirm(nil, p, wm, pe)
		}},
		{"request_limit_override", func(_ *mockWalletServer, wm *wallet.Manager, p *nodepool.Pool, pe *policy.Engine) server.ToolHandlerFunc {
			return handleRequestLimitOverride(p, wm, pe)
		}},
	}
	for _, hc := range handlers {
		t.Run(hc.name, func(t *testing.T) {
			var rebuilt atomic.Bool
			mock := &mockWalletServer{
				GetChainParametersFunc:   mlDsaParams(1),
				BroadcastTransactionFunc: failBroadcast(t),
				CreateTransaction2Func: func(_ context.Context, tc *core.TransferContract) (*api.TransactionExtention, error) {
					rebuilt.Store(true)
					return txExtFromTransfer(t, tc), nil
				},
			}
			wm, pool := newTestSignSetup(t, mock)
			pqAddr := newPQWallet(t, wm, "pq-wallet")
			// Before the rebuild the wallet controls its own account; by the
			// time of the re-check the owner permission has moved elsewhere.
			before := accountFor(testAccount(pqAddr, nil))
			after := accountFor(testAccount(pqAddr, testPermission(0, testAddr(0x33))))
			mock.GetAccountFunc = func(ctx context.Context, in *core.Account) (*core.Account, error) {
				if rebuilt.Load() {
					return after(ctx, in)
				}
				return before(ctx, in)
			}

			pe := newTestPolicyEngine(t, pqApprovalPolicy("pq-wallet"))
			approver := &countingApprover{approved: true}
			pe.SetApprover(approver)

			args := map[string]any{
				"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 10_000_000, 0),
				"wallet":          "pq-wallet",
				"reason":          "payout",
			}
			result := callTool(t, hc.h(mock, wm, pool, pe), args)
			require.True(t, result.IsError, "expected tool error, got: %s", resultText(result))
			assert.Contains(t, resultText(result), "is not a key of permission 0")
			assert.Equal(t, int32(1), approver.calls.Load(), "approval happens before the rebuild")
			assert.True(t, rebuilt.Load(), "transaction must have been rebuilt")
			assert.InDelta(t, 0.0, trxSpentToday(t, pe, "pq-wallet"), 1e-9, "reservation must be rolled back")
		})
	}
}

// captureElicitSession records the elicitation message and accepts it.
type captureElicitSession struct {
	id      string
	message string
	called  bool
}

func (c *captureElicitSession) SessionID() string { return c.id }
func (c *captureElicitSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return make(chan mcp.JSONRPCNotification, 1)
}
func (c *captureElicitSession) Initialize()       {}
func (c *captureElicitSession) Initialized() bool { return true }
func (c *captureElicitSession) RequestElicitation(_ context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	c.called = true
	c.message = req.Params.Message
	return &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionAccept},
	}, nil
}

func TestSignAndBroadcast_Elicitation_ShowsKeyType(t *testing.T) {
	const pqLine = "Signature: ML-DSA-44 (post-quantum, TIP-899)\n"
	const noteLine = "Note: post-quantum signatures make the transaction about 10-22x larger, so it uses more bandwidth\n"

	t.Run("post-quantum wallet", func(t *testing.T) {
		mock := &mockWalletServer{GetChainParametersFunc: mlDsaParams(1), BroadcastTransactionFunc: okBroadcast(nil)}
		wm, pool := newTestSignSetup(t, mock)
		pqAddr := newPQWallet(t, wm, "pq-wallet")
		mock.GetAccountFunc = accountFor(testAccount(pqAddr, nil))

		mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
		session := &captureElicitSession{id: "pq-elicit"}
		ctx := mcpSrv.WithContext(context.Background(), session)

		result := callToolWithCtx(t, ctx, handleSignAndBroadcast(mcpSrv, pool, wm, nil), map[string]any{
			"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 1_000_000, 0),
			"wallet":          "pq-wallet",
		})
		require.False(t, result.IsError, "expected success, got: %s", resultText(result))
		assert.Contains(t, session.message, pqLine)
		assert.Contains(t, session.message, noteLine)
		assert.Regexp(t, `\nEstimated bandwidth: about [0-9]+ bytes\n`, session.message)
	})

	t.Run("ECDSA wallet text unchanged", func(t *testing.T) {
		mock := &mockWalletServer{BroadcastTransactionFunc: okBroadcast(nil)}
		wm, pool := newTestSignSetup(t, mock)
		_, err := wm.CreateWallet("ecdsa-wallet")
		require.NoError(t, err)

		mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
		session := &captureElicitSession{id: "ecdsa-elicit"}
		ctx := mcpSrv.WithContext(context.Background(), session)

		owner := testAddr(0x11)
		result := callToolWithCtx(t, ctx, handleSignAndBroadcast(mcpSrv, pool, wm, nil), map[string]any{
			"transaction_hex": buildTransferTxHexPerm(t, owner, testAddr(0x22), 1_000_000, 0),
			"wallet":          "ecdsa-wallet",
		})
		require.False(t, result.IsError, "expected success, got: %s", resultText(result))
		want := "Confirm transaction before broadcast:\n\n" +
			"Type: TransferContract\n" +
			fmt.Sprintf("From: %s (wallet: %q)\n", owner, "ecdsa-wallet") +
			fmt.Sprintf("To: %s\n", testAddr(0x22)) +
			"Amount: 1.000000 TRX\n"
		assert.Equal(t, want, session.message)
	})
}

func TestSignTransaction_PQ(t *testing.T) {
	t.Run("signs with a PQ signature", func(t *testing.T) {
		mock := &mockWalletServer{GetChainParametersFunc: mlDsaParams(1)}
		wm, pool := newTestSignSetup(t, mock)
		pqAddr := newPQWallet(t, wm, "pq-wallet")
		mock.GetAccountFunc = accountFor(testAccount(pqAddr, nil))

		result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
			"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 1_000_000, 0),
			"wallet":          "pq-wallet",
		})
		require.False(t, result.IsError, "expected success, got: %s", resultText(result))

		data := parseJSONResult(t, result)
		signedHex, ok := data["signed_transaction_hex"].(string)
		require.True(t, ok)
		signedBytes, err := hex.DecodeString(signedHex)
		require.NoError(t, err)
		var signed core.Transaction
		require.NoError(t, proto.Unmarshal(signedBytes, &signed))
		require.Len(t, signed.GetPqAuthSig(), 1, "signed tx must carry one PQ signature")
		assert.Empty(t, signed.GetSignature())
		assert.Equal(t, "ml_dsa_44", data["key_type"])
		assert.Equal(t, float64(len(signedBytes)), data["signed_size_bytes"])
	})

	t.Run("scheme disabled stops before signing", func(t *testing.T) {
		mock := &mockWalletServer{GetChainParametersFunc: mlDsaParams(0)}
		mock.GetAccountFunc = failAccount(t)
		wm, pool := newTestSignSetup(t, mock)
		pqAddr := newPQWallet(t, wm, "pq-wallet")

		result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
			"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 1_000_000, 0),
			"wallet":          "pq-wallet",
		})
		require.True(t, result.IsError)
		assert.Contains(t, resultText(result), "sign_transaction: node does not allow ML-DSA-44 signatures")
	})

	t.Run("ECDSA wallet does not probe the node", func(t *testing.T) {
		mock := &mockWalletServer{
			GetChainParametersFunc: func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
				t.Errorf("GetChainParameters must not be called for an ECDSA wallet")
				return nil, fmt.Errorf("unexpected")
			},
		}
		mock.GetAccountFunc = failAccount(t)
		wm, pool := newTestSignSetup(t, mock)
		_, err := wm.CreateWallet("ecdsa-wallet")
		require.NoError(t, err)

		result := callTool(t, handleSignTransaction(pool, wm), map[string]any{
			"transaction_hex": buildTestTxHex(t),
			"wallet":          "ecdsa-wallet",
		})
		require.False(t, result.IsError, "expected success, got: %s", resultText(result))
		assert.Equal(t, "ecdsa", parseJSONResult(t, result)["key_type"])
	})
}

func TestRequestBroadcastConfirmation_KeyTypeLines(t *testing.T) {
	decoded := &transaction.ContractData{
		Type:   "TransferContract",
		Fields: map[string]any{"owner_address": "TAddr1", "to_address": "TAddr2", "amount": "1.000000"},
	}
	ecdsaWant := "Confirm transaction before broadcast:\n\n" +
		"Type: TransferContract\n" +
		"From: TAddr1 (wallet: \"w\")\n" +
		"To: TAddr2\n" +
		"Amount: 1.000000 TRX\n"
	pqWant := func(name string) string {
		return "Confirm transaction before broadcast:\n\n" +
			"Type: TransferContract\n" +
			"From: TAddr1 (wallet: \"w\")\n" +
			"Signature: " + name + " (post-quantum, TIP-899)\n" +
			"Note: post-quantum signatures make the transaction about 10-22x larger, so it uses more bandwidth\n" +
			"To: TAddr2\n" +
			"Amount: 1.000000 TRX\n"
	}

	tests := []struct {
		name    string
		keyType wallet.KeyType
		want    string
	}{
		{"ecdsa", wallet.KeyTypeECDSA, ecdsaWant},
		{"unknown (lookup failed)", "", ecdsaWant},
		{"ml_dsa_44", wallet.KeyTypeMLDSA44, pqWant("ML-DSA-44")},
		{"fn_dsa_512", wallet.KeyTypeFNDSA512, pqWant("FN-DSA-512")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
			session := &captureElicitSession{id: "kt-" + tt.name}
			ctx := mcpSrv.WithContext(context.Background(), session)
			require.NoError(t, requestBroadcastConfirmation(ctx, mcpSrv, nil, decoded, "w", tt.keyType, nil))
			assert.Equal(t, tt.want, session.message)
		})
	}
}

func TestRebuildTransaction_PermissionID(t *testing.T) {
	intent := &policy.Intent{
		Action:    "TransferContract",
		FromAddr:  "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		ToAddr:    "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		AmountSUN: 1_000_000,
	}

	t.Run("keeps the original permission id", func(t *testing.T) {
		for _, perm := range []int32{0, 2, 7} {
			mock := &mockWalletServer{
				CreateTransaction2Func: func(_ context.Context, tc *core.TransferContract) (*api.TransactionExtention, error) {
					return txExtFromTransfer(t, tc), nil
				},
			}
			tx, err := rebuildTransaction(context.Background(), newMockPool(t, mock), intent, perm)
			require.NoError(t, err)
			assert.Equal(t, perm, tx.GetRawData().GetContract()[0].GetPermissionId())
			owner, gotPerm, _, err := transaction.OwnerAndPermission(tx)
			require.NoError(t, err)
			assert.Equal(t, intent.FromAddr, owner.String())
			assert.Equal(t, perm, gotPerm)
		}
	})

	t.Run("node response without a contract is an error", func(t *testing.T) {
		mock := &mockWalletServer{
			CreateTransaction2Func: func(context.Context, *core.TransferContract) (*api.TransactionExtention, error) {
				return &api.TransactionExtention{
					Transaction: &core.Transaction{RawData: &core.TransactionRaw{}},
					Result:      &api.Return{Result: true, Code: api.Return_SUCCESS},
				}, nil
			},
		}
		_, err := rebuildTransaction(context.Background(), newMockPool(t, mock), intent, 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node returned 0 contracts")
	})
}

// The TIP-899 preflight runs before pe.Check: a disabled scheme is reported
// as a tool error even when the policy would also deny the transaction.
func TestSignAndBroadcast_PQ_PreflightBeforePolicyCheck(t *testing.T) {
	mock := &mockWalletServer{
		GetChainParametersFunc:   mlDsaParams(0),
		BroadcastTransactionFunc: failBroadcast(t),
	}
	mock.GetAccountFunc = failAccount(t)
	wm, pool := newTestSignSetup(t, mock)
	pqAddr := newPQWallet(t, wm, "pq-wallet")

	pe := newTestPolicyEngine(t, &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			"pq-wallet": {TokenLimits: map[string]*policy.TokenLimit{
				"TRX": {Decimals: 6, PerTxLimitUnits: 1},
			}},
		},
	})

	result := callTool(t, handleSignAndBroadcast(nil, pool, wm, pe), map[string]any{
		"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 10_000_000, 0),
		"wallet":          "pq-wallet",
	})
	require.True(t, result.IsError, "expected the preflight tool error, got: %s", resultText(result))
	assert.Contains(t, resultText(result), "node does not allow ML-DSA-44 signatures")
}

// countingPricer is a policy.PriceProvider that counts price lookups. With a
// USD limit on the wallet, pe.Check asks it for a price, so a call proves
// Check ran.
type countingPricer struct{ calls atomic.Int32 }

func (p *countingPricer) GetTokenPrice(context.Context, string) (float64, error) {
	p.calls.Add(1)
	return 0.1, nil
}

// usdLimitPolicy sets a per-transaction USD limit for name, so pe.Check
// always consults the price provider.
func usdLimitPolicy(name string) *policy.Config {
	return &policy.Config{
		Enabled: true,
		Wallets: map[string]*policy.WalletPolicy{
			name: {PerTxLimitUSD: 1000},
		},
	}
}

func failChainParams(t *testing.T) func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
	return func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
		t.Errorf("GetChainParameters must not be called")
		return nil, fmt.Errorf("unexpected")
	}
}

func confirmedTxInfo(_ context.Context, msg *api.BytesMessage) (*core.TransactionInfo, error) {
	return &core.TransactionInfo{Id: msg.Value, BlockNumber: 42}, nil
}

// The TIP-899 preflight runs before pe.Check. The pricer counts Check
// entries: one call with the scheme enabled (the control), none with it
// disabled.
func TestPQ_PreflightBeforeCheck_NoPricerCall(t *testing.T) {
	handlers := []struct {
		name string
		h    func(*nodepool.Pool, *wallet.Manager, *policy.Engine) server.ToolHandlerFunc
	}{
		{"sign_and_broadcast", func(p *nodepool.Pool, wm *wallet.Manager, pe *policy.Engine) server.ToolHandlerFunc {
			return handleSignAndBroadcast(nil, p, wm, pe)
		}},
		{"sign_and_confirm", func(p *nodepool.Pool, wm *wallet.Manager, pe *policy.Engine) server.ToolHandlerFunc {
			return handleSignAndConfirm(nil, p, wm, pe)
		}},
	}
	for _, hc := range handlers {
		t.Run(hc.name+"/scheme enabled enters Check", func(t *testing.T) {
			mock := &mockWalletServer{
				GetChainParametersFunc:     mlDsaParams(1),
				BroadcastTransactionFunc:   okBroadcast(nil),
				GetTransactionInfoByIdFunc: confirmedTxInfo,
			}
			wm, pool := newTestSignSetup(t, mock)
			pqAddr := newPQWallet(t, wm, "pq-wallet")
			mock.GetAccountFunc = accountFor(testAccount(pqAddr, nil))
			pe := newTestPolicyEngine(t, usdLimitPolicy("pq-wallet"))
			pricer := &countingPricer{}
			pe.SetPriceProvider(pricer)

			result := callTool(t, hc.h(pool, wm, pe), map[string]any{
				"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 10_000_000, 0),
				"wallet":          "pq-wallet",
			})
			require.False(t, result.IsError, "expected success, got: %s", resultText(result))
			assert.Positive(t, pricer.calls.Load(), "control: Check must consult the pricer")
		})
		t.Run(hc.name+"/scheme disabled never enters Check", func(t *testing.T) {
			mock := &mockWalletServer{
				GetChainParametersFunc:   mlDsaParams(0),
				BroadcastTransactionFunc: failBroadcast(t),
			}
			mock.GetAccountFunc = failAccount(t)
			wm, pool := newTestSignSetup(t, mock)
			pqAddr := newPQWallet(t, wm, "pq-wallet")
			pe := newTestPolicyEngine(t, usdLimitPolicy("pq-wallet"))
			pricer := &countingPricer{}
			pe.SetPriceProvider(pricer)

			result := callTool(t, hc.h(pool, wm, pe), map[string]any{
				"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 10_000_000, 0),
				"wallet":          "pq-wallet",
			})
			require.True(t, result.IsError, "expected tool error, got: %s", resultText(result))
			assert.Contains(t, resultText(result), hc.name+": node does not allow ML-DSA-44 signatures")
			assert.Equal(t, int32(0), pricer.calls.Load(), "pe.Check must not run before the preflight")
		})
	}
}

// Without a policy engine, a failed preflight returns before the
// confirmation prompt is sent.
func TestPQ_PreflightBeforeElicitation(t *testing.T) {
	handlers := []struct {
		name string
		h    func(*server.MCPServer, *nodepool.Pool, *wallet.Manager) server.ToolHandlerFunc
	}{
		{"sign_and_broadcast", func(s *server.MCPServer, p *nodepool.Pool, wm *wallet.Manager) server.ToolHandlerFunc {
			return handleSignAndBroadcast(s, p, wm, nil)
		}},
		{"sign_and_confirm", func(s *server.MCPServer, p *nodepool.Pool, wm *wallet.Manager) server.ToolHandlerFunc {
			return handleSignAndConfirm(s, p, wm, nil)
		}},
	}
	for _, hc := range handlers {
		t.Run(hc.name, func(t *testing.T) {
			mock := &mockWalletServer{
				GetChainParametersFunc:   mlDsaParams(0),
				BroadcastTransactionFunc: failBroadcast(t),
			}
			mock.GetAccountFunc = failAccount(t)
			wm, pool := newTestSignSetup(t, mock)
			pqAddr := newPQWallet(t, wm, "pq-wallet")

			mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
			session := &captureElicitSession{id: "preflight-" + hc.name}
			ctx := mcpSrv.WithContext(context.Background(), session)

			result := callToolWithCtx(t, ctx, hc.h(mcpSrv, pool, wm), map[string]any{
				"transaction_hex": buildTransferTxHexPerm(t, pqAddr, testAddr(0x22), 1_000_000, 0),
				"wallet":          "pq-wallet",
			})
			require.True(t, result.IsError, "expected tool error, got: %s", resultText(result))
			assert.Contains(t, resultText(result), hc.name+": node does not allow ML-DSA-44 signatures")
			assert.False(t, session.called, "the confirmation prompt must not be sent")
		})
	}
}

// A wallet name held by both an ECDSA and a post-quantum wallet fails at
// lookup: no PQ probe, no policy check, no approval, no prompt, no broadcast.
func TestSigningTools_AmbiguousWallet_FailsBeforePolicy(t *testing.T) {
	const wantMsg = `wallet name "dup" exists as both an ECDSA and a post-quantum wallet; rename one`

	type deps struct {
		srv *server.MCPServer
		wm  *wallet.Manager
		p   *nodepool.Pool
		pe  *policy.Engine
	}
	tests := []struct {
		name     string
		withPE   bool
		withElic bool
		h        func(deps) server.ToolHandlerFunc
	}{
		{"sign_transaction", false, false, func(d deps) server.ToolHandlerFunc {
			return handleSignTransaction(d.p, d.wm)
		}},
		{"sign_and_broadcast", true, false, func(d deps) server.ToolHandlerFunc {
			return handleSignAndBroadcast(d.srv, d.p, d.wm, d.pe)
		}},
		{"sign_and_broadcast", false, true, func(d deps) server.ToolHandlerFunc {
			return handleSignAndBroadcast(d.srv, d.p, d.wm, nil)
		}},
		{"sign_and_confirm", true, false, func(d deps) server.ToolHandlerFunc {
			return handleSignAndConfirm(d.srv, d.p, d.wm, d.pe)
		}},
		{"sign_and_confirm", false, true, func(d deps) server.ToolHandlerFunc {
			return handleSignAndConfirm(d.srv, d.p, d.wm, nil)
		}},
		{"request_limit_override", true, false, func(d deps) server.ToolHandlerFunc {
			return handleRequestLimitOverride(d.p, d.wm, d.pe)
		}},
	}
	for _, tt := range tests {
		label := tt.name + "/policy"
		if tt.withElic {
			label = tt.name + "/elicitation"
		}
		if !tt.withPE && !tt.withElic {
			label = tt.name
		}
		t.Run(label, func(t *testing.T) {
			mock := &mockWalletServer{
				GetChainParametersFunc:   failChainParams(t),
				BroadcastTransactionFunc: failBroadcast(t),
				CreateTransaction2Func: func(context.Context, *core.TransferContract) (*api.TransactionExtention, error) {
					t.Errorf("CreateTransaction2 must not be called")
					return nil, fmt.Errorf("unexpected")
				},
			}
			mock.GetAccountFunc = failAccount(t)
			wm, _, ecdsaAddr := newCollisionManager(t)
			pool := newMockPool(t, mock)

			// Approval above 5 TRX plus a USD limit: without the early
			// return, a 10 TRX transfer enters Check and asks the approver.
			cfg := pqApprovalPolicy("dup")
			cfg.Wallets["dup"].PerTxLimitUSD = 1000
			pe := newTestPolicyEngine(t, cfg)
			approver := &countingApprover{approved: true}
			pe.SetApprover(approver)
			pricer := &countingPricer{}
			pe.SetPriceProvider(pricer)

			mcpSrv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
			session := &captureElicitSession{id: "ambiguous-" + label}
			ctx := mcpSrv.WithContext(context.Background(), session)

			result := callToolWithCtx(t, ctx, tt.h(deps{srv: mcpSrv, wm: wm, p: pool, pe: pe}), map[string]any{
				"transaction_hex": buildTransferTxHexPerm(t, ecdsaAddr, testAddr(0x22), 10_000_000, 0),
				"wallet":          "dup",
				"reason":          "payout",
			})
			require.True(t, result.IsError, "expected tool error, got: %s", resultText(result))
			assert.Contains(t, resultText(result), tt.name+": ")
			assert.Contains(t, resultText(result), wantMsg)
			assert.Equal(t, int32(0), pricer.calls.Load(), "pe.Check must not run")
			assert.Equal(t, int32(0), approver.calls.Load(), "no approval may be requested")
			assert.False(t, session.called, "the confirmation prompt must not be sent")
		})
	}
}
