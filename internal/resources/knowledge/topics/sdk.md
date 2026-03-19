# GoTRON Fluent Builder API

The GoTRON SDK (v0.25.2+) provides a fluent builder API for constructing, signing, and broadcasting TRON transactions. These packages sit on top of the low-level `pkg/client` gRPC methods and offer a more ergonomic, type-safe interface.

## SDK Entry Point (`pkg/tron`)

```go
import "github.com/fbsobreira/gotron-sdk/pkg/tron"

sdk := tron.New(conn) // conn is *client.GrpcClient

// Access builders
builder := sdk.TxBuilder()                    // native transaction builder
call := sdk.Contract("TContractAddr...")      // smart contract call builder
token := sdk.TRC20("TContractAddr...")        // typed TRC20 wrapper
client := sdk.Client()                        // underlying gRPC client
```

## Signer Interface (`pkg/signer`)

All builder terminal operations that sign transactions accept a `signer.Signer`:

```go
import "github.com/fbsobreira/gotron-sdk/pkg/signer"

type Signer interface {
    Sign(tx *core.Transaction) (*core.Transaction, error)
    Address() address.Address
}
```

### Implementations

```go
// From raw ECDSA private key (validates secp256k1 curve)
s, err := signer.NewPrivateKeySigner(ecdsaKey)

// From btcec private key
s, err := signer.NewPrivateKeySignerFromBTCEC(btcecKey)

// From pre-unlocked keystore account
s := signer.NewKeystoreSigner(ks, account)

// From keystore with passphrase (unlocks per-sign)
s := signer.NewKeystorePassphraseSigner(ks, account, "passphrase")

// From Ledger hardware wallet
s, err := signer.NewLedgerSigner()
```

## Transaction Builder (`pkg/txbuilder`)

Fluent builder for native TRON transactions (transfers, staking, voting, delegation).

```go
import "github.com/fbsobreira/gotron-sdk/pkg/txbuilder"

builder := txbuilder.New(conn)
// Or with shared defaults:
builder = txbuilder.New(conn, txbuilder.WithMemo("hello"))
```

### Transfer

```go
receipt, err := builder.Transfer(from, to, amountSUN).Send(ctx, signer)
```

### Staking

```go
import "github.com/fbsobreira/gotron-sdk/pkg/proto/core"

// Stake TRX for energy
receipt, err := builder.FreezeV2(from, amountSUN, core.ResourceCode_ENERGY).
    Send(ctx, signer)

// Unstake
receipt, err = builder.UnfreezeV2(from, amountSUN, core.ResourceCode_ENERGY).
    Send(ctx, signer)
```

### Resource Delegation

```go
// Delegate with optional lock period
receipt, err := builder.DelegateResource(from, to, core.ResourceCode_ENERGY, amountSUN).
    Lock(86400). // lock for 86400 blocks (~3 days)
    Send(ctx, signer)

// Undelegate
receipt, err = builder.UnDelegateResource(from, to, core.ResourceCode_ENERGY, amountSUN).
    Send(ctx, signer)
```

### Voting

```go
// Fluent vote chaining
receipt, err := builder.VoteWitness(from).
    Vote("TSR1addr...", 1000).
    Vote("TSR2addr...", 500).
    Send(ctx, signer)

// Or from a map
votes := map[string]int64{"TSR1addr...": 1000, "TSR2addr...": 500}
receipt, err = builder.VoteWitness(from).
    Votes(votes).
    Send(ctx, signer)
```

### Terminal Operations

Every builder method returns a `*Tx` (or `*DelegateTx`, `*VoteTx`) with these terminals:

```go
// Build unsigned transaction (for external signing)
tx, err := builder.Transfer(from, to, amount).Build(ctx)

// Decode transaction for human-readable display
decoded, err := builder.Transfer(from, to, amount).Decode(ctx)
// decoded.Type = "TransferContract", decoded.Fields = {...}

// Sign and broadcast
receipt, err := builder.Transfer(from, to, amount).Send(ctx, signer)

// Sign, broadcast, and poll until confirmed
receipt, err = builder.Transfer(from, to, amount).SendAndConfirm(ctx, signer)
```

### Options

```go
// Add memo to any transaction
builder.Transfer(from, to, amount, txbuilder.WithMemo("payment for services"))

// Set permission ID for multi-sig
builder.Transfer(from, to, amount, txbuilder.WithPermissionID(2))
```

## Contract Call Builder (`pkg/contract`)

Fluent builder for smart contract interactions.

```go
import "github.com/fbsobreira/gotron-sdk/pkg/contract"

call := contract.New(conn, "TContractAddr...").
    From("TCallerAddr...").
    Method("transfer(address,uint256)").
    Params(`["TToAddr...", "1000000"]`).
    Apply(contract.WithFeeLimit(100_000_000))
```

### Fluent Setters

```go
call.Method("transfer(address,uint256)")   // method signature
call.From("TCallerAddr...")                // caller address
call.Params(`["TAddr...", "1000"]`)        // JSON params (plain or typed)
call.WithData(packedBytes)                 // pre-packed ABI data (alternative to Method+Params)
call.WithABI(abiJSON)                      // parsed ABI for future use
call.Apply(opts...)                        // apply options
```

### Terminal Operations

```go
// Read-only call (no transaction, no fees)
result, err := call.Call(ctx)
// result.RawResults [][]byte — raw return data
// result.EnergyUsed int64

// Estimate energy cost
energy, err := call.EstimateEnergy(ctx)

// Build unsigned transaction
tx, err := call.Build(ctx)

// Decode for human-readable display
decoded, err := call.Decode(ctx)

// Sign and broadcast
receipt, err := call.Send(ctx, signer)

// Sign, broadcast, and poll until confirmed
receipt, err = call.SendAndConfirm(ctx, signer)
```

### Options

```go
contract.WithFeeLimit(100_000_000)         // max TRX to burn (in SUN)
contract.WithCallValue(1_000_000)          // TRX to send with call (in SUN)
contract.WithTokenValue("1000001", 500)    // TRC10 token ID and amount
contract.WithPermissionID(2)               // multi-sig permission
```

### Deferred Error Pattern

Errors can be stored during chaining via `SetError()` and surfaced at any terminal call. This is used internally by wrappers like `trc20.Token.Transfer()`:

```go
// SetError stores a deferred error — surfaces at any terminal call
call := contract.New(conn, "TAddr...").
    SetError(fmt.Errorf("custom validation failed")).
    Method("transfer(address,uint256)")

result, err := call.Call(ctx)     // returns the deferred error
// err: "custom validation failed"

// Check for deferred errors explicitly
if call.Err() != nil { ... }
```

## TRC20 Typed Wrapper (`pkg/standards/trc20`)

Type-safe TRC20 token interactions with automatic decimal handling.

```go
import "github.com/fbsobreira/gotron-sdk/pkg/standards/trc20"

token := trc20.New(conn, "TContractAddr...")
```

### Query Methods

```go
// Get all metadata in one call
info, err := token.Info(ctx)
// info.Name, info.Symbol, info.Decimals (uint8), info.TotalSupply (*big.Int)

// Individual queries
name, err := token.Name(ctx)
symbol, err := token.Symbol(ctx)
decimals, err := token.Decimals(ctx)    // returns uint8
supply, err := token.TotalSupply(ctx)   // returns *big.Int

// Balance with formatted display
bal, err := token.BalanceOf(ctx, "TOwnerAddr...")
// bal.Raw     *big.Int — raw token units
// bal.Display string   — formatted (e.g., "1,000.50")
// bal.Symbol  string   — token symbol

// Allowance
allowance, err := token.Allowance(ctx, "TOwner...", "TSpender...")
```

### Write Methods

Write methods return `*contract.ContractCall` for fluent terminal operations:

```go
// Transfer tokens
receipt, err := token.Transfer(from, to, amount).Send(ctx, signer)

// Approve spender
receipt, err = token.Approve(from, spender, amount).Send(ctx, signer)

// Transfer on behalf (requires prior approval)
receipt, err = token.TransferFrom(caller, from, to, amount).Send(ctx, signer)

// With options
receipt, err = token.Transfer(from, to, amount,
    contract.WithFeeLimit(150_000_000),
    contract.WithPermissionID(2),
).Send(ctx, signer)

// Estimate energy before sending
energy, err := token.Transfer(from, to, amount).EstimateEnergy(ctx)
```

## Receipt Type (`pkg/txresult`)

All `Send` and `SendAndConfirm` terminal operations return a `Receipt`:

```go
type Receipt struct {
    TxID          string   // transaction hash
    BlockNumber   int64    // block that includes the tx
    Confirmed     bool     // true after confirmation polling
    EnergyUsed    int64    // energy consumed
    BandwidthUsed int64    // bandwidth consumed
    Fee           int64    // fee in SUN
    Result        []byte   // contract return data
    Error         string   // TRON error message if failed
}
```

`Send` returns a receipt with `TxID` immediately after broadcast. `SendAndConfirm` polls until the transaction is confirmed (or context cancelled) and fills in all fields.

## Choosing Between Approaches

| Use Case | Low-Level (`pkg/client`) | Builder (`pkg/txbuilder` / `pkg/contract`) |
|----------|-------------------------|-------------------------------------------|
| Simple read-only query | `conn.TriggerConstantContractCtx(...)` | `contract.New(...).Call(ctx)` |
| Build + external signing | `conn.TransferCtx(...)` | `builder.Transfer(...).Build(ctx)` |
| Sign + broadcast | Manual: build → sign → broadcast | `builder.Transfer(...).Send(ctx, signer)` |
| Wait for confirmation | Manual polling loop | `builder.Transfer(...).SendAndConfirm(ctx, signer)` |
| TRC20 with decimal handling | Manual: fetch decimals → scale amount | `trc20.New(...).BalanceOf(ctx, addr)` |

Both approaches are fully supported — builders wrap the low-level methods, not replace them.

## MCP Tools

The MCP server currently uses low-level `pkg/client` methods for its tools:

- `trigger_constant_contract` — read-only contract calls
- `trigger_contract` — write contract calls (returns unsigned tx)
- `estimate_energy` — energy estimation
- `transfer_trx` — TRX transfer (returns unsigned tx)
- `transfer_trc20` — TRC20 transfer (returns unsigned tx)
