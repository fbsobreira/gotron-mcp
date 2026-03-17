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

// Transfer TRC20 tokens
amount := new(big.Int).Mul(big.NewInt(100), new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil))  // 100 USDT (6 decimals)
feeLimit := int64(100_000_000)  // 100 TRX in SUN
tx, err := conn.TRC20Send("TFromAddr...", "TToAddr...", "TContractAddr...", amount, feeLimit)
```

## Amount Conversion

```
TRC20 amounts use *big.Int with token-specific decimals
USDT (6 decimals):  "100.5" → 100500000
WTRX (18 decimals): "1.0"   → 1000000000000000000
```

## MCP Tools

- `get_trc20_balance` — Get TRC20 token balance for an account
- `get_trc20_token_info` — Get token name, symbol, and decimals
- `transfer_trc20` — Create unsigned TRC20 transfer transaction
