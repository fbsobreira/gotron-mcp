package tools

import (
	"strings"
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestWalletManager(t *testing.T) *wallet.Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := wallet.NewManager(dir, "test-pass")
	require.NoError(t, err, "NewManager")
	m.SetKeystoreFactory(keystore.ForPathLight)
	t.Cleanup(func() { m.Close() })
	return m
}

func TestCreateWalletTool_Success(t *testing.T) {
	wm := newTestWalletManager(t)
	result := callTool(t, handleCreateWallet(wm), map[string]any{
		"name": "my-wallet",
	})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, "my-wallet", data["name"])
	addr, ok := data["address"].(string)
	assert.True(t, ok && addr != "", "address should be a non-empty string, got %v", data["address"])
	assert.True(t, strings.HasPrefix(addr, "T"), "address should start with T, got %s", addr)
}

func TestCreateWalletTool_EmptyName(t *testing.T) {
	wm := newTestWalletManager(t)
	result := callTool(t, handleCreateWallet(wm), map[string]any{
		"name": "",
	})
	assert.True(t, result.IsError, "expected error for empty name")
}

func TestCreateWalletTool_InvalidName(t *testing.T) {
	wm := newTestWalletManager(t)
	result := callTool(t, handleCreateWallet(wm), map[string]any{
		"name": "..",
	})
	assert.True(t, result.IsError, "expected error for invalid name '..'")
}

func TestListWalletsTool_Empty(t *testing.T) {
	wm := newTestWalletManager(t)
	result := callTool(t, handleListWallets(wm), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, 0, int(data["count"].(float64)))
	wallets, ok := data["wallets"].([]any)
	require.True(t, ok, "wallets should be an array, got %T", data["wallets"])
	assert.Empty(t, wallets)
}

func TestListWalletsTool_WithWallets(t *testing.T) {
	wm := newTestWalletManager(t)

	// Create two wallets via manager directly
	addr1, err := wm.CreateWallet("wallet-one")
	require.NoError(t, err, "CreateWallet wallet-one")
	addr2, err := wm.CreateWallet("wallet-two")
	require.NoError(t, err, "CreateWallet wallet-two")

	result := callTool(t, handleListWallets(wm), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	data := parseJSONResult(t, result)
	assert.Equal(t, 2, int(data["count"].(float64)))

	wallets, ok := data["wallets"].([]any)
	require.True(t, ok, "wallets should be an array, got %T", data["wallets"])
	require.Len(t, wallets, 2)

	// Build a map of name -> address from the result for easy lookup
	found := make(map[string]string)
	for _, w := range wallets {
		wm, ok := w.(map[string]any)
		require.True(t, ok, "wallet entry should be a map, got %T", w)
		name, _ := wm["name"].(string)
		addr, _ := wm["address"].(string)
		found[name] = addr
	}

	assert.Equal(t, addr1, found["wallet-one"])
	assert.Equal(t, addr2, found["wallet-two"])
}
