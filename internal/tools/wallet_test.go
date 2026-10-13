package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	c "github.com/fbsobreira/gotron-sdk/pkg/common"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/fbsobreira/gotron-sdk/pkg/pqc"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestWalletManager(t *testing.T) *wallet.Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := wallet.NewManager(dir, "test-pass")
	require.NoError(t, err, "NewManager")
	m.SetKeystoreFactory(keystore.ForPathLight)
	m.SetPQScryptParams(keystore.LightScryptN, keystore.LightScryptP)
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

func TestCreateWalletTool_KeyType(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantKey string
		wantPQ  bool
	}{
		{"default is ecdsa", map[string]any{"name": "w"}, "ecdsa", false},
		{"explicit ecdsa", map[string]any{"name": "w", "key_type": "ecdsa"}, "ecdsa", false},
		{"empty is ecdsa", map[string]any{"name": "w", "key_type": ""}, "ecdsa", false},
		{"ml_dsa_44", map[string]any{"name": "w", "key_type": "ml_dsa_44"}, "ml_dsa_44", true},
		{"ml-dsa-44 spelling", map[string]any{"name": "w", "key_type": "ML-DSA-44"}, "ml_dsa_44", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wm := newTestWalletManager(t)
			result := callTool(t, handleCreateWallet(wm), tt.args)
			require.False(t, result.IsError, "expected success, got error: %v", result.Content)

			data := parseJSONResult(t, result)
			assert.Equal(t, "w", data["name"])
			assert.Equal(t, tt.wantKey, data["key_type"])
			addr, _ := data["address"].(string)
			assert.True(t, strings.HasPrefix(addr, "T"), "address should start with T, got %q", addr)

			notes, hasNotes := data["notes"].([]any)
			if !tt.wantPQ {
				assert.False(t, hasNotes, "ecdsa wallets must not carry PQ notes")
				return
			}
			require.True(t, hasNotes, "PQ wallets must carry notes, got %v", data)
			require.Len(t, notes, 3)
			assert.Contains(t, notes[0], "get_pq_capabilities")
			assert.Contains(t, notes[1], "must receive TRX")
			assert.Contains(t, notes[2], "10-22x larger")

			info, err := wm.LookupWallet("w")
			require.NoError(t, err)
			assert.Equal(t, addr, info.Address)
		})
	}
}

func TestCreateWalletTool_InvalidKeyType(t *testing.T) {
	for _, kt := range []string{"rsa", "ed25519", "dilithium", "ml_dsa_65"} {
		t.Run(kt, func(t *testing.T) {
			wm := newTestWalletManager(t)
			result := callTool(t, handleCreateWallet(wm), map[string]any{"name": "w", "key_type": kt})
			require.True(t, result.IsError, "expected error for key_type %q", kt)
			assert.Contains(t, toolErrText(t, result), "create_wallet:")

			wallets, err := wm.ListWallets()
			require.NoError(t, err)
			assert.Empty(t, wallets, "no wallet may be created on invalid key_type")
		})
	}
}

func TestCreateWalletTool_NonStringKeyType(t *testing.T) {
	wm := newTestWalletManager(t)
	result := callTool(t, handleCreateWallet(wm), map[string]any{"name": "w", "key_type": 44})
	// A non-string key_type must never silently fall back to ecdsa.
	if !result.IsError {
		data := parseJSONResult(t, result)
		t.Fatalf("expected error for non-string key_type, created %v", data)
	}
}

func TestCreateWalletTool_FNDSA512(t *testing.T) {
	wm := newTestWalletManager(t)
	result := callTool(t, handleCreateWallet(wm), map[string]any{"name": "fn", "key_type": "fn_dsa_512"})

	if pqc.SchemeAvailable(core.PQScheme_FN_DSA_512) != nil {
		require.True(t, result.IsError, "expected error on a build without Falcon")
		assert.Equal(t,
			"create_wallet: this build cannot create FN-DSA-512 keys; it needs the gotron-mcp-pq build, or build locally with liboqs installed: CGO_ENABLED=1 go build -tags falcon ./cmd/gotron-mcp. ML-DSA-44 works in every build.",
			toolErrText(t, result))
		wallets, err := wm.ListWallets()
		require.NoError(t, err)
		assert.Empty(t, wallets, "no wallet may be created")
		return
	}

	require.False(t, result.IsError, "expected success on a falcon build, got: %v", result.Content)
	data := parseJSONResult(t, result)
	assert.Equal(t, "fn_dsa_512", data["key_type"])
	assert.NotEmpty(t, data["notes"])
}

func TestListWalletsTool_KeyType(t *testing.T) {
	wm := newTestWalletManager(t)
	ecAddr, err := wm.CreateWallet("classic")
	require.NoError(t, err)
	pqAddr, err := wm.CreateWalletWithKeyType("quantum", wallet.KeyTypeMLDSA44)
	require.NoError(t, err)

	result := callTool(t, handleListWallets(wm), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
	data := parseJSONResult(t, result)
	assert.Equal(t, 2, int(data["count"].(float64)))

	wallets, ok := data["wallets"].([]any)
	require.True(t, ok)
	require.Len(t, wallets, 2)
	got := map[string]map[string]any{}
	for _, w := range wallets {
		item, ok := w.(map[string]any)
		require.True(t, ok)
		got[item["name"].(string)] = item
	}
	assert.Equal(t, "ecdsa", got["classic"]["key_type"])
	assert.Equal(t, ecAddr, got["classic"]["address"])
	assert.Equal(t, "ml_dsa_44", got["quantum"]["key_type"])
	assert.Equal(t, pqAddr, got["quantum"]["address"])
	for name, item := range got {
		assert.NotContains(t, item, "duplicate_name", "%s must not be flagged duplicate", name)
	}
}

// newCollisionManager returns a manager holding an ML-DSA-44 wallet and an
// ECDSA wallet that share the name "dup", as an older keystore dir would,
// plus the PQ and ECDSA addresses. LookupWallet reports the name, and both
// addresses, as ErrAmbiguousWalletName.
func newCollisionManager(t *testing.T) (*wallet.Manager, string, string) {
	t.Helper()
	dirA := t.TempDir()
	mA, err := wallet.NewManager(dirA, "test-pass")
	require.NoError(t, err)
	mA.SetKeystoreFactory(keystore.ForPathLight)
	mA.SetPQScryptParams(keystore.LightScryptN, keystore.LightScryptP)
	t.Cleanup(func() { mA.Close() })
	pqAddr, err := mA.CreateWalletWithKeyType("dup", wallet.KeyTypeMLDSA44)
	require.NoError(t, err)

	dirB := t.TempDir()
	mB, err := wallet.NewManager(dirB, "test-pass")
	require.NoError(t, err)
	mB.SetKeystoreFactory(keystore.ForPathLight)
	ecdsaAddr, err := mB.CreateWallet("dup")
	require.NoError(t, err)
	mB.Close()
	require.NoError(t, os.Rename(
		filepath.Join(dirB, c.DefaultConfigAccountAliasesDirName, "dup"),
		filepath.Join(dirA, c.DefaultConfigAccountAliasesDirName, "dup"),
	))
	_, err = mA.LookupWallet("dup")
	require.ErrorIs(t, err, wallet.ErrAmbiguousWalletName, "fixture: name must be ambiguous")
	return mA, pqAddr, ecdsaAddr
}

func TestListWalletsTool_DuplicateName(t *testing.T) {
	dirA := t.TempDir()
	mA, err := wallet.NewManager(dirA, "test-pass")
	require.NoError(t, err)
	mA.SetKeystoreFactory(keystore.ForPathLight)
	mA.SetPQScryptParams(keystore.LightScryptN, keystore.LightScryptP)
	t.Cleanup(func() { mA.Close() })
	_, err = mA.CreateWalletWithKeyType("dup", wallet.KeyTypeMLDSA44)
	require.NoError(t, err)
	_, err = mA.CreateWallet("solo")
	require.NoError(t, err)

	// Plant an ECDSA wallet of the same name, as an older keystore dir would.
	dirB := t.TempDir()
	mB, err := wallet.NewManager(dirB, "test-pass")
	require.NoError(t, err)
	mB.SetKeystoreFactory(keystore.ForPathLight)
	_, err = mB.CreateWallet("dup")
	require.NoError(t, err)
	mB.Close()
	require.NoError(t, os.Rename(
		filepath.Join(dirB, c.DefaultConfigAccountAliasesDirName, "dup"),
		filepath.Join(dirA, c.DefaultConfigAccountAliasesDirName, "dup"),
	))

	result := callTool(t, handleListWallets(mA), map[string]any{})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)
	data := parseJSONResult(t, result)
	wallets, ok := data["wallets"].([]any)
	require.True(t, ok)
	require.Len(t, wallets, 3)

	kinds := map[string]bool{}
	for _, w := range wallets {
		item := w.(map[string]any)
		name := item["name"].(string)
		if name == "dup" {
			assert.Equal(t, true, item["duplicate_name"])
			kinds[item["key_type"].(string)] = true
		} else {
			assert.NotContains(t, item, "duplicate_name")
		}
	}
	assert.Equal(t, map[string]bool{"ecdsa": true, "ml_dsa_44": true}, kinds)
}

// toolErrText returns the text of a tool result's first content item.
func toolErrText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "empty result content")
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	return tc.Text
}
