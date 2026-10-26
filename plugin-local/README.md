# GoTRON local signing plugin for Claude Code

The `gotron-local` plugin gives Claude tools and skills for the TRON
blockchain, and it can sign with wallets kept on your machine. It runs the
`gotron-mcp` binary locally over stdio. On top of everything the hosted
`gotron` plugin does, it can create and list wallets and sign and broadcast
transactions. Signing checks the wallet policy and asks for approval when the
policy requires it.

## Install

Install the binary first, so `gotron-mcp` is on your PATH:

```bash
brew install fbsobreira/tap/gotron-mcp
# or: curl -fsSL gotron.sh/install-mcp | sh
# or: go install github.com/fbsobreira/gotron-mcp/cmd/gotron-mcp@latest
```

Public marketplace installation is not available yet. To use local signing
now, follow the [Claude Code MCP setup](../README.md#claude-code) in the main
README after installing the binary.

When plugins become available, install either this plugin or the hosted
`gotron` plugin, not both. With both, Claude sees two servers that can be on
different networks.

## Settings

When you enable the plugin, Claude Code asks for:

- **Network**: mainnet (the default), nile or shasta.
- **Keystore passphrase** (required): encrypts and unlocks your wallets.
  Claude Code keeps it in your system's secure credential storage and passes
  it to the binary as an environment variable. The server doesn't start
  until it is set.
- **Keystore directory**: leave empty for `~/.gotron-mcp/wallets`.
- **TronGrid API key** (optional): for higher rate limits. Stored like the
  passphrase.

## Skills

- `send-usdt`: sends USDT or another TRC20 token. It checks the network, token
  contract, recipient, balance and energy cost, asks for confirmation, then
  signs with your wallet.
- `estimate-energy`: explains what a transfer or contract call will cost in
  energy, bandwidth and burned TRX, and compares burning TRX with staking.

## What the plugin sends and fetches

The binary runs on your machine. Private keys stay in the keystore directory,
encrypted with your passphrase, and never leave your machine. The binary
connects to:

- the TRON gRPC node for the selected network (TronGrid's public node by
  default), to read chain data and broadcast the transactions you sign;
- the TronGrid REST API, for transaction history;
- CoinGecko, for token prices.

When a wallet policy is set to Telegram approval, approval requests go to the
Telegram bot you configure.

## Links

- Source, wallet policy and full tool list: https://github.com/fbsobreira/gotron-mcp
- Documentation: https://gotron.sh/docs/mcp-server
- License: LGPL-3.0
