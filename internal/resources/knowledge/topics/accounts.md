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

// Convert hex to address (no validation)
addr := address.HexToAddress("41abc...")

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

## MCP Tools

- `get_account` — Get account balance and details
- `get_account_resources` — Get energy/bandwidth usage and limits
- `validate_address` — Validate and convert address formats
