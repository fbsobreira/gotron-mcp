# TRON Accounts

## Address Format

- 34 characters long, starting with `T`
- Base58Check encoded (similar to Bitcoin)
- Internally: 21-byte hex with `0x41` prefix
- New accounts must receive at least 0.1 TRX to be activated on-chain

## SDK: Address Utilities

```go
import "github.com/fbsobreira/gotron-sdk/pkg/address"

// Parse and validate base58 address
addr, err := address.Base58ToAddress("TXyz...")
if err != nil || !addr.IsValid() { // IsValid validates length, prefix, AND Base58Check checksum (v0.25.3+)
    // invalid address
}

// Convert hex to address (returns error on invalid hex)
addr, err := address.HexToAddress("41abc...")

// Convert raw bytes to TRON address (prepends 0x41 for 20-byte input)
addr = address.BytesToAddress(rawBytes)

// Convert 20-byte Ethereum address to TRON address
addr, err = address.EthAddressToAddress(ethAddrBytes)

// Other conversions
addr, err = address.Base64ToAddress(base64String)
addr, err = address.BigToAddress(bigInt)  // returns error on oversize big.Int
addr = address.PubkeyToAddress(ecdsaPubKey)

// Format
addr.String()  // base58: "TXyz..."
addr.Hex()     // hex: "41abc..."
```

## SDK: Account Queries

```go
// Get account info (balance, resources, frozen)
account, err := conn.GetAccount("TXyz...")
balance := account.Balance  // in SUN (divide by 1,000,000 for TRX)

// Get resource usage
resources, err := conn.GetAccountResource("TXyz...")
// resources.EnergyUsed, resources.EnergyLimit
// resources.NetUsed, resources.NetLimit
// resources.FreeNetUsed, resources.FreeNetLimit
```

## SDK: Account Permissions (Multi-Signature)

TRON accounts support multi-signature permissions:

| Permission ID | Type | Description |
|---------------|------|-------------|
| 0 | Owner | Full control, can modify permissions |
| 1 | Witness | Block production (SRs only) |
| 2 | Active | Customizable, limited operations |

```go
// Update account permissions for multi-sig
tx, err := conn.UpdateAccountPermission(
    "TOwnerAddr...",
    ownerPermission,    // map[string]interface{}
    witnessPermission,  // map[string]interface{} (nil for non-SR)
    activePermissions,  // []map[string]interface{}
)

// When building transactions with multi-sig:
// Set PermissionId to specify which permission to use
tx.SetPermissionId(2)   // use active permission
tx.UpdateHash()         // recalculate hash after modification
```

## SDK: Keystore

```go
import "github.com/fbsobreira/gotron-sdk/pkg/keystore"

ks := keystore.NewKeyStore("/path/to/keystore", keystore.StandardScryptN, keystore.StandardScryptP)
defer ks.Close()  // Important: prevents goroutine leaks

// List accounts
accounts := ks.Accounts()

// Sign transaction with passphrase
signedTx, err := ks.SignTxWithPassphrase(account, "passphrase", unsignedTx)

// v0.25.3+: Keys are automatically zeroed from memory on Lock(), TimedUnlock(), Export(), and Update()
```

## SDK: Signer Interface (v0.25.2+)

The `pkg/signer` package provides a unified signing interface used by the fluent builder API:

```go
import "github.com/fbsobreira/gotron-sdk/pkg/signer"

type Signer interface {
    Sign(tx *core.Transaction) (*core.Transaction, error)
    Address() address.Address
}

// From ECDSA private key
s, err := signer.NewPrivateKeySigner(ecdsaKey)

// From btcec private key
s, err = signer.NewPrivateKeySignerFromBTCEC(btcecKey)

// From pre-unlocked keystore
s = signer.NewKeystoreSigner(ks, account)

// From keystore with passphrase (unlocks per-sign)
s = signer.NewKeystorePassphraseSigner(ks, account, "passphrase")

// From Ledger hardware wallet
s, err = signer.NewLedgerSigner()
```

Use with fluent builders:

```go
receipt, err := builder.Transfer(from, to, amount).Send(ctx, s)
```

## SDK: Named Wallet Store (v0.25.3+)

The `pkg/store` package provides named wallet management on top of the keystore:

```go
import "github.com/fbsobreira/gotron-sdk/pkg/store"

// Create a store rooted at a custom directory
s := store.NewStore("~/.gotron-mcp/wallets")
s.InitConfigDir()

// Or use the default ~/.tronctl directory
s = store.DefaultStoreInstance()

// List named wallets
names := s.LocalAccounts()  // ["savings", "agent-bot", ...]

// Get address for a named wallet
addr, err := s.AddressFromAccountName("savings")

// Find keystore by address
ks := s.FromAddress("TJD...")

// Unlock a wallet for signing
ks, acct, err := s.UnlockedKeystore("TJD...", "passphrase")
// Use with signer: signer.NewKeystoreSigner(ks, *acct)

// Always close keystores when done
defer ks.Close()
defer s.Forget(ks)
```

## SDK: Mnemonic Generation

```go
import "github.com/fbsobreira/gotron-sdk/pkg/mnemonic"

// Generate a new 24-word BIP39 mnemonic — returns error on entropy failure
phrase, err := mnemonic.Generate()
```

## SDK: HD Wallet / BIP44

```go
import (
    "github.com/btcsuite/btcd/btcec/v2"
    "github.com/fbsobreira/gotron-sdk/pkg/keys"
    "github.com/fbsobreira/gotron-sdk/pkg/keys/hd"
)

// Quick: derive from mnemonic (uses default BIP44 path m/44'/195'/0'/0/{index})
// Deprecated: use mnemonic.FromSeedAndPassphrase instead
privKey, pubKey := keys.FromMnemonicSeedAndPassphrase(mnemonic, passphrase, 0)

// Preferred (v0.25.3+):
// import "github.com/fbsobreira/gotron-sdk/pkg/mnemonic"
// privKey, pubKey := mnemonic.FromSeedAndPassphrase(seedPhrase, passphrase, 0)

// Manual: parse BIP44 path (both formats accepted — with or without m/ prefix)
params, err := hd.NewParamsFromPath("m/44'/195'/0'/0/0")

// Compute master key from seed
secret, chainCode := hd.ComputeMastersFromSeed(seed, []byte("Bitcoin seed"))

// Derive private key for path (must use secp256k1 curve for TRON)
// Signature: DerivePrivateKeyForPath(curve, privKeyBytes [32]byte, chainCode [32]byte, path string) ([32]byte, error)
derivedKey, err := hd.DerivePrivateKeyForPath(btcec.S256(), secret, chainCode, "m/44'/195'/0'/0/0")
```

**Note (v0.25.2+):** `hd.DerivePrivateKeyForPath` and `hd.NewParamsFromPath` now return errors for invalid paths — always check the returned error. `keys.FromMnemonicSeedAndPassphrase` and `hd.ComputeMastersFromSeed` do not return errors; they return nil/zero values on failure.

## MCP: Wallet Management

> **Note:** Wallet and signing tools require local mode with `--keystore-dir` and `GOTRON_MCP_KEYSTORE_PASSPHRASE` configured. They are not available in hosted (HTTP) mode or when keystore is not configured.

The MCP server manages wallets in an isolated directory (`~/.gotron-mcp/wallets/` by default). A pre-configured passphrase (`GOTRON_MCP_KEYSTORE_PASSPHRASE` env) handles keystore encryption.

### Configuration

- `--keystore-dir` / `GOTRON_MCP_KEYSTORE_DIR` — wallet directory (default: `~/.gotron-mcp/wallets/`)
- `--keystore-pass` / `GOTRON_MCP_KEYSTORE_PASSPHRASE` — passphrase for wallet encryption
- `--require-policy` / `GOTRON_MCP_REQUIRE_POLICY` — refuse to sign without policy config (safety net)

### Wallet Tools

- `create_wallet(name)` — create a new named wallet, returns address
- `list_wallets()` — list all wallets with names and addresses

### Sign Tools

- `sign_transaction(wallet, transaction_hex)` — sign and return signed hex (no broadcast)
- `sign_and_broadcast(wallet, transaction_hex)` — sign + broadcast, returns txid and receipt
- `sign_and_confirm(wallet, transaction_hex)` — sign + broadcast + poll for on-chain confirmation
- `broadcast_transaction(signed_transaction_hex)` — broadcast a pre-signed transaction

## MCP Tools

- `get_account` — Get account balance and details
- `get_account_resources` — Get energy/bandwidth usage and limits
- `validate_address` — Validate and convert address formats
- `create_wallet` — Create a new named wallet
- `list_wallets` — List all managed wallets
- `sign_transaction` — Sign a transaction with a managed wallet
- `sign_and_broadcast` — Sign and broadcast in one step
- `sign_and_confirm` — Sign, broadcast, and wait for confirmation
- `broadcast_transaction` — Broadcast a pre-signed transaction
