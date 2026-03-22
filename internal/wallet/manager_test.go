package wallet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewManager(dir, "test-passphrase")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m.SetKeystoreFactory(keystore.ForPathLight)
	t.Cleanup(func() { m.Close() })
	return m
}

func TestNewManager_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wallets")
	// Verify dir does not exist yet
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("expected dir to not exist before NewManager")
	}
	m, err := NewManager(dir, "test-pass")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Close()
	// Verify dir was created
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected dir to be a directory")
	}
}

func TestCreateWallet(t *testing.T) {
	m := newTestManager(t)

	addr, err := m.CreateWallet("test-wallet")
	if err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}
	if addr == "" {
		t.Fatal("expected non-empty address")
	}
	if !strings.HasPrefix(addr, "T") {
		t.Fatalf("expected address starting with T, got %s", addr)
	}

	wallets, err := m.ListWallets()
	if err != nil {
		t.Fatalf("ListWallets: %v", err)
	}
	found := false
	for _, w := range wallets {
		if w.Name == "test-wallet" && w.Address == addr {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created wallet not found in ListWallets; got %v", wallets)
	}
}

func TestCreateWallet_DuplicateName(t *testing.T) {
	m := newTestManager(t)

	if _, err := m.CreateWallet("dup"); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := m.CreateWallet("dup")
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
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
			if err == nil {
				t.Fatalf("expected error for %s", tt.desc)
			}
		})
	}
}

func TestListWallets_Empty(t *testing.T) {
	m := newTestManager(t)

	wallets, err := m.ListWallets()
	if err != nil {
		t.Fatalf("ListWallets: %v", err)
	}
	if len(wallets) != 0 {
		t.Fatalf("expected empty list, got %v", wallets)
	}
}

func TestListWallets(t *testing.T) {
	m := newTestManager(t)

	addr1, err := m.CreateWallet("wallet-one")
	if err != nil {
		t.Fatalf("CreateWallet wallet-one: %v", err)
	}
	addr2, err := m.CreateWallet("wallet-two")
	if err != nil {
		t.Fatalf("CreateWallet wallet-two: %v", err)
	}

	wallets, err := m.ListWallets()
	if err != nil {
		t.Fatalf("ListWallets: %v", err)
	}
	if len(wallets) != 2 {
		t.Fatalf("expected 2 wallets, got %d", len(wallets))
	}

	addrMap := make(map[string]string)
	for _, w := range wallets {
		addrMap[w.Name] = w.Address
	}
	if addrMap["wallet-one"] != addr1 {
		t.Fatalf("wallet-one address mismatch: got %s, want %s", addrMap["wallet-one"], addr1)
	}
	if addrMap["wallet-two"] != addr2 {
		t.Fatalf("wallet-two address mismatch: got %s, want %s", addrMap["wallet-two"], addr2)
	}
}

func TestGetSigner(t *testing.T) {
	m := newTestManager(t)

	addr, err := m.CreateWallet("signer-test")
	if err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}

	s, err := m.GetSigner("signer-test")
	if err != nil {
		t.Fatalf("GetSigner: %v", err)
	}

	signerAddr := s.Address().String()
	if signerAddr != addr {
		t.Fatalf("signer address mismatch: got %s, want %s", signerAddr, addr)
	}
}

func TestGetSigner_ByAddress(t *testing.T) {
	m := newTestManager(t)
	addr, err := m.CreateWallet("addr-test")
	if err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}
	s, err := m.GetSigner(addr) // use address instead of name
	if err != nil {
		t.Fatalf("GetSigner by address: %v", err)
	}
	if s.Address().String() != addr {
		t.Errorf("signer address = %s, want %s", s.Address().String(), addr)
	}
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
		t.Errorf("concurrent CreateWallet failed: %v", err)
	}
	wallets, err := m.ListWallets()
	if err != nil {
		t.Fatalf("ListWallets: %v", err)
	}
	if len(wallets) != 5 {
		t.Errorf("expected 5 wallets, got %d", len(wallets))
	}
}

func TestGetSigner_NotFound(t *testing.T) {
	m := newTestManager(t)

	_, err := m.GetSigner("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown wallet")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
