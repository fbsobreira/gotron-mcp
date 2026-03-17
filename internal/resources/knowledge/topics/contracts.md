# TRON Smart Contracts

## Overview

- Solidity-compatible (similar to Ethereum)
- Requires energy for execution
- Read-only calls are free (no transaction needed)
- Fee limit: maximum TRX willing to burn for execution

## SDK: Read-Only Calls

```go
// No transaction created, no fees
result, err := conn.TriggerConstantContract(
    "TCallerAddr...",
    "TContractAddr...",
    "balanceOf(address)",
    `[{"address": "TOwnerAddr..."}]`,
)
// result.ConstantResult contains the return data
```

## SDK: Write Calls

```go
tx, err := conn.TriggerContract(
    "TCallerAddr...",                         // from
    "TContractAddr...",                       // contract
    "transfer(address,uint256)",              // method signature
    `[{"address":"TToAddr..."},{"uint256":"1000000"}]`,  // params as JSON
    100_000_000,                              // fee limit (100 TRX in SUN)
    0,                                        // call value (TRX to send, in SUN)
    "",                                       // token ID (empty for TRX)
    0,                                        // token amount
)
// Returns unsigned transaction
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

// Parse contract event topics into structured map
eventMap, err := abi.ParseTopicsIntoMap(contractABI, eventLog)

// Get parser for specific events
parser, err := abi.GetEventParser(contractABI, "Transfer")
```

## SDK: Overloaded Methods

The SDK supports overloaded contract methods (same name, different parameters):

```go
// The SDK will match the correct method by parameter types
// Both "transfer(address,uint256)" and "transfer(address,uint256,bytes)"
// are handled correctly by GetParser() and GetInputsParser()
```

## Parameter Encoding

Parameters are passed as a JSON array of typed values:

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
    {"bytes[]": ["0xab", "0xcd"]}
]
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

## MCP Tools

- `get_contract_abi` — Get smart contract ABI (auto-resolves proxy contracts)
- `trigger_constant_contract` — Call read-only (view/pure) methods with decoded results
- `estimate_energy` — Estimate energy cost before calling
- `trigger_contract` — Call a smart contract method (returns unsigned tx)
