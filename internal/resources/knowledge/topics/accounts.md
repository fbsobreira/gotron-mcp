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
if err != nil || !addr.IsValid() {
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
privKey, pubKey := keys.FromMnemonicSeedAndPassphrase(mnemonic, passphrase, 0)

// Manual: parse BIP44 path (both formats accepted — with or without m/ prefix)
params, err := hd.NewParamsFromPath("m/44'/195'/0'/0/0")

// Compute master key from seed
secret, chainCode := hd.ComputeMastersFromSeed(seed, []byte("Bitcoin seed"))

// Derive private key for path (must use secp256k1 curve for TRON)
// Signature: DerivePrivateKeyForPath(curve, privKeyBytes [32]byte, chainCode [32]byte, path string) ([32]byte, error)
derivedKey, err := hd.DerivePrivateKeyForPath(btcec.S256(), secret, chainCode, "m/44'/195'/0'/0/0")
```

**Note (v0.25.2+):** `hd.DerivePrivateKeyForPath` and `hd.NewParamsFromPath` now return errors for invalid paths — always check the returned error. `keys.FromMnemonicSeedAndPassphrase` and `hd.ComputeMastersFromSeed` do not return errors; they return nil/zero values on failure.

## MCP Tools

- `get_account` — Get account balance and details
- `get_account_resources` — Get energy/bandwidth usage and limits
- `validate_address` — Validate and convert address formats
