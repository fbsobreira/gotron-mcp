# TRON Smart Contracts

## Overview

- Solidity-compatible (similar to Ethereum)
- Requires energy for execution
- Read-only calls are free (no transaction needed)
- Fee limit: maximum TRX willing to burn for execution

## SDK: Read-Only Calls

```go
// No transaction created, no fees — plain-value params (types inferred from signature)
result, err := conn.TriggerConstantContractCtx(ctx,
    "TCallerAddr...",
    "TContractAddr...",
    "balanceOf(address)",
    `["TOwnerAddr..."]`,
)
// result.ConstantResult contains the return data

// Simulate payable functions with WithCallValue (msg.value in SUN)
result, err = conn.TriggerConstantContractCtx(ctx,
    "TCallerAddr...", "TContractAddr...",
    "deposit()", `[]`,
    client.WithCallValue(1_000_000), // 1 TRX
)

// With TRC10 token value
opt, err := client.WithTokenValue("1000001", 500)
result, err = conn.TriggerConstantContractCtx(ctx,
    "TCallerAddr...", "TContractAddr...",
    "onTokenReceived()", `[]`,
    opt,
)
```

## SDK: Write Calls

```go
// Plain-value params (types inferred from method signature)
tx, err := conn.TriggerContractCtx(ctx,
    "TCallerAddr...",                         // from
    "TContractAddr...",                       // contract
    "transfer(address,uint256)",              // method signature
    `["TToAddr...", "1000000"]`,              // params (plain-value format)
    100_000_000,                              // fee limit (100 TRX in SUN)
    0,                                        // call value (TRX to send, in SUN)
    "",                                       // token ID (empty for TRX)
    0,                                        // token amount
)
// Returns unsigned transaction
```

## SDK: Pre-Packed ABI Data Calls

For callers that already have ABI-packed data (e.g., from go-ethereum's `abi.Pack()`):

```go
// Read-only call with pre-packed data
result, err := conn.TriggerConstantContractWithDataCtx(ctx,
    "TCallerAddr...", "TContractAddr...", packedData,
)

// Write call with pre-packed data
tx, err := conn.TriggerContractWithDataCtx(ctx,
    "TCallerAddr...", "TContractAddr...", packedData,
    100_000_000, // fee limit
    0, "", 0,    // callValue, tokenID, tokenAmount
)
```

## SDK: Get Contract ABI

```go
// Basic ABI lookup
abi, err := conn.GetContractABI("TContractAddr...")

// Proxy-aware ABI lookup (resolves ERC-1967 proxies automatically)
abi, err := conn.GetContractABIResolved("TContractAddr...")
// Checks for proxy pattern, resolves implementation, returns actual ABI
```

## SDK: Event Parsing

```go
import "github.com/fbsobreira/gotron-sdk/pkg/abi"

// Get indexed and non-indexed argument parsers for an event
indexed, _, err := abi.GetEventParser(contractABI, "Transfer")

// Parse event topics into a map
out := make(map[string]interface{})
err = abi.ParseTopicsIntoMap(out, indexed, topics)
// out now contains decoded indexed event fields
```

## SDK: Overloaded Methods

The SDK supports overloaded contract methods. Use the full signature to disambiguate:

```go
// Use full signature to select the correct overload
// "transfer(address,uint256)" vs "transfer(address,uint256,bytes)"
// GetParser and GetInputsParser match by full signature when provided
```

## Parameter Encoding

Two parameter formats are supported. The SDK auto-detects which format is used.

### Plain-Value Format (recommended)

Pass values directly — types are inferred from the method signature:

```json
["TXyz...", "1000000"]
```

Examples:
- `balanceOf(address)` → `["TJDENsfBJs4RFETt1X1W8wMDc8M5XnS5f4"]`
- `transfer(address,uint256)` → `["TJDENsfBJs4RFETt1X1W8wMDc8M5XnS5f4", "1000000"]`
- `approve(address,uint256)` → `["TJDENsfBJs4RFETt1X1W8wMDc8M5XnS5f4", "100"]`

### Typed-Object Format (also supported)

Explicitly specify the type for each parameter:

```json
[
    {"address": "TXyz..."},
    {"uint256": "1000000"},
    {"bool": "true"},
    {"string": "hello"}
]
```

Supported array types:

```json
[
    {"uint256[]": ["100", "200", "300"]},
    {"address[]": ["TAddr1...", "TAddr2..."]},
    {"bytes[]": ["ab", "cd"]}
]
```

### SDK: Parameter Parsing

```go
import "github.com/fbsobreira/gotron-sdk/pkg/abi"

// Auto-detect format and infer types from method signature
params, err := abi.LoadFromJSONWithMethod("transfer(address,uint256)", `["TJD...", "1000"]`)

// Or use typed-object format directly
params, err := abi.LoadFromJSON(`[{"address": "TJD..."}, {"uint256": "1000"}]`)
```

## Fee Limits

- TRC20 transfers: typically 100 TRX
- Complex contract calls: estimate energy first
- Simple reads: no fee (constant call)

## SDK: Decode Output

```go
import "github.com/fbsobreira/gotron-sdk/pkg/abi"

// Decode return values from TriggerConstantContract
decoded, err := abi.DecodeOutput(contractABI, "balanceOf(address)", resultBytes)
// Returns []interface{} with typed values (addresses auto-converted to TRON base58)

// Decode revert reasons
reason, err := abi.DecodeRevertReason(resultBytes)
// Supports Error(string) selector 0x08c379a0
// Supports Panic(uint256) selector 0x4e487b71
```

## SDK: Fluent Contract Builder (v0.25.2+)

```go
import "github.com/fbsobreira/gotron-sdk/pkg/contract"

call := contract.New(conn, "TContractAddr...").
    From("TCallerAddr...").
    Method("transfer(address,uint256)").
    Params(`["TToAddr...", "1000000"]`).
    Apply(contract.WithFeeLimit(100_000_000))

// Read-only call (no transaction, no fees)
result, err := call.Call(ctx)
// result.RawResults [][]byte, result.EnergyUsed int64

// Estimate energy
energy, err := call.EstimateEnergy(ctx)

// Build unsigned transaction
tx, err := call.Build(ctx)

// Decode for human-readable display
decoded, err := call.Decode(ctx)

// Sign and broadcast
receipt, err := call.Send(ctx, signer)

// Sign, broadcast, and wait for confirmation
receipt, err := call.SendAndConfirm(ctx, signer)
```

### Contract Builder Options

```go
contract.WithFeeLimit(100_000_000)         // max TRX to burn (in SUN)
contract.WithCallValue(1_000_000)          // TRX to send with call (in SUN)
contract.WithTokenValue("1000001", 500)    // TRC10 token ID and amount
contract.WithPermissionID(2)               // multi-sig permission
```

### Pre-Packed Data via Builder

```go
call := contract.New(conn, "TContractAddr...").
    From("TCallerAddr...").
    WithData(packedBytes). // pre-packed ABI data instead of Method+Params
    Apply(contract.WithFeeLimit(100_000_000))

result, err := call.Call(ctx)
```

### Deferred Error Pattern

Errors can be stored via `SetError()` and surfaced at any terminal call. Used internally by wrappers like `trc20.Token.Transfer()`:

```go
call := contract.New(conn, "TAddr...").
    SetError(fmt.Errorf("custom validation failed")).
    Method("transfer(address,uint256)")

result, err := call.Call(ctx)     // returns the deferred error

// Check explicitly
if call.Err() != nil { ... }
```

## MCP Tools

- `get_contract_abi` — Get smart contract ABI (auto-resolves proxy contracts)
- `list_contract_methods` — Human-readable summary of contract methods with signatures and mutability
- `trigger_constant_contract` — Call read-only (view/pure) methods with decoded results
- `estimate_energy` — Estimate energy cost before calling
- `trigger_contract` — Call a smart contract method (returns unsigned tx)
- `decode_abi_output` — Decode ABI-encoded output hex (return values, revert reasons, panic codes)
