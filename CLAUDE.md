# GoTRON MCP Server

MCP server wrapping the GoTRON SDK for TRON blockchain interaction.

## Quick Reference

```bash
make build          # Build binary to bin/
make fmt            # Format with goimports
make test           # Run tests with race detector
make lint           # Run golangci-lint
make run-http       # Run HTTP mode locally on :8080
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
5. Read-only tools go in the read section; transaction builders are always registered; sign/broadcast require local mode + keystore

## GoTRON SDK Usage

Full SDK docs: https://github.com/fbsobreira/gotron-sdk/tree/master/docs

### Client Setup

```go
import "github.com/fbsobreira/gotron-sdk/pkg/client"

conn := client.NewGrpcClient("grpc.trongrid.io:50051")
conn.Start()                    // with TLS (default for trongrid)
conn.Start(grpc.WithTransportCredentials(insecure.NewCredentials())) // without TLS
conn.SetAPIKey("your-key")     // for trongrid rate limits
defer conn.Stop()
```

### Key SDK Methods

**Account:** `GetAccount(addr)`, `GetAccountResource(addr)`
**TRC20:** `TRC20ContractBalance(addr, contract)`, `TRC20GetName/Symbol/Decimals(contract)`, `TRC20Send(from, to, contract, amount, feeLimit)`
**Blocks:** `GetNowBlock()`, `GetBlockByNum(num)`
**Transactions:** `GetTransactionByID(id)`, `GetTransactionInfoByID(id)`, `Transfer(from, to, amount)`, `Broadcast(tx)`
**Contracts:** `TriggerContract(from, contract, method, params, feeLimit, amount, tokenID, tokenAmount)`, `EstimateEnergy(...)`, `GetContractABI(contract)`
**Staking:** `FreezeBalanceV2(from, resource, amount)`, `UnfreezeBalanceV2(from, resource, amount)`
**Governance:** `ListWitnesses()`, `VoteWitnessAccount(from, votes)`, `ProposalsList()`
**Network:** `GetEnergyPrices()`, `GetBandwidthPrices()`, `GetNodeInfo()`

### Address Utilities

```go
import "github.com/fbsobreira/gotron-sdk/pkg/address"

addr, err := address.Base58ToAddress("TXyz...")  // validates + converts
addr := address.HexToAddress("41...")             // no validation
addr.String()   // base58
addr.Hex()      // hex with 41 prefix
addr.IsValid()  // check validity
```

### Amount Conventions

- All SDK methods use **SUN** (int64): 1 TRX = 1,000,000 SUN
- All MCP tool inputs/outputs use **TRX** (human-readable strings)
- Use `util.TRXToSun()` / `util.SunToTRX()` for conversion
- TRC20 amounts use `*big.Int` with token-specific decimals

### Context Propagation

- Always use `*Ctx` SDK method variants (e.g., `GetAccountCtx(ctx, addr)` not `GetAccount(addr)`)
- Pass the MCP request `ctx` from the handler through to all gRPC calls
- For non-request contexts (health checks), use `context.Background()`

### Error Handling

- Validate addresses with `validateAddress()` before any gRPC call
- Return tool errors via `mcp.NewToolResultError()` (not Go errors)
- Wrap gRPC errors with context: `fmt.Sprintf("tool_name: %v", err)`

### Write Tools Pattern

All write tools return unsigned transaction hex — never auto-sign:

```go
tx, err := conn.TransferCtx(ctx, from, to, sun)
txBytes, _ := proto.Marshal(tx.Transaction)
result["transaction_hex"] = hex.EncodeToString(txBytes)
result["txid"] = hex.EncodeToString(tx.Txid)
```

## Conventions

- Use GoTRON or GoTRON SDK (not Gotron) when referring to the project
- Follow Go conventions: gofmt, goimports, effective Go
- Handle all errors explicitly — no blank `_` for error returns
- Table-driven tests
- Empty line at end of files
