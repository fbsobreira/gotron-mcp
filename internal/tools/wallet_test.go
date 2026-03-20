package tools

import (
	"strings"
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
)

func newTestWalletManager(t *testing.T) *wallet.Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := wallet.NewManager(dir, "test-pass")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m.SetKeystoreFactory(keystore.ForPathLight)
	t.Cleanup(func() { m.Close() })
	return m
}

func TestCreateWalletTool_Success(t *testing.T) {
	wm := newTestWalletManager(t)
	result := callTool(t, handleCreateWallet(wm), map[string]any{
		"name": "my-wallet",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["name"] != "my-wallet" {
		t.Errorf("name = %v, want my-wallet", data["name"])
	}
	addr, ok := data["address"].(string)
	if !ok || addr == "" {
		t.Errorf("address should be a non-empty string, got %v", data["address"])
	}
	if !strings.HasPrefix(addr, "T") {
		t.Errorf("address should start with T, got %s", addr)
	}
}

func TestCreateWalletTool_EmptyName(t *testing.T) {
	wm := newTestWalletManager(t)
	result := callTool(t, handleCreateWallet(wm), map[string]any{
		"name": "",
	})
	if !result.IsError {
		t.Error("expected error for empty name")
	}
}

func TestCreateWalletTool_InvalidName(t *testing.T) {
	wm := newTestWalletManager(t)
	result := callTool(t, handleCreateWallet(wm), map[string]any{
		"name": "..",
	})
	if !result.IsError {
		t.Error("expected error for invalid name '..'")
	}
}

func TestListWalletsTool_Empty(t *testing.T) {
	wm := newTestWalletManager(t)
	result := callTool(t, handleListWallets(wm), map[string]any{})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if count := int(data["count"].(float64)); count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	wallets, ok := data["wallets"].([]any)
	if !ok {
		t.Fatalf("wallets should be an array, got %T", data["wallets"])
	}
	if len(wallets) != 0 {
		t.Errorf("wallets length = %d, want 0", len(wallets))
	}
}

func TestListWalletsTool_WithWallets(t *testing.T) {
	wm := newTestWalletManager(t)

	// Create two wallets via manager directly
	addr1, err := wm.CreateWallet("wallet-one")
	if err != nil {
		t.Fatalf("CreateWallet wallet-one: %v", err)
	}
	addr2, err := wm.CreateWallet("wallet-two")
	if err != nil {
		t.Fatalf("CreateWallet wallet-two: %v", err)
	}

	result := callTool(t, handleListWallets(wm), map[string]any{})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if count := int(data["count"].(float64)); count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	wallets, ok := data["wallets"].([]any)
	if !ok {
		t.Fatalf("wallets should be an array, got %T", data["wallets"])
	}
	if len(wallets) != 2 {
		t.Fatalf("wallets length = %d, want 2", len(wallets))
	}

	// Build a map of name -> address from the result for easy lookup
	found := make(map[string]string)
	for _, w := range wallets {
		wm, ok := w.(map[string]any)
		if !ok {
			t.Fatalf("wallet entry should be a map, got %T", w)
		}
		name, _ := wm["name"].(string)
		addr, _ := wm["address"].(string)
		found[name] = addr
	}

	if found["wallet-one"] != addr1 {
		t.Errorf("wallet-one address = %s, want %s", found["wallet-one"], addr1)
	}
	if found["wallet-two"] != addr2 {
		t.Errorf("wallet-two address = %s, want %s", found["wallet-two"], addr2)
	}
}
