package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/protobuf/proto"
)

func newTestSignSetup(t *testing.T, mock *mockWalletServer) (*wallet.Manager, *nodepool.Pool) {
	t.Helper()
	wm, err := wallet.NewManager(t.TempDir(), "test-pass")
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatalf("failed to marshal test tx: %v", err)
	}
	return hex.EncodeToString(txBytes)
}

// --- sign_transaction tests ---

func TestSignTransaction_EmptyHex(t *testing.T) {
	wm, _ := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(wm), map[string]any{
		"transaction_hex": "",
		"wallet":          "test",
	})
	if !result.IsError {
		t.Error("expected error for empty transaction_hex")
	}
}

func TestSignTransaction_TooLong(t *testing.T) {
	wm, _ := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(wm), map[string]any{
		"transaction_hex": strings.Repeat("aa", maxHexInputLen+1),
		"wallet":          "test",
	})
	if !result.IsError {
		t.Error("expected error for too-long transaction_hex")
	}
}

func TestSignTransaction_EmptyWallet(t *testing.T) {
	wm, _ := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(wm), map[string]any{
		"transaction_hex": "0a0208",
		"wallet":          "",
	})
	if !result.IsError {
		t.Error("expected error for empty wallet")
	}
}

func TestSignTransaction_WalletNotFound(t *testing.T) {
	wm, _ := newTestSignSetup(t, &mockWalletServer{})
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignTransaction(wm), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "nonexistent",
	})
	if !result.IsError {
		t.Error("expected error for wallet not found")
	}
	found := false
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			if strings.Contains(tc.Text, "sign_transaction:") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected error text containing 'sign_transaction:', got: %+v", result.Content)
	}
}

func TestSignTransaction_InvalidHex(t *testing.T) {
	wm, _ := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignTransaction(wm), map[string]any{
		"transaction_hex": "not-hex",
		"wallet":          "test",
	})
	if !result.IsError {
		t.Error("expected error for invalid hex")
	}
}

func TestSignTransaction_Success(t *testing.T) {
	t.Run("by name", func(t *testing.T) {
		wm, _ := newTestSignSetup(t, &mockWalletServer{})

		// Create a wallet
		_, err := wm.CreateWallet("test-signer")
		if err != nil {
			t.Fatalf("CreateWallet: %v", err)
		}

		txHex := buildTestTxHex(t)
		result := callTool(t, handleSignTransaction(wm), map[string]any{
			"transaction_hex": txHex,
			"wallet":          "test-signer",
		})
		if result.IsError {
			t.Fatalf("expected success, got error: %v", result.Content)
		}

		data := parseJSONResult(t, result)
		signedHex, ok := data["signed_transaction_hex"].(string)
		if !ok || signedHex == "" {
			t.Errorf("signed_transaction_hex should be a non-empty string, got %v", data["signed_transaction_hex"])
		}
		if data["wallet"] != "test-signer" {
			t.Errorf("wallet = %v, want test-signer", data["wallet"])
		}
	})

	t.Run("by address", func(t *testing.T) {
		wm, _ := newTestSignSetup(t, &mockWalletServer{})

		// Create a wallet and use the returned address to sign
		addr, err := wm.CreateWallet("addr-signer")
		if err != nil {
			t.Fatalf("CreateWallet: %v", err)
		}

		txHex := buildTestTxHex(t)
		result := callTool(t, handleSignTransaction(wm), map[string]any{
			"transaction_hex": txHex,
			"wallet":          addr,
		})
		if result.IsError {
			t.Fatalf("expected success, got error: %v", result.Content)
		}

		data := parseJSONResult(t, result)
		signedHex, ok := data["signed_transaction_hex"].(string)
		if !ok || signedHex == "" {
			t.Errorf("signed_transaction_hex should be a non-empty string, got %v", data["signed_transaction_hex"])
		}
		if data["wallet"] != addr {
			t.Errorf("wallet = %v, want %s", data["wallet"], addr)
		}
	})
}

// --- sign_and_broadcast tests ---

func TestSignAndBroadcast_InvalidHex(t *testing.T) {
	wm, pool := newTestSignSetup(t, &mockWalletServer{})
	result := callTool(t, handleSignAndBroadcast(pool, wm), map[string]any{
		"transaction_hex": "not-hex",
		"wallet":          "test",
	})
	if !result.IsError {
		t.Error("expected error for invalid hex")
	}
}

func TestSignAndBroadcast_WalletNotFound(t *testing.T) {
	wm, pool := newTestSignSetup(t, &mockWalletServer{})
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndBroadcast(pool, wm), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "nonexistent",
	})
	if !result.IsError {
		t.Error("expected error for wallet not found")
	}
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
	if err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}

	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndBroadcast(pool, wm), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "broadcast-signer",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["success"] != true {
		t.Errorf("success = %v, want true", data["success"])
	}
	txid, ok := data["txid"].(string)
	if !ok || txid == "" {
		t.Error("txid should be a non-empty string")
	}
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
	if err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}

	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndConfirm(pool, wm), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "confirm-signer",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["success"] != true {
		t.Errorf("success = %v, want true", data["success"])
	}
	if data["confirmed"] != true {
		t.Errorf("confirmed = %v, want true", data["confirmed"])
	}
	if data["block_number"].(float64) != 12345678 {
		t.Errorf("block_number = %v, want 12345678", data["block_number"])
	}
	txid, ok := data["txid"].(string)
	if !ok || txid == "" {
		t.Error("txid should be a non-empty string")
	}
}

// --- broadcast_transaction tests (kept as-is) ---

func TestBroadcastTransaction_EmptyHex(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleBroadcastTransaction(pool), map[string]any{
		"signed_transaction_hex": "",
	})
	if !result.IsError {
		t.Error("expected error for empty hex")
	}
}

func TestBroadcastTransaction_TooLong(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleBroadcastTransaction(pool), map[string]any{
		"signed_transaction_hex": strings.Repeat("aa", maxHexInputLen+1),
	})
	if !result.IsError {
		t.Error("expected error for too-long hex")
	}
}

func TestBroadcastTransaction_InvalidHex(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleBroadcastTransaction(pool), map[string]any{
		"signed_transaction_hex": "zzzz",
	})
	if !result.IsError {
		t.Error("expected error for invalid hex")
	}
}

func TestBroadcastTransaction_InvalidProto(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleBroadcastTransaction(pool), map[string]any{
		"signed_transaction_hex": "0f",
	})
	if !result.IsError {
		t.Fatal("expected error for invalid protobuf payload")
	}
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
	if !found {
		t.Errorf("expected parse or broadcast error, got: %+v", result.Content)
	}
}

func TestSignAndBroadcast_BroadcastFails(t *testing.T) {
	mock := &mockWalletServer{
		BroadcastTransactionFunc: func(_ context.Context, _ *core.Transaction) (*api.Return, error) {
			return nil, fmt.Errorf("node unavailable")
		},
	}
	wm, pool := newTestSignSetup(t, mock)
	if _, err := wm.CreateWallet("test-wallet"); err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndBroadcast(pool, wm), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "test-wallet",
	})
	if !result.IsError {
		t.Error("expected error when broadcast fails")
	}
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
	if _, err := wm.CreateWallet("test-wallet"); err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndBroadcast(pool, wm), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "test-wallet",
	})
	// SDK BroadcastCtx returns error when Result is false
	if !result.IsError {
		t.Error("expected error when broadcast is rejected")
	}
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
	if _, err := wm.CreateWallet("test-wallet"); err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}
	txHex := buildTestTxHex(t)

	// Use a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"transaction_hex": txHex,
		"wallet":          "test-wallet",
	}
	handler := handleSignAndConfirm(pool, wm)
	result, err := handler(ctx, req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for cancelled context")
	}
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
	if _, err := wm.CreateWallet("test-wallet"); err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}
	txHex := buildTestTxHex(t)
	result := callTool(t, handleSignAndConfirm(pool, wm), map[string]any{
		"transaction_hex": txHex,
		"wallet":          "test-wallet",
	})
	if !result.IsError {
		t.Error("expected error for RPC failure")
	}
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
	if err != nil {
		t.Fatalf("failed to marshal tx: %v", err)
	}

	result := callTool(t, handleBroadcastTransaction(pool), map[string]any{
		"signed_transaction_hex": hex.EncodeToString(txBytes),
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["success"] != true {
		t.Errorf("success = %v, want true", data["success"])
	}
	if data["transaction_id"] == nil || data["transaction_id"] == "" {
		t.Error("transaction_id should not be empty")
	}
}
