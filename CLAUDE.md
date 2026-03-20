# GoTRON MCP Server

MCP server wrapping the GoTRON SDK for TRON blockchain interaction.

## Quick Reference

```bash
make build          # Build binary to bin/
make fmt            # Format with goimports
make test           # Run tests with race detector
make lint           # Run golangci-lint
make run-http       # Run HTTP mode locally on :8080
GOTRON_MCP_KEYSTORE_PASSPHRASE=... # Passphrase for wallet encryption
```

## Architecture

- `cmd/gotron-mcp/main.go` — Entry point, config, transport setup
- `internal/config/` — CLI flags + env var parsing
- `internal/server/` — MCP server creation, tool registration by mode
- `internal/tools/` — One file per domain, each exports `Register*Tools`
- `internal/health/` — Health endpoint with cached node status
- `internal/util/` — Amount conversion (TRX/SUN, TRC20)

## Adding a New Tool

1. Add handler in the appropriate `internal/tools/*.go` file
2. Use the pattern: `func handleToolName(pool *nodepool.Pool) server.ToolHandlerFunc`
3. Register in the `Register*Tools` function
4. If it's a new file, wire it up in `internal/server/server.go`
5. Read-only tools go in the read section; transaction builders are always registered; wallet management and sign/broadcast require local mode + keystore
   - Wallet management: create and list wallets (create_wallet, list_wallets) [local mode + keystore]
   - Sign transactions: sign only, sign+broadcast, or sign+broadcast+confirm (sign_transaction, sign_and_broadcast, sign_and_confirm, broadcast_transaction) [local mode + keystore]

## GoTRON SDK Usage

Full SDK docs: https://github.com/fbsobreira/gotron-sdk/tree/master/docs

### Client Setup

```go
import "github.com/fbsobreira/gotron-sdk/pkg/client"

conn := client.NewGrpcClient("grpc.trongrid.io:50051")
// Use ONE of the following Start options:
conn.Start()                    // with TLS
// OR
conn.Start(grpc.WithTransportCredentials(insecure.NewCredentials())) // without TLS
conn.SetAPIKey("your-key")     // for trongrid rate limits
defer conn.Stop()
```

### Key SDK Methods

**Account:** `GetAccountCtx(ctx, addr)`, `GetAccountResourceCtx(ctx, addr)`
**TRC20:** `TRC20ContractBalanceCtx(ctx, addr, contract)`, `TRC20GetNameCtx/SymbolCtx/DecimalsCtx(ctx, contract)`, `TRC20SendCtx(ctx, from, to, contract, amount, feeLimit)`
**Blocks:** `GetNowBlockCtx(ctx)`, `GetBlockByNumCtx(ctx, num)`
**Transactions:** `GetTransactionByIDCtx(ctx, id)`, `GetTransactionInfoByIDCtx(ctx, id)`, `TransferCtx(ctx, from, to, amount)`, `BroadcastCtx(ctx, tx)`
**Contracts:** `TriggerContractCtx(ctx, from, contract, method, params, feeLimit, amount, tokenID, tokenAmount)`, `EstimateEnergyCtx(ctx, ...)`, `GetContractABICtx(ctx, contract)`
**Staking:** `FreezeBalanceV2Ctx(ctx, from, resource, amount)`, `UnfreezeBalanceV2Ctx(ctx, from, resource, amount)`
**Governance:** `ListWitnessesCtx(ctx)`, `VoteWitnessAccountCtx(ctx, from, votes)`, `ProposalsListCtx(ctx)`
**Network:** `GetEnergyPriceHistoryCtx(ctx)`, `GetBandwidthPriceHistoryCtx(ctx)`, `GetNodeInfoCtx(ctx)`

### Address Utilities

```go
import "github.com/fbsobreira/gotron-sdk/pkg/address"

addrB58, err := address.Base58ToAddress("TXyz...")  // validates + converts
addrHex, err := address.HexToAddress("41...")       // validates + converts
addr := address.BytesToAddress(rawBytes)            // from raw bytes (no error)
addr.String()      // base58
addr.Hex()         // hex with 41 prefix
addr.IsValid()     // check validity (validates checksum)
```

### Amount Conventions

- All SDK methods use **SUN** (int64): 1 TRX = 1,000,000 SUN
- All MCP tool inputs/outputs use **TRX** (human-readable strings)
- Use `util.TRXToSun()` / `util.SunToTRX()` for conversion
- TRC20 amounts use `*big.Int` with token-specific decimals

### Context Propagation

- Always use `*Ctx` SDK method variants (e.g., `GetAccountCtx(ctx, addr)` not `GetAccount(addr)`)
- Pass the MCP request `ctx` from the handler through to all gRPC calls
- For non-request contexts (health checks), use `context.WithTimeout(context.Background(), 5*time.Second)`

### Error Handling

- Validate addresses with `validateAddress()` before any gRPC call
- Return tool errors via `mcp.NewToolResultError()` (not Go errors)
- Wrap gRPC errors with context: `fmt.Sprintf("tool_name: %v", err)`

### Write Tools Pattern

All write tools return unsigned transaction hex — never auto-sign:

```go
tx, err := conn.TransferCtx(ctx, from, to, sun)
// handle err...
txBytes, err := proto.Marshal(tx.Transaction)
// handle err...
result["transaction_hex"] = hex.EncodeToString(txBytes)
result["txid"] = hex.EncodeToString(tx.Txid)
```

## Conventions

- Use GoTRON or GoTRON SDK (not Gotron) when referring to the project
- Follow Go conventions: gofmt, goimports, effective Go
- Handle all errors explicitly — no blank `_` for error returns
- Table-driven tests
- Files must end with exactly one newline character — no extra blank line
