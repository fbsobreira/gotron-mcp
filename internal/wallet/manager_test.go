package wallet

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	c "github.com/fbsobreira/gotron-sdk/pkg/common"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/fbsobreira/gotron-sdk/pkg/pqc"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/fbsobreira/gotron-sdk/pkg/signer"
	"github.com/fbsobreira/gotron-sdk/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewManager(dir, "test-passphrase")
	require.NoError(t, err, "NewManager")
	m.SetKeystoreFactory(keystore.ForPathLight)
	m.SetPQScryptParams(keystore.LightScryptN, keystore.LightScryptP)
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

func TestResolveWalletName_Name(t *testing.T) {
	m := newTestManager(t)
	result := m.ResolveWalletName("my-wallet")
	assert.Equal(t, "my-wallet", result)
}

func TestResolveWalletName_NonTAddress(t *testing.T) {
	m := newTestManager(t)
	result := m.ResolveWalletName("not-a-t-address")
	assert.Equal(t, "not-a-t-address", result)
}

func TestResolveWalletName_UnknownAddress(t *testing.T) {
	m := newTestManager(t)
	// A valid-length T-address that doesn't exist in the store
	addr := "TXyz1234567890abcdefghijklmnopqrst"
	result := m.ResolveWalletName(addr)
	assert.Equal(t, addr, result)
}

// newTestTx returns a minimal unsigned TransferContract transaction.
func newTestTx() *core.Transaction {
	return &core.Transaction{
		RawData: &core.TransactionRaw{
			Contract: []*core.Transaction_Contract{{
				Type: core.Transaction_Contract_TransferContract,
			}},
			Timestamp:  1,
			Expiration: 2,
		},
	}
}

func findWallet(wallets []WalletInfo, name string) (WalletInfo, bool) {
	for _, w := range wallets {
		if w.Name == name {
			return w, true
		}
	}
	return WalletInfo{}, false
}

func TestCreateWalletWithKeyType_MLDSA(t *testing.T) {
	m := newTestManager(t)

	ecdsaAddr, err := m.CreateWallet("classic")
	require.NoError(t, err, "CreateWallet classic")
	pqAddr, err := m.CreateWalletWithKeyType("pq", KeyTypeMLDSA44)
	require.NoError(t, err, "CreateWalletWithKeyType ml_dsa_44")
	require.True(t, strings.HasPrefix(pqAddr, "T"), "expected T-address, got %s", pqAddr)
	require.Len(t, pqAddr, 34)
	require.NotEqual(t, ecdsaAddr, pqAddr)

	wallets, err := m.ListWallets()
	require.NoError(t, err, "ListWallets")
	require.Len(t, wallets, 2)
	assert.Equal(t, "classic", wallets[0].Name, "wallets must be sorted by name")
	assert.Equal(t, "pq", wallets[1].Name, "wallets must be sorted by name")

	tests := []struct {
		name    string
		addr    string
		keyType KeyType
	}{
		{"classic", ecdsaAddr, KeyTypeECDSA},
		{"pq", pqAddr, KeyTypeMLDSA44},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, ok := findWallet(wallets, tt.name)
			require.True(t, ok, "wallet %q missing from ListWallets: %v", tt.name, wallets)
			assert.Equal(t, tt.addr, w.Address)
			assert.Equal(t, tt.keyType, w.KeyType)

			byName, err := m.LookupWallet(tt.name)
			require.NoError(t, err, "LookupWallet by name")
			assert.Equal(t, WalletInfo{Name: tt.name, Address: tt.addr, KeyType: tt.keyType}, byName)

			byAddr, err := m.LookupWallet(tt.addr)
			require.NoError(t, err, "LookupWallet by address")
			assert.Equal(t, byName, byAddr)

			resolved, err := m.ResolveAddress(tt.name)
			require.NoError(t, err, "ResolveAddress")
			assert.Equal(t, tt.addr, resolved)

			assert.Equal(t, tt.name, m.ResolveWalletName(tt.addr))
		})
	}
}

func TestLookupWallet_NotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.CreateWalletWithKeyType("pq", KeyTypeMLDSA44)
	require.NoError(t, err)

	tests := []struct {
		desc  string
		input string
	}{
		{"unknown name", "nope"},
		{"unknown T-address", "TXyz1234567890abcdefghijklmnopqrst"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, err := m.LookupWallet(tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not found")
		})
	}
}

func TestResolveAddress_NonWalletAddressFallback(t *testing.T) {
	m := newTestManager(t)
	_, err := m.CreateWalletWithKeyType("pq", KeyTypeMLDSA44)
	require.NoError(t, err)

	addr := "TXyz1234567890abcdefghijklmnopqrst"
	got, err := m.ResolveAddress(addr)
	require.NoError(t, err)
	assert.Equal(t, addr, got)

	_, err = m.ResolveAddress("nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestCreateWalletWithKeyType_NameUniqueAcrossTypes(t *testing.T) {
	tests := []struct {
		desc   string
		first  KeyType
		second KeyType
	}{
		{"ecdsa then ml_dsa_44", KeyTypeECDSA, KeyTypeMLDSA44},
		{"ml_dsa_44 then ecdsa", KeyTypeMLDSA44, KeyTypeECDSA},
		{"ml_dsa_44 then ml_dsa_44", KeyTypeMLDSA44, KeyTypeMLDSA44},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			m := newTestManager(t)
			addr, err := m.CreateWalletWithKeyType("dup", tt.first)
			require.NoError(t, err, "first create")

			_, err = m.CreateWalletWithKeyType("dup", tt.second)
			require.Error(t, err, "expected duplicate name error")
			assert.Contains(t, err.Error(), "already exists")

			wallets, err := m.ListWallets()
			require.NoError(t, err)
			require.Len(t, wallets, 1)
			assert.Equal(t, addr, wallets[0].Address)
			assert.Equal(t, tt.first, wallets[0].KeyType)
		})
	}
}

func TestCreateWalletWithKeyType_InvalidInput(t *testing.T) {
	m := newTestManager(t)
	tests := []struct {
		desc    string
		name    string
		keyType KeyType
	}{
		{"invalid name", "bad/name", KeyTypeMLDSA44},
		{"empty name", "", KeyTypeMLDSA44},
		{"unknown key type", "x", KeyType("rsa")},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, err := m.CreateWalletWithKeyType(tt.name, tt.keyType)
			require.Error(t, err)
		})
	}
	wallets, err := m.ListWallets()
	require.NoError(t, err)
	require.Empty(t, wallets)
}

func TestCreateWalletWithKeyType_FNDSAUnavailable(t *testing.T) {
	if pqc.SchemeAvailable(core.PQScheme_FN_DSA_512) == nil {
		t.Skip("FN-DSA-512 backend is compiled in (-tags falcon)")
	}
	m := newTestManager(t)
	_, err := m.CreateWallet("existing")
	require.NoError(t, err)
	before, err := m.ListWallets()
	require.NoError(t, err)

	_, err = m.CreateWalletWithKeyType("falcon", KeyTypeFNDSA512)
	require.Error(t, err)
	require.True(t, errors.Is(err, pqc.ErrFalconUnavailable), "expected ErrFalconUnavailable, got %v", err)

	after, err := m.ListWallets()
	require.NoError(t, err)
	assert.Equal(t, before, after, "no wallet may be created")
	assert.False(t, m.store.NameExists("falcon"), "no keystore may be written")
}

func TestSignTransaction_MLDSA(t *testing.T) {
	m := newTestManager(t)
	addr, err := m.CreateWalletWithKeyType("pq", KeyTypeMLDSA44)
	require.NoError(t, err)

	for _, input := range []string{"pq", addr} {
		t.Run(input, func(t *testing.T) {
			tx := newTestTx()
			signed, err := m.SignTransaction(input, tx)
			require.NoError(t, err, "SignTransaction")

			require.Empty(t, signed.GetSignature(), "ECDSA signatures must be untouched")
			require.Len(t, signed.GetPqAuthSig(), 1)
			sig := signed.GetPqAuthSig()[0]
			assert.Equal(t, core.PQScheme_ML_DSA_44, sig.GetScheme())

			rawBytes, err := proto.Marshal(signed.GetRawData())
			require.NoError(t, err)
			digest := sha256.Sum256(rawBytes)
			pk, err := pqc.ParsePublicKey(core.PQScheme_ML_DSA_44, sig.GetPublicKey())
			require.NoError(t, err)
			assert.Equal(t, addr, pk.Address().String(), "signing key must be the wallet's key")
			ok, err := pk.Verify(digest[:], sig.GetSignature())
			require.NoError(t, err)
			assert.True(t, ok, "signature must verify")

			_, err = m.SignTransaction(input, signed)
			require.Error(t, err, "second signature by the same key")
			assert.True(t, errors.Is(err, signer.ErrPQAlreadySigned), "expected ErrPQAlreadySigned, got %v", err)
		})
	}
}

func TestSignTransaction_ECDSA(t *testing.T) {
	m := newTestManager(t)
	aliceAddr, err := m.CreateWallet("alice")
	require.NoError(t, err)
	bobAddr, err := m.CreateWallet("bob")
	require.NoError(t, err)

	tests := []struct {
		input     string
		wantAddr  string
		otherAddr string
	}{
		{"alice", aliceAddr, bobAddr},
		{aliceAddr, aliceAddr, bobAddr},
		{"bob", bobAddr, aliceAddr},
		{bobAddr, bobAddr, aliceAddr},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			signed, err := m.SignTransaction(tt.input, newTestTx())
			require.NoError(t, err)
			require.Len(t, signed.GetSignature(), 1)
			assert.Empty(t, signed.GetPqAuthSig())

			rawBytes, err := proto.Marshal(signed.GetRawData())
			require.NoError(t, err)
			digest := sha256.Sum256(rawBytes)
			recovered, err := keystore.RecoverPubkey(digest[:], signed.GetSignature()[0])
			require.NoError(t, err, "recover signer")
			assert.Equal(t, tt.wantAddr, recovered.String(), "transaction must be signed by the requested wallet")
			assert.NotEqual(t, tt.otherAddr, recovered.String())
		})
	}
}

func TestSignTransaction_NotFound(t *testing.T) {
	m := newTestManager(t)
	for _, input := range []string{"nope", "TXyz1234567890abcdefghijklmnopqrst"} {
		t.Run(input, func(t *testing.T) {
			_, err := m.SignTransaction(input, newTestTx())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not found")
		})
	}
}

func TestGetSigner_PQWalletRejected(t *testing.T) {
	m := newTestManager(t)
	addr, err := m.CreateWalletWithKeyType("pq", KeyTypeMLDSA44)
	require.NoError(t, err)

	for _, input := range []string{"pq", addr} {
		t.Run(input, func(t *testing.T) {
			s, err := m.GetSigner(input)
			require.Error(t, err)
			assert.Nil(t, s)
			assert.Contains(t, err.Error(), "SignTransaction")
		})
	}
}

func TestSignTransaction_PQWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir, "right-pass")
	require.NoError(t, err)
	m.SetKeystoreFactory(keystore.ForPathLight)
	m.SetPQScryptParams(keystore.LightScryptN, keystore.LightScryptP)
	t.Cleanup(func() { m.Close() })
	_, err = m.CreateWalletWithKeyType("pq", KeyTypeMLDSA44)
	require.NoError(t, err)

	other, err := NewManager(dir, "wrong-pass")
	require.NoError(t, err)
	other.SetKeystoreFactory(keystore.ForPathLight)
	t.Cleanup(func() { other.Close() })

	tx := newTestTx()
	_, err = other.SignTransaction("pq", tx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, keystore.ErrDecrypt), "expected ErrDecrypt, got %v", err)
	assert.Empty(t, tx.GetPqAuthSig(), "tx must not be signed")
}

// newTestManagerAt is newTestManager for a given directory and passphrase.
func newTestManagerAt(t *testing.T, dir, passphrase string) *Manager {
	t.Helper()
	m, err := NewManager(dir, passphrase)
	require.NoError(t, err, "NewManager")
	m.SetKeystoreFactory(keystore.ForPathLight)
	m.SetPQScryptParams(keystore.LightScryptN, keystore.LightScryptP)
	t.Cleanup(func() { m.Close() })
	return m
}

func TestSignTransaction_ECDSAWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	m := newTestManagerAt(t, dir, "right-pass")
	addr, err := m.CreateWallet("classic")
	require.NoError(t, err)

	other := newTestManagerAt(t, dir, "wrong-pass")
	for _, input := range []string{"classic", addr} {
		t.Run(input, func(t *testing.T) {
			tx := newTestTx()
			_, err := other.SignTransaction(input, tx)
			require.Error(t, err)
			assert.True(t, errors.Is(err, keystore.ErrDecrypt), "expected ErrDecrypt, got %v", err)
			assert.True(t, errors.Is(err, store.ErrNoUnlockBadPassphrase), "original error must stay in the chain, got %v", err)
			assert.Empty(t, tx.GetSignature(), "tx must not be signed")
		})
	}
}

func TestSignTransaction_ECDSANotLocalNotDecryptError(t *testing.T) {
	t.Run("address not in a local keystore", func(t *testing.T) {
		m := newTestManager(t)
		_, err := m.ecdsaSigner("TXyz1234567890abcdefghijklmnopqrst")
		require.Error(t, err)
		assert.False(t, errors.Is(err, keystore.ErrDecrypt), "only a bad passphrase maps to ErrDecrypt, got %v", err)
	})

	// The store wraps every unlock failure as ErrNoUnlockBadPassphrase. A
	// corrupt keystore must keep its own cause, not read as a bad passphrase.
	t.Run("corrupt keystore cipher", func(t *testing.T) {
		dir := t.TempDir()
		m := newTestManagerAt(t, dir, "right-pass")
		_, err := m.CreateWallet("classic")
		require.NoError(t, err)
		m.Close()

		files, err := filepath.Glob(filepath.Join(dir, c.DefaultConfigAccountAliasesDirName, "classic", "*"))
		require.NoError(t, err)
		require.Len(t, files, 1, "fixture: expected one keystore file")
		raw, err := os.ReadFile(files[0])
		require.NoError(t, err)
		var doc map[string]any
		require.NoError(t, json.Unmarshal(raw, &doc))
		cryptoField, ok := doc["crypto"].(map[string]any)
		require.True(t, ok, "fixture: keystore JSON has no crypto object: %s", raw)
		require.Equal(t, "aes-128-ctr", cryptoField["cipher"], "fixture: unexpected cipher")
		cryptoField["cipher"] = "bogus-cipher"
		raw, err = json.Marshal(doc)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(files[0], raw, 0o600))

		fresh := newTestManagerAt(t, dir, "right-pass")
		tx := newTestTx()
		_, err = fresh.SignTransaction("classic", tx)
		require.Error(t, err)
		assert.False(t, errors.Is(err, keystore.ErrDecrypt), "a corrupt keystore is not a bad passphrase, got %v", err)
		assert.Contains(t, err.Error(), "bogus-cipher", "the cause must reach the caller")
		assert.Empty(t, tx.GetSignature(), "tx must not be signed")
	})
}

// TestWalletNameCollision covers a name present in both account-keys and
// pq-keys (an older tronctl keystore dir, or two racing processes). Every
// name or address that lands on that name must fail closed.
func TestWalletNameCollision(t *testing.T) {
	dirA := t.TempDir()
	mA := newTestManagerAt(t, dirA, "test-passphrase")
	pqAddr, err := mA.CreateWalletWithKeyType("dup", KeyTypeMLDSA44)
	require.NoError(t, err)
	otherAddr, err := mA.CreateWallet("other")
	require.NoError(t, err)

	dirB := t.TempDir()
	mB := newTestManagerAt(t, dirB, "test-passphrase")
	ecdsaAddr, err := mB.CreateWallet("dup")
	require.NoError(t, err)
	mB.Close()
	require.NoError(t, os.Rename(
		filepath.Join(dirB, c.DefaultConfigAccountAliasesDirName, "dup"),
		filepath.Join(dirA, c.DefaultConfigAccountAliasesDirName, "dup"),
	))
	require.True(t, mA.store.DoesNamedAccountExist("dup"), "fixture: ECDSA dup must exist in A")
	_, err = mA.store.PQAccountByName("dup")
	require.NoError(t, err, "fixture: PQ dup must exist in A")

	const wantMsg = `wallet name "dup" exists as both an ECDSA and a post-quantum wallet; rename one`
	assertAmbiguous := func(t *testing.T, err error) {
		t.Helper()
		require.Error(t, err)
		assert.Contains(t, err.Error(), wantMsg)
		assert.True(t, errors.Is(err, ErrAmbiguousWalletName), "expected ErrAmbiguousWalletName, got %v", err)
		assert.False(t, errors.Is(err, ErrWalletNotFound), "ambiguity is not not-found")
	}

	for _, input := range []string{"dup", pqAddr, ecdsaAddr} {
		t.Run("lookup/"+input, func(t *testing.T) {
			_, err := mA.LookupWallet(input)
			assertAmbiguous(t, err)
		})
		t.Run("sign/"+input, func(t *testing.T) {
			tx := newTestTx()
			signed, err := mA.SignTransaction(input, tx)
			assertAmbiguous(t, err)
			assert.Nil(t, signed)
			assert.Empty(t, tx.GetSignature(), "no ECDSA signature may be added")
			assert.Empty(t, tx.GetPqAuthSig(), "no PQ signature may be added")
		})
		t.Run("signer/"+input, func(t *testing.T) {
			s, err := mA.GetSigner(input)
			assertAmbiguous(t, err)
			assert.Nil(t, s)
		})
		t.Run("resolve-name/"+input, func(t *testing.T) {
			assert.Equal(t, input, mA.ResolveWalletName(input), "must not map onto an ambiguous name")
		})
	}

	_, err = mA.ResolveAddress("dup")
	assertAmbiguous(t, err)

	got, err := mA.ResolveAddress("other")
	require.NoError(t, err)
	assert.Equal(t, otherAddr, got)

	wallets, err := mA.ListWallets()
	require.NoError(t, err)
	assert.ElementsMatch(t, []WalletInfo{
		{Name: "dup", Address: ecdsaAddr, KeyType: KeyTypeECDSA, Duplicate: true},
		{Name: "dup", Address: pqAddr, KeyType: KeyTypeMLDSA44, Duplicate: true},
		{Name: "other", Address: otherAddr, KeyType: KeyTypeECDSA},
	}, wallets)
}

func TestLookupWallet_NotFoundSentinel(t *testing.T) {
	m := newTestManager(t)
	for _, input := range []string{"nope", "TXyz1234567890abcdefghijklmnopqrst", ""} {
		t.Run(input, func(t *testing.T) {
			_, err := m.LookupWallet(input)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrWalletNotFound), "expected ErrWalletNotFound, got %v", err)
			assert.Contains(t, err.Error(), "not found")
		})
	}
	_, err := m.ResolveAddress("nope")
	assert.True(t, errors.Is(err, ErrWalletNotFound), "expected ErrWalletNotFound, got %v", err)
}

func TestGetSigner_LookupErrorNotHidden(t *testing.T) {
	m := newTestManager(t)
	// A pq-keys/<name> folder with two keystore files is unreadable, which
	// is not the same as not found.
	broken := filepath.Join(m.store.PQKeysDir(), "broken")
	require.NoError(t, os.MkdirAll(broken, 0o700))
	for _, f := range []string{"a.json", "b.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(broken, f), []byte("{}"), 0o600))
	}

	_, err := m.LookupWallet("broken")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrWalletNotFound), "fixture: lookup error must not be not-found, got %v", err)

	s, err := m.GetSigner("broken")
	require.Error(t, err)
	assert.Nil(t, s)
	assert.True(t, errors.Is(err, store.ErrInvalidPQKeystore), "GetSigner must return the lookup error, got %v", err)
}

func TestListWallets_PQDirUnreadable(t *testing.T) {
	m := newTestManager(t)
	addr, err := m.CreateWallet("classic")
	require.NoError(t, err)
	// A plain file where the pq-keys folder should be makes PQAccounts fail.
	require.NoError(t, os.WriteFile(m.store.PQKeysDir(), []byte("not a folder"), 0o600))
	_, pqErr := m.store.PQAccounts()
	require.Error(t, pqErr, "fixture: PQAccounts must fail")

	wallets, err := m.ListWallets()
	require.NoError(t, err)
	assert.Equal(t, []WalletInfo{{Name: "classic", Address: addr, KeyType: KeyTypeECDSA}}, wallets)
}

// collectPQKeys sets pqKeyDone for the test and returns the keys it sees.
// Tests that call it must not run in parallel.
func collectPQKeys(t *testing.T) func() []pqc.PrivateKey {
	t.Helper()
	var mu sync.Mutex
	var keys []pqc.PrivateKey
	pqKeyDone = func(k pqc.PrivateKey) {
		mu.Lock()
		defer mu.Unlock()
		keys = append(keys, k)
	}
	t.Cleanup(func() { pqKeyDone = nil })
	return func() []pqc.PrivateKey {
		mu.Lock()
		defer mu.Unlock()
		return append([]pqc.PrivateKey(nil), keys...)
	}
}

func assertAllDestroyed(t *testing.T, keys []pqc.PrivateKey, want int) {
	t.Helper()
	require.Len(t, keys, want, "pqKeyDone calls")
	for i, k := range keys {
		require.NotNil(t, k, "key %d", i)
		assert.True(t, k.Destroyed(), "key %d must be destroyed", i)
	}
}

func TestPQKeyDestroyed_Create(t *testing.T) {
	m := newTestManager(t)
	got := collectPQKeys(t)
	_, err := m.CreateWalletWithKeyType("pq", KeyTypeMLDSA44)
	require.NoError(t, err)
	assertAllDestroyed(t, got(), 1)
}

func TestPQKeyDestroyed_SignAndAlreadySigned(t *testing.T) {
	m := newTestManager(t)
	_, err := m.CreateWalletWithKeyType("pq", KeyTypeMLDSA44)
	require.NoError(t, err)
	got := collectPQKeys(t)

	signed, err := m.SignTransaction("pq", newTestTx())
	require.NoError(t, err)
	assertAllDestroyed(t, got(), 1)

	_, err = m.SignTransaction("pq", signed)
	require.True(t, errors.Is(err, signer.ErrPQAlreadySigned), "expected ErrPQAlreadySigned, got %v", err)
	assertAllDestroyed(t, got(), 2)
}

func TestPQKeyDestroyed_AddressMismatch(t *testing.T) {
	m := newTestManager(t)
	_, err := m.CreateWalletWithKeyType("pq", KeyTypeMLDSA44)
	require.NoError(t, err)
	otherAddr, err := m.CreateWallet("classic")
	require.NoError(t, err)
	got := collectPQKeys(t)

	tx := newTestTx()
	_, err = m.signPQ(WalletInfo{Name: "pq", Address: otherAddr, KeyType: KeyTypeMLDSA44}, tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
	assert.Empty(t, tx.GetPqAuthSig(), "tx must not be signed")
	assertAllDestroyed(t, got(), 1)
}

// TestSignTransaction_FNDSAUnavailableNoDecrypt checks that a build without
// a Falcon backend refuses an FN-DSA-512 wallet before decrypting it. The SDK
// ships no FN-DSA-512 keystore fixture, so the wallet is an ML-DSA-44
// keystore relabelled FN_DSA_512 in its clear fields: decrypting it would
// fail with a different error, or reach pqKeyDone.
func TestSignTransaction_FNDSAUnavailableNoDecrypt(t *testing.T) {
	if pqc.SchemeAvailable(core.PQScheme_FN_DSA_512) == nil {
		t.Skip("FN-DSA-512 backend is compiled in (-tags falcon)")
	}
	m := newTestManager(t)
	addr, err := m.CreateWalletWithKeyType("falcon", KeyTypeMLDSA44)
	require.NoError(t, err)
	acct, err := m.store.PQAccountByName("falcon")
	require.NoError(t, err)
	data, err := os.ReadFile(acct.Path)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(data, &fields))
	fields["scheme"] = core.PQScheme_FN_DSA_512.String()
	data, err = json.Marshal(fields)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(acct.Path, data, 0o600))

	info, err := m.LookupWallet("falcon")
	require.NoError(t, err)
	require.Equal(t, KeyTypeFNDSA512, info.KeyType, "fixture: wallet must read as FN-DSA-512")

	got := collectPQKeys(t)
	for _, input := range []string{"falcon", addr} {
		t.Run(input, func(t *testing.T) {
			tx := newTestTx()
			_, err := m.SignTransaction(input, tx)
			require.Error(t, err)
			assert.True(t, errors.Is(err, pqc.ErrFalconUnavailable), "expected ErrFalconUnavailable, got %v", err)
			assert.Empty(t, tx.GetPqAuthSig(), "tx must not be signed")
		})
	}
	assert.Empty(t, got(), "no key may be decrypted")
}
