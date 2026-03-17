# TRON Blockchain Overview

## What is TRON?

TRON is a decentralized blockchain platform focused on content sharing, entertainment, and decentralized applications (dApps). It uses a Delegated Proof of Stake (DPoS) consensus mechanism.

## Addresses

- TRON addresses are 34 characters long, starting with `T`
- Base58Check encoded (similar to Bitcoin)
- Internally represented as 21-byte hex with `0x41` prefix
- Example: `TXyz1234567890abcdefghijklmnopqrst`

## Native Currency: TRX

- TRX is the native token of the TRON network
- Smallest unit: **SUN** (1 TRX = 1,000,000 SUN)
- Used for transaction fees, staking, and governance voting

## Resources: Energy and Bandwidth

TRON uses a resource model instead of gas fees:

### Bandwidth
- Consumed by all transactions (basic transfers, token transfers)
- Each account gets **1,500 free bandwidth points per day**
- Additional bandwidth obtained by staking TRX
- If bandwidth is insufficient, TRX is burned as a fee

### Energy
- Consumed only by smart contract interactions
- No free energy allocation
- Must be obtained by staking TRX for energy
- If energy is insufficient, TRX is burned as a fee

## Staking (Stake 2.0)

- Stake TRX to obtain Energy or Bandwidth resources
- `FreezeBalanceV2` — stake TRX for a specific resource
- `UnfreezeBalanceV2` — unstake (14-day waiting period)
- Resources can be delegated to other accounts
- Staked TRX also grants voting power for governance

## Token Standards

### TRC10
- Native token standard built into the protocol
- Low-cost creation and transfer
- No smart contract required

### TRC20
- Smart contract-based tokens (similar to ERC-20 on Ethereum)
- Requires energy for transfers
- Standard methods: `name()`, `symbol()`, `decimals()`, `balanceOf(address)`, `transfer(address, uint256)`
- Popular tokens: USDT, USDC, WTRX

## Smart Contracts

- Solidity-compatible (similar to Ethereum)
- Deployed and interacted with via `TriggerContract`
- Read-only calls via `TriggerConstantContract` (no transaction needed)
- Energy estimation available via `EstimateEnergy`

## Governance

### Super Representatives (SR)
- 27 Super Representatives produce blocks
- Elected by TRX holders through voting
- Must stake TRX to gain voting power (1 staked TRX = 1 vote)
- SRs earn block rewards and voting rewards

### Proposals
- SRs can create network parameter change proposals
- Other SRs vote to approve/reject
- Parameters include energy prices, bandwidth prices, and other network settings

## Network Endpoints

| Network | gRPC Endpoint | Purpose |
|---------|--------------|---------|
| Mainnet | `grpc.trongrid.io:50051` | Production |
| Nile | `grpc.nile.trongrid.io:50051` | Testnet |
| Shasta | `grpc.shasta.trongrid.io:50051` | Testnet |

## Transaction Flow

1. **Build** — Create an unsigned transaction using the SDK
2. **Sign** — Sign with private key (local keystore, hardware wallet, etc.)
3. **Broadcast** — Submit signed transaction to the network
4. **Confirm** — Transaction is included in a block (~3 second block time)

## Multi-Signature Accounts

- TRON accounts support multi-signature permissions
- **Owner** (ID 0): Full control, can modify permissions
- **Witness** (ID 1): Block production (SRs only)
- **Active** (ID 2+): Customizable, limited operations
- Transactions can require multiple signatures based on permission thresholds
- Useful for exchanges, DAOs, and shared wallets

## Key Concepts

- **Block time**: ~3 seconds
- **Transaction finality**: ~1 minute (19 confirmed blocks)
- **Fee limit**: Maximum TRX willing to burn for smart contract execution
- **Account activation**: New accounts must receive at least 0.1 TRX to be activated on-chain
- **Resource delegation**: Stake owners can delegate energy/bandwidth to other accounts
- **Permission ID**: Specifies which permission set to use when signing multi-sig transactions
