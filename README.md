# GoTRON MCP Server

[![Go Reference](https://pkg.go.dev/badge/github.com/fbsobreira/gotron-mcp.svg)](https://pkg.go.dev/github.com/fbsobreira/gotron-mcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/fbsobreira/gotron-mcp)](https://goreportcard.com/report/github.com/fbsobreira/gotron-mcp)
[![CI](https://github.com/fbsobreira/gotron-mcp/actions/workflows/test.yml/badge.svg)](https://github.com/fbsobreira/gotron-mcp/actions/workflows/test.yml)
[![Coverage](https://codecov.io/gh/fbsobreira/gotron-mcp/branch/develop/graph/badge.svg)](https://codecov.io/gh/fbsobreira/gotron-mcp)
[![License](https://img.shields.io/github/license/fbsobreira/gotron-mcp)](LICENSE)

MCP server for TRON blockchain — let AI agents query balances, tokens, blocks, and build transactions.

Built on the [GoTRON SDK](https://github.com/fbsobreira/gotron-sdk) and the [Model Context Protocol](https://modelcontextprotocol.io).

## Two Modes

| | Local Mode | Hosted Mode |
|---|---|---|
| Transport | stdio | Streamable HTTP |
| Read tools | All | All |
| Transaction builders | All (unsigned tx hex) | All (unsigned tx hex) |
| Sign/Broadcast | Opt-in via keystore passphrase | Disabled |
| Install | Required | Zero install |

## Install

### Go Install

```bash
go install github.com/fbsobreira/gotron-mcp/cmd/gotron-mcp@latest
```

### Homebrew

```bash
brew install fbsobreira/tap/gotron-mcp
```

### curl

```bash
curl -fsSL gotron.sh/install-mcp | sh
```

### Docker

```bash
docker run -p 8080:8080 ghcr.io/fbsobreira/gotron-mcp
```

The image starts the HTTP transport. For stdio, append `--transport stdio`:

```bash
docker run -i --rm ghcr.io/fbsobreira/gotron-mcp --transport stdio
```

### MCP Registry

Published to the [MCP Registry](https://registry.modelcontextprotocol.io) as `io.github.fbsobreira/gotron-mcp` on each stable release tag (vX.Y.Z), from v0.4.1, with the Docker image (stdio) and the hosted endpoint.

### Hosted (zero install)

Connect directly to `https://mcp.gotron.sh/mcp` — no installation needed.

### Claude Code plugin

The `gotron` plugin bundles the hosted server with skills for common TRON tasks (send USDT, estimate energy):

```bash
/plugin marketplace add fbsobreira/gotron-plugins
/plugin install gotron@gotron
```

The plugin connects to the hosted server, so it builds unsigned transactions but can't sign. For signing, install the `gotron-local` plugin instead: it runs the `gotron-mcp` binary from your PATH and asks for the keystore passphrase when you enable it.

```bash
/plugin install gotron-local@gotron
```

Install one of the two, not both. The plugins live in [`plugin/`](plugin) and [`plugin-local/`](plugin-local) and install from `main`, so they update with each release.

## Claude Desktop Configuration

### Local mode

```json
{
  "mcpServers": {
    "gotron": {
      "command": "gotron-mcp",
      "args": ["--network", "mainnet"],
      "env": {
        "GOTRON_NODE_API_KEY": "your-trongrid-api-key"
      }
    }
  }
}
```

### Local mode with keystore signing

```json
{
  "mcpServers": {
    "gotron": {
      "command": "gotron-mcp",
      "args": ["--network", "mainnet"],
      "env": {
        "GOTRON_NODE_API_KEY": "your-trongrid-api-key",
        "GOTRON_MCP_KEYSTORE_PASSPHRASE": "your-keystore-passphrase"
      }
    }
  }
}
```

### Hosted mode (zero install)

```json
{
  "mcpServers": {
    "gotron": {
      "url": "https://mcp.gotron.sh/mcp"
    }
  }
}
```

### Claude Code

Add to current project:

```bash
claude mcp add gotron --scope project -- gotron-mcp --network mainnet
```

With keystore:

```bash
claude mcp add gotron --scope user -e GOTRON_MCP_KEYSTORE_PASSPHRASE=your-keystore-passphrase -- gotron-mcp --network mainnet
```

Add globally:

```bash
claude mcp add gotron --scope user -- gotron-mcp --network mainnet
```

Hosted (zero install):

```bash
claude mcp add gotron --scope user --transport http https://mcp.gotron.sh/mcp
```

## Available Tools

### Read-Only (both modes)

| Tool | Description |
|------|-------------|
| `get_account` | Get account balance, bandwidth, energy, and details |
| `get_account_resources` | Get energy/bandwidth usage and limits |
| `get_trc20_balance` | Get TRC20 token balance for an account |
| `get_trc20_token_info` | Get TRC20 token name, symbol, and decimals |
| `get_block` | Get block by number or latest |
| `get_transaction` | Get transaction details by ID |
| `list_witnesses` | List all super representatives |
| `get_contract_abi` | Get smart contract ABI |
| `estimate_energy` | Estimate energy cost for a contract call |
| `trigger_constant_contract` | Call read-only (view/pure) smart contract method |
| `decode_abi_output` | Decode ABI-encoded output or revert reasons from contract calls |
| `list_contract_methods` | Get human-readable summary of contract methods |
| `get_chain_parameters` | Get network parameters |
| `validate_address` | Validate a TRON address |
| `get_energy_prices` | Get energy price history |
| `get_bandwidth_prices` | Get bandwidth price history |
| `list_proposals` | List governance proposals |
| `get_network` | Get current network connection info |
| `get_pq_capabilities` | Report node and build support for post-quantum (TIP-899) signing |

### Transaction Builders (both modes)

| Tool | Description |
|------|-------------|
| `transfer_trx` | Build unsigned TRX transfer |
| `transfer_trc20` | Build unsigned TRC20 token transfer |
| `freeze_balance` | Build unsigned stake TRX for energy/bandwidth (Stake 2.0) |
| `unfreeze_balance` | Build unsigned unstake TRX (Stake 2.0) |
| `vote_witness` | Build unsigned vote for super representatives |
| `trigger_contract` | Build unsigned smart contract call |

### Sign & Broadcast (local mode + keystore)

| Tool | Description |
|------|-------------|
| `sign_transaction` | Sign transaction using local keystore |
| `broadcast_transaction` | Broadcast signed transaction to network |

### Post-quantum signing (TIP-899, Nile)

`create_wallet(name, key_type?)` accepts `ecdsa` (default), `ml_dsa_44` or `fn_dsa_512`; `list_wallets` and signing results report `key_type`. `sign_transaction`, `sign_and_broadcast`, `sign_and_confirm` and `request_limit_override` accept PQ wallets with the same policy limits, Telegram approval and elicitation as ECDSA wallets.

- TIP-899 is a draft, live only on Nile (`grpc.nile.trongrid.io:50051`); governance switches each scheme on or off and mainnet reports none. Use `get_pq_capabilities` to check the node and this build.
- Before any policy check or approval, a PQ signature requires that the node has the scheme enabled and that the wallet's address is a key of the transaction's permission (`permission_id`) on the owner account. A fresh PQ address must receive TRX first.
- PQ keys are stored encrypted with `GOTRON_MCP_KEYSTORE_PASSPHRASE` under `<keystore-dir>/pq-keys/<name>/` (wallet-cli compatible, same layout as tronctl). Point `--keystore-dir` at a tronctl config root (e.g. `~/.tronctl`) to reuse its keys; the passphrase must match their password. Wallet names are unique across ECDSA and PQ wallets; `list_wallets` flags `duplicate_name: true` and such a wallet cannot sign until one copy is renamed.
- The signature goes into `pq_auth_sig` (txid unchanged); a transaction carries at most 5 signatures, ECDSA and PQ combined. PQ transactions are about 10-22x larger than ECDSA ones (roughly 10x for FN-DSA-512, 22x for ML-DSA-44), so they use more bandwidth.
- ML-DSA-44 is pure Go and works in every build. FN-DSA-512 (Falcon-512, currently enabled on Nile) needs a build with `-tags falcon`, CGO and liboqs installed via pkg-config: `CGO_ENABLED=1 go build -tags falcon ./cmd/gotron-mcp`. Without it, FN-DSA create/sign returns an error pointing to the `gotron-mcp-pq` build; release binaries for it are planned (GOT-97).
- Not supported yet: adding a PQ key to an existing account's permissions from MCP (GOT-98; use tronctl or the [SDK guide](https://github.com/fbsobreira/gotron-sdk/blob/master/docs/post-quantum.md)) and co-signing one transaction with several MCP wallets (GOT-99).

## Configuration

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--node` | `GOTRON_MCP_NODE` | (per network) | TRON gRPC node address |
| `--api-key` | `GOTRON_NODE_API_KEY` | — | TronGrid API key |
| `--network` | `GOTRON_MCP_NETWORK` | `mainnet` | Network: mainnet, nile, shasta |
| `--transport` | — | `stdio` | Transport: stdio, http |
| `--port` | — | `8080` | HTTP server port |
| `--bind` | — | `127.0.0.1` | HTTP server bind address |
| `--fallback-node` | `GOTRON_MCP_FALLBACK_NODE` | — | Fallback gRPC node (auto-failover) |
| `--keystore-dir` | `GOTRON_MCP_KEYSTORE_DIR` | `~/.gotron-mcp/wallets` | Wallet directory |
| `--keystore-pass` | `GOTRON_MCP_KEYSTORE_PASSPHRASE` | — | Keystore passphrase; wallet and sign tools are registered only when set |
| `--tls` | — | `false` | Use TLS for gRPC connection (default: plaintext) |

### Network Presets

| Network | Default Node |
|---------|-------------|
| mainnet | `grpc.trongrid.io:50051` |
| nile | `grpc.nile.trongrid.io:50051` |
| shasta | `grpc.shasta.trongrid.io:50051` |

Explicit `--node` overrides network presets.

## Security

- The server **never** stores or manages private keys directly
- All write tools return **unsigned transaction hex** — the user decides how to sign
- Keystore signing is opt-in: wallet and sign tools are registered only when a keystore passphrase is set
- In hosted (HTTP) mode, all write and sign tools are automatically disabled
- API key is optional and only needed for TronGrid rate limits

## Development

```bash
# Build
make build

# Format code
make fmt

# Run tests
make test

# Lint
make lint

# Run HTTP mode locally
make run-http
```

## Community

Built and maintained by a [CryptoChain](https://tronscan.org/#/representative/TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF) community developer.

If you find this project useful, consider supporting TRON governance by voting for the CryptoChain Super Representative:

- [Vote on TronScan](https://tronscan.org/#/representative/TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF)
- SR Address: `TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF`

## License

[LGPL-3.0](LICENSE) — same as the [GoTRON SDK](https://github.com/fbsobreira/gotron-sdk).

## Links

- [GoTRON SDK](https://github.com/fbsobreira/gotron-sdk)
- [GoTRON](https://gotron.sh)
- [MCP Specification](https://modelcontextprotocol.io)
