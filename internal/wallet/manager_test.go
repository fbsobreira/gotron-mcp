package wallet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewManager(dir, "test-passphrase")
	require.NoError(t, err, "NewManager")
	m.SetKeystoreFactory(keystore.ForPathLight)
	t.Cleanup(func() { m.Close() })
	return m
}

func TestNewManager_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wallets")
	// Verify dir does not exist yet
	_, err := os.Stat(dir)
	require.True(t, os.IsNotExist(err), "expected dir to not exist before NewManager")
	m, err := NewManager(dir, "test-pass")
	require.NoError(t, err, "NewManager")
	defer m.Close()
	// Verify dir was created
	info, err := os.Stat(dir)
	require.NoError(t, err, "dir not created")
	require.True(t, info.IsDir(), "expected dir to be a directory")
}

func TestCreateWallet(t *testing.T) {
	m := newTestManager(t)

	addr, err := m.CreateWallet("test-wallet")
	require.NoError(t, err, "CreateWallet")
	require.NotEmpty(t, addr, "expected non-empty address")
	require.True(t, strings.HasPrefix(addr, "T"), "expected address starting with T, got %s", addr)

	wallets, err := m.ListWallets()
	require.NoError(t, err, "ListWallets")
	found := false
	for _, w := range wallets {
		if w.Name == "test-wallet" && w.Address == addr {
			found = true
			break
		}
	}
	require.True(t, found, "created wallet not found in ListWallets; got %v", wallets)
}

func TestCreateWallet_DuplicateName(t *testing.T) {
	m := newTestManager(t)

	_, err := m.CreateWallet("dup")
	require.NoError(t, err, "first create")

	_, err = m.CreateWallet("dup")
	require.Error(t, err, "expected error for duplicate name")
	assert.Contains(t, err.Error(), "already exists")
}

func TestCreateWallet_InvalidName(t *testing.T) {
	m := newTestManager(t)

	tests := []struct {
		name string
		desc string
	}{
		{"", "empty name"},
		{"..", "dot-dot"},
		{"foo/bar", "slash in name"},
		{"my wallet", "space in name"},
		{"bad!name", "special char in name"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, err := m.CreateWallet(tt.name)
			require.Error(t, err, "expected error for %s", tt.desc)
		})
	}
}

func TestListWallets_Empty(t *testing.T) {
	m := newTestManager(t)

	wallets, err := m.ListWallets()
	require.NoError(t, err, "ListWallets")
	require.Empty(t, wallets, "expected empty list")
}

func TestListWallets(t *testing.T) {
	m := newTestManager(t)

	addr1, err := m.CreateWallet("wallet-one")
	require.NoError(t, err, "CreateWallet wallet-one")
	addr2, err := m.CreateWallet("wallet-two")
	require.NoError(t, err, "CreateWallet wallet-two")

	wallets, err := m.ListWallets()
	require.NoError(t, err, "ListWallets")
	require.Len(t, wallets, 2)

	addrMap := make(map[string]string)
	for _, w := range wallets {
		addrMap[w.Name] = w.Address
	}
	require.Equal(t, addr1, addrMap["wallet-one"], "wallet-one address mismatch")
	require.Equal(t, addr2, addrMap["wallet-two"], "wallet-two address mismatch")
}

func TestGetSigner(t *testing.T) {
	m := newTestManager(t)

	addr, err := m.CreateWallet("signer-test")
	require.NoError(t, err, "CreateWallet")

	s, err := m.GetSigner("signer-test")
	require.NoError(t, err, "GetSigner")

	signerAddr := s.Address().String()
	require.Equal(t, addr, signerAddr, "signer address mismatch")
}

func TestGetSigner_ByAddress(t *testing.T) {
	m := newTestManager(t)
	addr, err := m.CreateWallet("addr-test")
	require.NoError(t, err, "CreateWallet")
	s, err := m.GetSigner(addr) // use address instead of name
	require.NoError(t, err, "GetSigner by address")
	assert.Equal(t, addr, s.Address().String())
}

func TestCreateWallet_Concurrent(t *testing.T) {
	m := newTestManager(t)
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := m.CreateWallet(fmt.Sprintf("wallet-%d", idx))
			if err != nil {
				errors <- err
			}
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		assert.NoError(t, err, "concurrent CreateWallet failed")
	}
	wallets, err := m.ListWallets()
	require.NoError(t, err, "ListWallets")
	assert.Len(t, wallets, 5)
}

func TestGetSigner_NotFound(t *testing.T) {
	m := newTestManager(t)

	_, err := m.GetSigner("nonexistent")
	require.Error(t, err, "expected error for unknown wallet")
	assert.Contains(t, err.Error(), "not found")
}
