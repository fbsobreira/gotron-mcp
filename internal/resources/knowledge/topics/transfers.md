# TRON Transfers

## TRX Transfer

- Native TRX transfer between accounts
- Consumes bandwidth (free daily bandwidth or staked)
- Amount specified in SUN (1 TRX = 1,000,000 SUN)

## SDK: TRX Transfer

```go
// amount in SUN (1 TRX = 1,000,000 SUN)
tx, err := conn.Transfer("TFromAddr...", "TToAddr...", 1_000_000)  // 1 TRX

// tx.Transaction contains the unsigned transaction
// tx.Txid contains the transaction ID
```

## SDK: Transaction Serialization

```go
import "github.com/fbsobreira/gotron-sdk/pkg/client/transaction"

// All write methods return *api.TransactionExtention

// Serialize to hex (for external signing)
txHex, err := transaction.ToRawDataHex(tx.Transaction)

// Serialize to JSON (TRON HTTP API compatible)
txJSON, err := transaction.ToJSON(tx.Transaction)

// Reconstruct from hex (received from external system)
restored, err := transaction.FromRawDataHex(hexString)

// Reconstruct from JSON (TRON HTTP API format)
restored, err := transaction.FromJSON(jsonBytes)
```

## SDK: Multi-Signature Transactions

```go
// Set permission ID for multi-sig transactions
// PermissionId 0 = owner, PermissionId 2 = active
tx.Transaction.RawData.Contract[0].PermissionId = 2

// Or use the TransactionExtention helper
tx.SetPermissionId(2)
tx.UpdateHash()  // Recalculate hash after modification

// Validate signature weight
signWeight, err := conn.GetTransactionSignWeight(signedTx)
// Check if enough signatures collected
```

## Transaction Flow

1. **Build** — Create unsigned transaction via SDK
2. **Sign** — Sign with private key (keystore, hardware wallet, etc.)
3. **Broadcast** — Submit signed transaction to network
4. **Confirm** — Included in block (~3 second block time)

## Transaction Lookup

```go
// Get transaction by ID
tx, err := conn.GetTransactionByID("abc123...")

// Get transaction receipt (fees, energy, result)
info, err := conn.GetTransactionInfoByID("abc123...")
// info.Fee, info.BlockNumber, info.Receipt.EnergyUsage
```

## SDK: Transaction Decoding

```go
import "github.com/fbsobreira/gotron-sdk/pkg/client/transaction"

// Decode a transaction's contract data into human-readable form
decoded, err := transaction.DecodeContractData(tx)
if err != nil {
    // Handle unsupported contract type or nil transaction
}
// decoded.Type = "TransferContract"
// decoded.Fields = map[string]any{
//     "owner_address": "TSender...",   // base58
//     "to_address":    "TReceiver...", // base58
//     "amount":        "5.000000",     // TRX (converted from SUN)
// }
```

Supported contract types:
- `TransferContract` — owner_address, to_address, amount (TRX)
- `TransferAssetContract` — owner_address, to_address, asset_name, amount
- `TriggerSmartContract` — owner_address, contract_address, data (hex), call_value (TRX)
- `FreezeBalanceV2Contract` — owner_address, frozen_balance (TRX), resource
- `UnfreezeBalanceV2Contract` — owner_address, unfreeze_balance (TRX), resource
- `VoteWitnessContract` — owner_address, votes (array of {vote_address, vote_count})
- `DelegateResourceContract` — owner_address, receiver_address, balance (TRX), resource, lock, lock_period
- `UnDelegateResourceContract` — owner_address, receiver_address, balance (TRX), resource

Sentinel errors: `ErrNilTransaction`, `ErrNoContracts`, `ErrNilParameter`, `ErrUnsupportedContract`, `ErrUnmarshalContract`

## SDK: Broadcasting

```go
result, err := conn.Broadcast(signedTx)
// result.Result (bool), result.Code, result.Message
```

## SDK: Complete Sign & Broadcast Flow

```go
import (
    "github.com/fbsobreira/gotron-sdk/pkg/client/transaction"
    "github.com/fbsobreira/gotron-sdk/pkg/keys"
)

// 1. Build transaction (e.g., TRX transfer)
tx, err := conn.Transfer(from, to, amount)

// 2. Get private key
privKey, err := keys.GetPrivateKeyFromHex("your_hex_private_key")
// Or from keystore:
// ks := keystore.NewKeyStore(path, ...)
// signedTx, err := ks.SignTxWithPassphrase(account, passphrase, tx.Transaction)

// 3. Sign the transaction
signedTx, err := transaction.SignTransaction(tx.Transaction, privKey)

// 4. Broadcast
result, err := conn.Broadcast(signedTx)
// result.Result (bool), result.Code, result.Message

// For multi-sig: set permission ID before signing
tx.SetPermissionId(2)   // active permission
tx.UpdateHash()
signedTx, err := transaction.SignTransaction(tx.Transaction, privKey)
```

## SDK: Fluent Transfer Builder (v0.25.2+)

```go
import (
    "github.com/fbsobreira/gotron-sdk/pkg/txbuilder"
    "github.com/fbsobreira/gotron-sdk/pkg/signer"
)

builder := txbuilder.New(conn)
s, err := signer.NewPrivateKeySigner(privKey)
// handle err

// Build unsigned transaction (for external signing)
tx, err := builder.Transfer(from, to, amountSUN).Build(ctx)

// Sign and broadcast in one step
receipt, err := builder.Transfer(from, to, amountSUN).Send(ctx, s)

// Sign, broadcast, and wait for confirmation
receipt, err := builder.Transfer(from, to, amountSUN).SendAndConfirm(ctx, s)
// receipt.TxID, receipt.BlockNumber, receipt.Confirmed, receipt.Fee

// With options
receipt, err := builder.Transfer(from, to, amountSUN,
    txbuilder.WithMemo("payment"),
    txbuilder.WithPermissionID(2), // multi-sig active permission
).Send(ctx, s)
```

## SDK: Receipt Type

All builder `Send` and `SendAndConfirm` operations return a `txresult.Receipt`:

```go
type Receipt struct {
    TxID          string   // transaction hash
    BlockNumber   int64    // block number
    Confirmed     bool     // true after confirmation polling
    EnergyUsed    int64    // energy consumed
    BandwidthUsed int64    // bandwidth consumed
    Fee           int64    // fee in SUN
    Result        []byte   // contract return data
    Error         string   // TRON error message if failed
}
```

## SDK: Pending Pool / Mempool

Query the pending transaction pool before confirmation:

```go
// Check if a transaction is still pending
pending, err := conn.IsTransactionPendingCtx(ctx, txID)

// Get a specific pending transaction
tx, err := conn.GetTransactionFromPendingCtx(ctx, txID)
// Returns client.ErrPendingTxNotFound if not in pool

// List all pending transaction IDs
list, err := conn.GetTransactionListFromPendingCtx(ctx)

// Get pending pool size
size, err := conn.GetPendingSizeCtx(ctx)

// Get pending transactions for a specific address
txs, err := conn.GetPendingTransactionsByAddressCtx(ctx, "TAddr...")
```

## TronGrid REST API

The MCP server also queries the TronGrid REST API for address-based history that is not available via gRPC.

### Transaction History

- `GET /v1/accounts/{address}/transactions` — Full transaction history for an address
- Supports pagination via `fingerprint` cursor, `limit` (max 200), timestamp filtering
- Filter by direction: `only_to`, `only_from`

### TRC20 Transfer History

- `GET /v1/accounts/{address}/transactions/trc20` — TRC20 token transfer history
- Includes token metadata (symbol, name, decimals, contract address)
- Same pagination and filtering as transaction history

### Contract Events

- `GET /v1/contracts/{address}/events` — Decoded events emitted by a smart contract
- Filter by `event_name` (e.g. `Transfer`, `Approval`)
- Returns decoded event parameters with types

### Authentication

- Uses the same `GOTRON_NODE_API_KEY` as gRPC connections
- Sent via `TRON-PRO-API-KEY` header
- Required for higher rate limits on TronGrid

## MCP Tools

- `transfer_trx` — Create unsigned TRX transfer
- `transfer_trc20` — Create unsigned TRC20 transfer
- `get_transaction` — Look up transaction details by ID (includes decoded contract_data)
- `get_transaction_history` — Get paginated transaction history for an address (TronGrid REST)
- `get_trc20_transfers` — Get paginated TRC20 token transfer history (TronGrid REST)
- `get_contract_events` — Get decoded smart contract events (TronGrid REST)
- `sign_transaction` — Sign using local keystore (opt-in)
- `broadcast_transaction` — Broadcast signed transaction
- `get_network` — Check current network connection
- `get_pending_transactions` — List pending transaction IDs and pool size
- `is_transaction_pending` — Check if a transaction is still in the mempool
- `get_pending_by_address` — Get pending transactions for a specific address
