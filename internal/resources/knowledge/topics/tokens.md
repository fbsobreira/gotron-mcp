# TRON Tokens

## TRC10

- Native token standard built into the protocol
- Low-cost creation and transfer
- No smart contract required

## TRC20

- Smart contract-based tokens (similar to ERC-20 on Ethereum)
- Requires energy for transfers
- Standard methods: `name()`, `symbol()`, `decimals()`, `balanceOf(address)`, `transfer(address, uint256)`
- Popular tokens: USDT, USDC, WTRX

## SDK: Token Operations

```go
import "math/big"

// Get token metadata
name, _ := conn.TRC20GetName("TContractAddr...")
symbol, _ := conn.TRC20GetSymbol("TContractAddr...")
decimals, _ := conn.TRC20GetDecimals("TContractAddr...")  // returns *big.Int

// Get balance
balance, _ := conn.TRC20ContractBalance("TOwnerAddr...", "TContractAddr...")
// balance is *big.Int in raw units (divide by 10^decimals for human-readable)

// Transfer TRC20 tokens (opts ...TRC20Option supported)
amount := new(big.Int).Mul(big.NewInt(100), new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil))  // 100 USDT (6 decimals)
feeLimit := int64(100_000_000)  // 100 TRX in SUN
tx, err := conn.TRC20SendCtx(ctx, "TFromAddr...", "TToAddr...", "TContractAddr...", amount, feeLimit)

// Dry-run energy estimation (no transaction created)
tx, err := conn.TRC20SendCtx(ctx, from, to, contract, amount, feeLimit, client.WithEstimate())

// WithEstimate() also works with TRC20ApproveCtx and TRC20TransferFromCtx
```

## Amount Conversion

```
TRC20 amounts use *big.Int with token-specific decimals
USDT (6 decimals):  "100.5" → 100500000
WTRX (18 decimals): "1.0"   → 1000000000000000000
```

## SDK: Typed TRC20 Wrapper (v0.25.2+)

The `pkg/standards/trc20` package provides a type-safe TRC20 interface with automatic decimal handling:

```go
import "github.com/fbsobreira/gotron-sdk/pkg/standards/trc20"

token := trc20.New(conn, "TContractAddr...")

// With metadata caching (recommended for repeated queries)
cache := trc20.NewMetadataCache(256)
token = trc20.New(conn, "TContractAddr...", trc20.WithCache(cache))
// Name, symbol, and decimals are cached after first fetch — thread-safe LRU

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
// bal.Display string   — formatted with decimals (e.g., "1,000.50")
// bal.Symbol  string   — token symbol

// Allowance
allowance, err := token.Allowance(ctx, "TOwner...", "TSpender...")
```

### TRC20 Write Methods (Fluent Builder)

Write methods return `*contract.ContractCall` for fluent terminal operations:

```go
// Transfer tokens — returns builder, not raw transaction
receipt, err := token.Transfer(from, to, amount).Send(ctx, signer)

// Approve spender
receipt, err := token.Approve(from, spender, amount).Send(ctx, signer)

// Transfer on behalf (requires prior approval)
receipt, err := token.TransferFrom(caller, from, to, amount).Send(ctx, signer)

// With options
receipt, err := token.Transfer(from, to, amount,
    contract.WithFeeLimit(150_000_000),
).Send(ctx, signer)

// Estimate energy before sending
energy, err := token.Transfer(from, to, amount).EstimateEnergy(ctx)
```

## MCP Tools

- `get_trc20_balance` — Get TRC20 token balance for an account
- `get_trc20_token_info` — Get token name, symbol, decimals, and total supply
- `transfer_trc20` — Create unsigned TRC20 transfer transaction
- `estimate_trc20_energy` — Estimate energy cost for a TRC20 transfer (dry-run)
