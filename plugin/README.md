# GoTRON plugin for Claude Code

The `gotron` plugin gives Claude tools and skills for the TRON blockchain. It
can read accounts, balances, TRC20 tokens, blocks, transactions, resources,
super representatives and governance proposals. It can also build unsigned
transactions for TRX and TRC20 transfers, staking, delegation, voting and
contract calls. It can't sign or broadcast.

## Install

Public marketplace installation is not available yet. To use the hosted MCP
server now, follow the [Claude Code MCP setup](../README.md#claude-code) in
the main README.

## Skills

- `send-usdt`: sends USDT or another TRC20 token. It checks the network, token
  contract, recipient, balance and energy cost, and asks for confirmation
  before it builds the transaction.
- `estimate-energy`: explains what a transfer or contract call will cost in
  energy, bandwidth and burned TRX, and compares burning TRX with staking.

## What the plugin sends and fetches

The plugin connects to one remote MCP server, `https://mcp.gotron.sh/mcp`,
run by the GoTRON maintainer. Each tool call sends that server its arguments:
TRON addresses, token contract addresses, amounts, transaction IDs and
contract call parameters. The server queries public TRON nodes (TronGrid) and
CoinGecko, for token prices, and returns the result.

No private key, seed phrase or keystore ever reaches the server. It has no
signing tools: transactions come back unsigned, and you sign them with your
own wallet. TRON addresses and transactions are public on-chain data, but the
server sees which addresses you ask about.

## Signing locally

The [`gotron-local`](../plugin-local) plugin source supports local keystore
signing through a `gotron-mcp` binary on your machine. Until marketplace
installation is available, use the local signing setup in the main README.
When plugins become available, install one of the two, not both.

## Links

- Source and full tool list: https://github.com/fbsobreira/gotron-mcp
- Documentation: https://gotron.sh/docs/mcp-server
- License: LGPL-3.0
