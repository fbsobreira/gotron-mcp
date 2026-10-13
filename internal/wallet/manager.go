package wallet

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/fbsobreira/gotron-sdk/pkg/keys"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/fbsobreira/gotron-sdk/pkg/pqc"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/fbsobreira/gotron-sdk/pkg/signer"
	"github.com/fbsobreira/gotron-sdk/pkg/store"
)

// Manager manages named wallets backed by the SDK's store package.
// ECDSA wallets live in the store's account keystores; post-quantum
// (TIP-899) wallets live in its pq-keys folder. Wallet names are unique
// across both.
type Manager struct {
	store      *store.Store
	passphrase string
	mu         sync.Mutex
	unlockedKS []*keystore.KeyStore

	// Scrypt parameters for new post-quantum keystores.
	pqScryptN int
	pqScryptP int
}

// WalletInfo holds the name, address and key type of a wallet.
type WalletInfo struct {
	Name    string  `json:"name"`
	Address string  `json:"address"`
	KeyType KeyType `json:"key_type"`
	// Duplicate is set by ListWallets on both entries of a name that exists
	// as both an ECDSA and a post-quantum wallet. Such a name cannot be used
	// to look up, resolve or sign until one of the wallets is renamed.
	Duplicate bool `json:"duplicate_name,omitempty"`
}

var (
	// ErrWalletNotFound is wrapped by errors for a name or address that is
	// not a local wallet.
	ErrWalletNotFound = errors.New("wallet not found")
	// ErrAmbiguousWalletName is wrapped by errors for a wallet name that
	// exists as both an ECDSA and a post-quantum wallet.
	ErrAmbiguousWalletName = errors.New("wallet name exists as both an ECDSA and a post-quantum wallet")
)

// walletError carries a message and a sentinel for errors.Is, so the
// message stays as callers already read it.
type walletError struct {
	msg  string
	kind error
}

func (e *walletError) Error() string { return e.msg }
func (e *walletError) Unwrap() error { return e.kind }

func errWalletNotFound(nameOrAddress string) error {
	return &walletError{msg: fmt.Sprintf("wallet %q not found", nameOrAddress), kind: ErrWalletNotFound}
}

func errAmbiguousName(name string) error {
	return &walletError{
		msg:  fmt.Sprintf("wallet name %q exists as both an ECDSA and a post-quantum wallet; rename one", name),
		kind: ErrAmbiguousWalletName,
	}
}

// pqKeyDone, when set, is called with every post-quantum private key the
// manager generated or decrypted, right after the key is destroyed. It is
// nil in production; tests set it to check that keys are destroyed.
var pqKeyDone func(pqc.PrivateKey)

// destroyPQKey destroys k and reports it to pqKeyDone.
func destroyPQKey(k pqc.PrivateKey) {
	k.Destroy()
	if pqKeyDone != nil {
		pqKeyDone(k)
	}
}

// NewManager creates a Manager rooted at the given directory.
// The passphrase is used to encrypt/decrypt all wallet keys.
func NewManager(dir, passphrase string) (*Manager, error) {
	s := store.NewStore(dir)
	s.InitConfigDir()
	return &Manager{
		store:      s,
		passphrase: passphrase,
		pqScryptN:  keystore.StandardScryptN,
		pqScryptP:  keystore.StandardScryptP,
	}, nil
}

// SetPQScryptParams sets the scrypt parameters used to encrypt new
// post-quantum keystores (for testing; the default is the standard strength).
func (m *Manager) SetPQScryptParams(n, p int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pqScryptN = n
	m.pqScryptP = p
}

// CreateWallet creates a new named ECDSA wallet and returns its address.
func (m *Manager) CreateWallet(name string) (string, error) {
	return m.CreateWalletWithKeyType(name, KeyTypeECDSA)
}

// CreateWalletWithKeyType creates a new named wallet of the given key type
// and returns its address. For a post-quantum key type the scheme must be
// available in this build; otherwise the error wraps the pqc error (for
// example pqc.ErrFalconUnavailable) and nothing is created.
func (m *Manager) CreateWalletWithKeyType(name string, kt KeyType) (string, error) {
	if err := validateWalletName(name); err != nil {
		return "", err
	}
	scheme, isPQ := kt.PQScheme()
	if !isPQ && kt != KeyTypeECDSA {
		return "", fmt.Errorf("unsupported key type %q", kt)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// NameExists covers both ECDSA and post-quantum accounts.
	if m.store.NameExists(name) {
		return "", fmt.Errorf("wallet %q already exists", name)
	}

	if isPQ {
		return m.createPQWallet(name, scheme)
	}

	key, err := keys.GenerateKey()
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	defer keys.ZeroPrivateKey(key)

	ks := m.store.FromAccountName(name)
	defer func() {
		ks.Close()
		m.store.Forget(ks)
	}()

	acct, err := ks.ImportECDSA(key.ToECDSA(), m.passphrase)
	if err != nil {
		return "", fmt.Errorf("import key: %w", err)
	}

	return acct.Address.String(), nil
}

// createPQWallet generates and stores a post-quantum key. The caller holds m.mu.
func (m *Manager) createPQWallet(name string, scheme core.PQScheme) (string, error) {
	if err := pqc.SchemeAvailable(scheme); err != nil {
		return "", fmt.Errorf("create %s wallet: %w", scheme, err)
	}

	key, err := pqc.GenerateKey(scheme, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate %s key: %w", scheme, err)
	}
	defer destroyPQKey(key)

	acct, err := m.store.SavePQKey(name, key, m.passphrase, m.pqScryptN, m.pqScryptP)
	if err != nil {
		return "", fmt.Errorf("save %s key: %w", scheme, err)
	}
	return acct.Address, nil
}

// ListWallets returns all ECDSA and post-quantum wallets, sorted by name.
// Wallets that cannot be read are skipped; if the post-quantum folder cannot
// be read at all, the error is logged and only ECDSA wallets are returned.
// A name that exists as both an ECDSA and a post-quantum wallet is listed
// twice, with Duplicate set on both entries.
func (m *Manager) ListWallets() ([]WalletInfo, error) {
	names := m.store.LocalAccounts()
	wallets := make([]WalletInfo, 0, len(names))
	for _, name := range names {
		addr, err := m.store.AddressFromAccountName(name)
		if err != nil {
			continue // skip wallets that can't be read
		}
		wallets = append(wallets, WalletInfo{Name: name, Address: addr, KeyType: KeyTypeECDSA})
	}

	pqAccts, unreadable, err := m.store.ScanPQAccounts()
	if err != nil {
		log.Printf("wallet: list post-quantum wallets: %v", err)
		pqAccts = nil
	}
	for _, u := range unreadable {
		log.Printf("wallet: skipping post-quantum folder %q: %v", u.Name, u.Err)
	}
	for _, acct := range pqAccts {
		kt, ktErr := KeyTypeFromPQScheme(acct.Scheme)
		if ktErr != nil {
			continue // skip wallets with a scheme this build does not know
		}
		wallets = append(wallets, WalletInfo{Name: acct.Name, Address: acct.Address, KeyType: kt})
	}

	count := make(map[string]int, len(wallets))
	for _, w := range wallets {
		count[w.Name]++
	}
	for i := range wallets {
		wallets[i].Duplicate = count[wallets[i].Name] > 1
	}

	sort.SliceStable(wallets, func(i, j int) bool {
		if wallets[i].Name != wallets[j].Name {
			return wallets[i].Name < wallets[j].Name
		}
		return wallets[i].KeyType < wallets[j].KeyType
	})
	return wallets, nil
}

// LookupWallet finds a local wallet (ECDSA or post-quantum) by name, then by
// address. Unlike ResolveAddress it never accepts an address that is not a
// local wallet. An input that is not a local wallet returns an error
// wrapping ErrWalletNotFound. A name that exists as both an ECDSA and a
// post-quantum wallet, or an address whose wallet has such a name, returns
// an error wrapping ErrAmbiguousWalletName.
func (m *Manager) LookupWallet(nameOrAddress string) (WalletInfo, error) {
	// By name.
	info, found, err := m.lookupByName(nameOrAddress)
	if err != nil || found {
		return info, err
	}

	// By address.
	if nameOrAddress != "" {
		for _, name := range m.store.LocalAccounts() {
			addr, aErr := m.store.AddressFromAccountName(name)
			if aErr == nil && addr == nameOrAddress {
				if m.pqNameTaken(name) {
					return WalletInfo{}, errAmbiguousName(name)
				}
				return WalletInfo{Name: name, Address: addr, KeyType: KeyTypeECDSA}, nil
			}
		}
	}
	acct, err := m.store.PQAccountByAddress(nameOrAddress)
	if err == nil {
		if m.store.DoesNamedAccountExist(acct.Name) {
			return WalletInfo{}, errAmbiguousName(acct.Name)
		}
		return pqWalletInfo(acct)
	}
	if !errors.Is(err, store.ErrPQAccountNotFound) {
		return WalletInfo{}, fmt.Errorf("look up wallet %q: %w", nameOrAddress, err)
	}

	return WalletInfo{}, errWalletNotFound(nameOrAddress)
}

// lookupByName returns the wallet stored under name. found is false, with a
// nil error, when name is neither an ECDSA nor a post-quantum wallet. A name
// present in both account-keys and pq-keys fails closed with an error
// wrapping ErrAmbiguousWalletName, even if one of the two entries is
// unreadable, since the caller cannot be sure which key it would get.
func (m *Manager) lookupByName(name string) (WalletInfo, bool, error) {
	ecdsaAddr, ecdsaErr := m.store.AddressFromAccountName(name)
	ecdsaExists := ecdsaErr == nil || m.store.DoesNamedAccountExist(name)
	acct, pqFound, pqErr := m.pqAccountByName(name)

	if ecdsaExists && (pqFound || pqErr != nil) {
		return WalletInfo{}, false, errAmbiguousName(name)
	}
	if pqErr != nil {
		return WalletInfo{}, false, pqErr
	}
	if pqFound {
		info, err := pqWalletInfo(acct)
		return info, err == nil, err
	}
	if ecdsaErr == nil {
		return WalletInfo{Name: name, Address: ecdsaAddr, KeyType: KeyTypeECDSA}, true, nil
	}
	return WalletInfo{}, false, nil
}

// pqNameTaken reports whether pq-keys has an entry for name, readable or
// not.
func (m *Manager) pqNameTaken(name string) bool {
	_, found, err := m.pqAccountByName(name)
	return found || err != nil
}

// nameAmbiguous reports whether name exists in both account-keys and
// pq-keys.
func (m *Manager) nameAmbiguous(name string) bool {
	return m.store.DoesNamedAccountExist(name) && m.pqNameTaken(name)
}

// pqAccountByName returns the post-quantum account stored under name.
// found is false, with a nil error, when no such account exists or name is
// not a valid account name.
func (m *Manager) pqAccountByName(name string) (store.PQAccount, bool, error) {
	acct, err := m.store.PQAccountByName(name)
	switch {
	case err == nil:
		return acct, true, nil
	case errors.Is(err, store.ErrPQAccountNotFound), errors.Is(err, store.ErrInvalidAccountName):
		return store.PQAccount{}, false, nil
	default:
		return store.PQAccount{}, false, fmt.Errorf("wallet %q: %w", name, err)
	}
}

func pqWalletInfo(acct store.PQAccount) (WalletInfo, error) {
	kt, err := KeyTypeFromPQScheme(acct.Scheme)
	if err != nil {
		return WalletInfo{}, fmt.Errorf("wallet %q: %w", acct.Name, err)
	}
	return WalletInfo{Name: acct.Name, Address: acct.Address, KeyType: kt}, nil
}

// GetSigner returns a Signer for the named ECDSA wallet or address.
// The wallet is unlocked with the manager's passphrase. Post-quantum wallets
// are rejected: use SignTransaction, which never keeps a decrypted PQ key.
func (m *Manager) GetSigner(nameOrAddress string) (signer.Signer, error) {
	info, err := m.LookupWallet(nameOrAddress)
	if err == nil {
		if info.KeyType.IsPQ() {
			return nil, fmt.Errorf("post-quantum wallet %q: use SignTransaction", info.Name)
		}
		return m.ecdsaSigner(info.Address)
	}
	if !errors.Is(err, ErrWalletNotFound) {
		return nil, err
	}

	// Not a local wallet: keep the previous behaviour (name lookup, then any
	// T-address). UnlockedKeystore only opens ECDSA keystores.
	addr, err := m.ResolveAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}
	return m.ecdsaSigner(addr)
}

// ecdsaSigner unlocks the ECDSA keystore holding addr. The store wraps the
// keystore's unlock error, so only a wrong passphrase puts keystore.ErrDecrypt
// in the error chain, as for post-quantum wallets; any other unlock failure
// keeps its own cause.
func (m *Manager) ecdsaSigner(addr string) (signer.Signer, error) {
	ks, acct, err := m.store.UnlockedKeystore(addr, m.passphrase)
	if err != nil {
		return nil, fmt.Errorf("unlock wallet: %w", err)
	}

	m.mu.Lock()
	m.unlockedKS = append(m.unlockedKS, ks)
	m.mu.Unlock()

	return signer.NewKeystoreSigner(ks, *acct), nil
}

// SignTransaction signs tx with the local wallet named or addressed by
// nameOrAddress and returns the signed transaction. An ECDSA wallet adds an
// entry to tx.Signature. A post-quantum wallet adds a pq_auth_sig entry; its
// private key is decrypted for this call only and destroyed before return.
// SDK errors are wrapped, so errors.Is works for pqc.ErrFalconUnavailable,
// signer.ErrPQAlreadySigned, keystore.ErrTooManySignatures and
// keystore.ErrDecrypt (a wrong passphrase, for both kinds of wallet). Lookup
// errors wrap ErrWalletNotFound or ErrAmbiguousWalletName; an ambiguous name
// never signs.
func (m *Manager) SignTransaction(nameOrAddress string, tx *core.Transaction) (*core.Transaction, error) {
	info, err := m.LookupWallet(nameOrAddress)
	if err != nil {
		return nil, err
	}

	if !info.KeyType.IsPQ() {
		s, sErr := m.ecdsaSigner(info.Address)
		if sErr != nil {
			return nil, sErr
		}
		signed, sErr := s.Sign(tx)
		if sErr != nil {
			return nil, fmt.Errorf("sign: %w", sErr)
		}
		return signed, nil
	}

	return m.signPQ(info, tx)
}

func (m *Manager) signPQ(info WalletInfo, tx *core.Transaction) (*core.Transaction, error) {
	acct, err := m.store.PQAccountByName(info.Name)
	if err != nil {
		return nil, fmt.Errorf("post-quantum wallet %q: %w", info.Name, err)
	}
	// Refuse a scheme this build cannot sign with before the key is ever
	// decrypted into memory.
	if err := pqc.SchemeAvailable(acct.Scheme); err != nil {
		return nil, fmt.Errorf("post-quantum wallet %q: %w", info.Name, err)
	}

	key, err := m.store.DecryptPQKey(acct, m.passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypt post-quantum wallet %q: %w", info.Name, err)
	}
	defer destroyPQKey(key)

	if key.Scheme() != acct.Scheme || key.Public().Address().String() != info.Address {
		return nil, fmt.Errorf("post-quantum wallet %q: decrypted key does not match address %s",
			info.Name, info.Address)
	}

	s, err := signer.NewPQSigner(key)
	if err != nil {
		return nil, fmt.Errorf("post-quantum wallet %q: %w", info.Name, err)
	}
	signed, err := s.Sign(tx)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}
	return signed, nil
}

// SetKeystoreFactory sets the keystore factory function (for testing).
func (m *Manager) SetKeystoreFactory(fn func(string) *keystore.KeyStore) {
	m.store.SetKeystoreFactory(fn)
}

// Close closes all tracked keystores.
func (m *Manager) Close() {
	m.mu.Lock()
	for _, ks := range m.unlockedKS {
		ks.Close()
	}
	m.unlockedKS = nil
	m.mu.Unlock()
	m.store.CloseAll()
}

// ResolveAddress converts a wallet name (ECDSA or post-quantum) to its
// address, or returns the input if it's already a base58 address (starts
// with T). A name that exists as both an ECDSA and a post-quantum wallet
// returns an error wrapping ErrAmbiguousWalletName; an unknown name returns
// one wrapping ErrWalletNotFound.
func (m *Manager) ResolveAddress(nameOrAddress string) (string, error) {
	// Try as wallet name first
	info, found, err := m.lookupByName(nameOrAddress)
	if err != nil {
		return "", err
	}
	if found {
		return info.Address, nil
	}
	// Fall back to address if it looks like a base58 TRON address
	if strings.HasPrefix(nameOrAddress, "T") && len(nameOrAddress) == 34 {
		return nameOrAddress, nil
	}
	return "", errWalletNotFound(nameOrAddress)
}

// ResolveWalletName returns the canonical wallet name for a name-or-address input.
// If the input is a wallet name, returns it. If it's an address, looks up
// the wallet name (ECDSA or post-quantum) via the store. Returns the input
// unchanged if no mapping is found, or if the wallet's name exists as both
// an ECDSA and a post-quantum wallet.
func (m *Manager) ResolveWalletName(nameOrAddress string) string {
	// If it looks like a name (not a T-address), return as-is
	if !strings.HasPrefix(nameOrAddress, "T") || len(nameOrAddress) != 34 {
		return nameOrAddress
	}
	// It's an address — try to find the wallet name
	for _, name := range m.store.LocalAccounts() {
		addr, aErr := m.store.AddressFromAccountName(name)
		if aErr == nil && addr == nameOrAddress {
			if m.nameAmbiguous(name) {
				return nameOrAddress
			}
			return name
		}
	}
	if acct, pqErr := m.store.PQAccountByAddress(nameOrAddress); pqErr == nil {
		if m.nameAmbiguous(acct.Name) {
			return nameOrAddress
		}
		return acct.Name
	}
	return nameOrAddress
}

var validWalletName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// validateWalletName checks that the name is safe for filesystem use.
func validateWalletName(name string) error {
	if name == "" {
		return fmt.Errorf("wallet name is required")
	}
	if !validWalletName.MatchString(name) {
		return fmt.Errorf("invalid wallet name %q: must contain only letters, digits, hyphens, and underscores", name)
	}
	return nil
}
