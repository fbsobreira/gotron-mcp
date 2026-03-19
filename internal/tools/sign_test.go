package tools

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/protobuf/proto"
)

func TestSignTransaction_EmptyHex(t *testing.T) {
	handler := handleSignTransaction(t.TempDir())
	result := callTool(t, handler, map[string]any{
		"transaction_hex": "",
		"signer":          "TSomeAddr",
		"passphrase":      "pass",
	})
	if !result.IsError {
		t.Error("expected error for empty transaction_hex")
	}
}

func TestSignTransaction_TooLong(t *testing.T) {
	handler := handleSignTransaction(t.TempDir())
	result := callTool(t, handler, map[string]any{
		"transaction_hex": strings.Repeat("aa", maxHexInputLen+1),
		"signer":          "TSomeAddr",
		"passphrase":      "pass",
	})
	if !result.IsError {
		t.Error("expected error for too-long transaction_hex")
	}
}

func TestSignTransaction_EmptySigner(t *testing.T) {
	handler := handleSignTransaction(t.TempDir())
	result := callTool(t, handler, map[string]any{
		"transaction_hex": "0a0208",
		"signer":          "",
		"passphrase":      "pass",
	})
	if !result.IsError {
		t.Error("expected error for empty signer")
	}
}

func TestSignTransaction_EmptyPassphrase(t *testing.T) {
	handler := handleSignTransaction(t.TempDir())
	result := callTool(t, handler, map[string]any{
		"transaction_hex": "0a0208",
		"signer":          "TSomeAddr",
		"passphrase":      "",
	})
	if !result.IsError {
		t.Error("expected error for empty passphrase")
	}
}

func TestSignTransaction_InvalidHex(t *testing.T) {
	handler := handleSignTransaction(t.TempDir())
	result := callTool(t, handler, map[string]any{
		"transaction_hex": "not-hex",
		"signer":          "TSomeAddr",
		"passphrase":      "pass",
	})
	if !result.IsError {
		t.Error("expected error for invalid hex")
	}
}

func TestSignTransaction_InvalidProto(t *testing.T) {
	handler := handleSignTransaction(t.TempDir())
	result := callTool(t, handler, map[string]any{
		"transaction_hex": "0f",
		"signer":          "TSomeAddr",
		"passphrase":      "pass",
	})
	if !result.IsError {
		t.Fatal("expected error for invalid protobuf payload")
	}
	found := false
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			if strings.Contains(tc.Text, "failed to parse transaction") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected proto parse error, got: %+v", result.Content)
	}
}

func TestSignTransaction_AccountNotFound(t *testing.T) {
	handler := handleSignTransaction(t.TempDir())
	// Create valid proto bytes for a minimal transaction
	tx := &core.Transaction{
		RawData: &core.TransactionRaw{},
	}
	txBytes, err := proto.Marshal(tx)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	result := callTool(t, handler, map[string]any{
		"transaction_hex": hex.EncodeToString(txBytes),
		"signer":          "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"passphrase":      "pass",
	})
	if !result.IsError {
		t.Error("expected error for account not found in keystore")
	}
	found := false
	for _, c := range result.Content {
		tc, ok := c.(mcp.TextContent)
		if !ok {
			continue
		}
		if strings.Contains(tc.Text, "not found in keystore") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error text containing 'not found in keystore', got: %+v", result.Content)
	}
}

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
