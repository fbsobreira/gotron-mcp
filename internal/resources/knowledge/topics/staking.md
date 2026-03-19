# TRON Staking & Resources

## Resources: Energy and Bandwidth

TRON uses a resource model instead of gas fees:

### Bandwidth
- Consumed by all transactions (basic transfers, token transfers)
- Each account gets 1,500 free bandwidth points per day
- Additional bandwidth obtained by staking TRX
- If bandwidth is insufficient, TRX is burned as a fee

### Energy
- Consumed only by smart contract interactions
- No free energy allocation
- Must be obtained by staking TRX for energy
- If energy is insufficient, TRX is burned as a fee

## Staking (Stake 2.0)

- Stake TRX to obtain Energy or Bandwidth resources
- Staked TRX also grants voting power for governance (1 staked TRX = 1 vote)
- 14-day waiting period after unstaking
- Resources can be delegated to other accounts

## SDK: Staking Operations

```go
import "github.com/fbsobreira/gotron-sdk/pkg/proto/core"

// Stake TRX for energy
tx, err := conn.FreezeBalanceV2("TAddr...", core.ResourceCode_ENERGY, 10_000_000)  // 10 TRX

// Stake TRX for bandwidth
tx, err := conn.FreezeBalanceV2("TAddr...", core.ResourceCode_BANDWIDTH, 10_000_000)

// Unstake (14-day waiting period)
tx, err := conn.UnfreezeBalanceV2("TAddr...", core.ResourceCode_ENERGY, 10_000_000)

// Note: amounts must be positive (validated by SDK)
```

## SDK: Resource Delegation

```go
// Delegate resources to another account
tx, err := conn.DelegateResource(
    "TFromAddr...",                // owner
    "TToAddr...",                  // receiver
    core.ResourceCode_ENERGY,     // resource type
    10_000_000,                   // amount in SUN
    false,                        // lock (prevent early undelegation)
    0,                            // lock period (blocks)
)

// Undelegate resources
tx, err := conn.UnDelegateResource(
    "TFromAddr...",                // owner
    "TToAddr...",                  // receiver
    core.ResourceCode_ENERGY,     // resource type
    10_000_000,                   // amount in SUN
)

// Query delegated resources
delegations, err := conn.GetDelegatedResourcesV2("TAddr...")

// Query received delegations
received, err := conn.GetReceivedDelegatedResourcesV2("TAddr...")

// Check max delegatable amount
maxSize, err := conn.GetCanDelegatedMaxSize("TAddr...", 0)  // 0=bandwidth, 1=energy
```

## SDK: Energy Estimation

```go
import "errors"

estimate, err := conn.EstimateEnergy(
    "TCallerAddr...", "TContractAddr...",
    "transfer(address,uint256)",
    `[{"address":"TToAddr..."},{"uint256":"1000000"}]`,
    0, "", 0,
)
if errors.Is(err, client.ErrEstimateEnergyNotSupported) {
    // Node doesn't support energy estimation — try a different endpoint
}
energyNeeded := estimate.EnergyRequired
```

## SDK: Fluent Staking Builder (v0.25.2+)

```go
import (
    "github.com/fbsobreira/gotron-sdk/pkg/txbuilder"
    "github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

builder := txbuilder.New(conn)

// Stake TRX for energy
receipt, err := builder.FreezeV2(from, 10_000_000, core.ResourceCode_ENERGY).
    Send(ctx, signer)

// Unstake
receipt, err := builder.UnfreezeV2(from, 10_000_000, core.ResourceCode_ENERGY).
    Send(ctx, signer)

// Withdraw expired unfrozen TRX (after 14-day waiting period)
receipt, err := builder.WithdrawExpireUnfreeze(from, 0).Send(ctx, signer)

// Delegate with lock period
receipt, err := builder.DelegateResource(from, to, core.ResourceCode_ENERGY, 10_000_000).
    Lock(86400). // lock for 86400 blocks (~3 days)
    Send(ctx, signer)

// Undelegate
receipt, err := builder.UnDelegateResource(from, to, core.ResourceCode_ENERGY, 10_000_000).
    Send(ctx, signer)

// All staking builders support fluent memo and permission_id
builder.FreezeV2(from, amt, res).WithMemo("stake").WithPermissionID(2).Build(ctx)
```

## MCP Tools

- `get_account_resources` — Get energy/bandwidth usage and limits
- `freeze_balance` — Stake TRX for energy or bandwidth
- `unfreeze_balance` — Unstake TRX
- `withdraw_expire_unfreeze` — Withdraw TRX after 14-day unstaking period
- `delegate_resource` — Delegate energy or bandwidth to another address
- `undelegate_resource` — Reclaim previously delegated resources
- `estimate_energy` — Estimate energy cost for a contract call
- `get_energy_prices` — Get current energy prices
- `get_bandwidth_prices` — Get current bandwidth prices
