package wallet

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/fbsobreira/gotron-sdk/pkg/signer"
	"github.com/fbsobreira/gotron-sdk/pkg/store"
)

// Manager manages named wallets backed by the SDK's store package.
type Manager struct {
	store      *store.Store
	passphrase string
}

// WalletInfo holds the name and address of a wallet.
type WalletInfo struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// NewManager creates a Manager rooted at the given directory.
// The passphrase is used to encrypt/decrypt all wallet keys.
func NewManager(dir, passphrase string) (*Manager, error) {
	s := store.NewStore(dir)
	s.InitConfigDir()
	return &Manager{store: s, passphrase: passphrase}, nil
}

// CreateWallet creates a new named wallet and returns its address.
func (m *Manager) CreateWallet(name string) (string, error) {
	if err := validateWalletName(name); err != nil {
		return "", err
	}
	if m.store.DoesNamedAccountExist(name) {
		return "", fmt.Errorf("wallet %q already exists", name)
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}

	ks := m.store.FromAccountName(name)
	defer func() {
		ks.Close()
		m.store.Forget(ks)
	}()

	acct, err := ks.ImportECDSA(key, m.passphrase)
	if err != nil {
		return "", fmt.Errorf("import key: %w", err)
	}

	return acct.Address.String(), nil
}

// ListWallets returns all wallets with their names and addresses.
func (m *Manager) ListWallets() ([]WalletInfo, error) {
	names := m.store.LocalAccounts()
	wallets := make([]WalletInfo, 0, len(names))
	for _, name := range names {
		addr, err := m.store.AddressFromAccountName(name)
		if err != nil {
			continue // skip wallets that can't be read
		}
		wallets = append(wallets, WalletInfo{Name: name, Address: addr})
	}
	return wallets, nil
}

// GetSigner returns a Signer for the named wallet or address.
// The wallet is unlocked with the manager's passphrase.
func (m *Manager) GetSigner(nameOrAddress string) (signer.Signer, error) {
	addr, err := m.resolveAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}

	ks, acct, err := m.store.UnlockedKeystore(addr, m.passphrase)
	if err != nil {
		return nil, fmt.Errorf("unlock wallet: %w", err)
	}

	return signer.NewKeystoreSigner(ks, *acct), nil
}

// SetKeystoreFactory sets the keystore factory function (for testing).
func (m *Manager) SetKeystoreFactory(fn func(string) *keystore.KeyStore) {
	m.store.SetKeystoreFactory(fn)
}

// Close closes all tracked keystores.
func (m *Manager) Close() {
	m.store.CloseAll()
}

// resolveAddress converts a wallet name to its address, or returns the
// input if it's already a base58 address (starts with T).
func (m *Manager) resolveAddress(nameOrAddress string) (string, error) {
	if strings.HasPrefix(nameOrAddress, "T") && len(nameOrAddress) == 34 {
		return nameOrAddress, nil
	}
	addr, err := m.store.AddressFromAccountName(nameOrAddress)
	if err != nil {
		return "", fmt.Errorf("wallet %q not found", nameOrAddress)
	}
	return addr, nil
}

// validateWalletName checks that the name is safe for filesystem use.
func validateWalletName(name string) error {
	if name == "" {
		return fmt.Errorf("wallet name is required")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) || filepath.IsAbs(name) {
		return fmt.Errorf("invalid wallet name %q", name)
	}
	return nil
}
