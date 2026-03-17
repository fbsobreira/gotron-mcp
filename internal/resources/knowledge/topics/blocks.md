# TRON Blocks

## Overview

- Block time: ~3 seconds
- Transaction finality: ~1 minute (19 confirmed blocks)
- Blocks produced by elected Super Representatives

## SDK: Block Operations

```go
// Get latest block
block, err := conn.GetNowBlock()
blockNum := block.BlockHeader.RawData.Number
timestamp := block.BlockHeader.RawData.Timestamp
txCount := len(block.Transactions)

// Witness address helpers (returns base58 string)
witness := client.BlockExtentionWitnessBase58(block)
// Or as address.Address type:
witnessAddr := client.BlockExtentionWitnessAddress(block)

// Get specific block by number
block, err := conn.GetBlockByNum(12345678)

// Get block by ID
block, err := conn.GetBlockByID("0000000000bc614e...")
```

## SDK: Network Info

```go
// Node information
nodeInfo, err := conn.GetNodeInfo()
// nodeInfo.BeginSyncNum, nodeInfo.Block, nodeInfo.SolidityBlock

// Structured energy price history
prices, err := conn.GetEnergyPriceHistory()
// Returns []client.PriceEntry{{Timestamp: 0, Price: 100}, ...}

// Structured bandwidth price history
bwPrices, err := conn.GetBandwidthPriceHistory()

// Memo fee history
memoFees, err := conn.GetMemoFeeHistory()
```

## MCP Tools

- `get_block` — Get block by number or latest
- `get_chain_parameters` — Get network parameters and node info
- `get_energy_prices` — Get energy price history
- `get_bandwidth_prices` — Get bandwidth price history
