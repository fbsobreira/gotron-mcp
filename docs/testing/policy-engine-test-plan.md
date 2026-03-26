# Policy Engine Test Plan

Manual test procedure for the wallet policy engine (#16).
Run these tests with an AI agent connected to the MCP server via Claude Code or Claude Desktop.

## Prerequisites

- GoTRON MCP built from `feature/16-policy-engine` branch
- A nile testnet wallet funded with ~100 TRX (use https://nileex.io/join/getJoinPage)
- Network set to `nile`

## Setup

### 1. Create test wallets

```
Use gotron create_wallet with name "policy-wallet"
Use gotron create_wallet with name "unrestricted-wallet"
```

Record both wallet addresses. Fund `policy-wallet` with ~100 nile TRX.

### 2. Create policy config

Save to `~/.gotron-mcp/policy.yaml`:

```yaml
enabled: true

wallets:
  policy-wallet:
    # TRX limits (legacy shorthand — auto-promoted to token_limits.TRX)
    per_tx_limit_trx: 10
    daily_limit_trx: 25
    approval_required_above_trx: 5

    # Per-token limits (human-readable units — set decimals per token)
    token_limits:
      TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf:  # nile USDT
        decimals: 6                          # 1 USDT = 1,000,000 raw units
        per_tx_limit_units: 50               # max 50 USDT per TX
        daily_limit_units: 200               # max 200 USDT/day

    # Only these addresses can receive funds
    whitelist:
      - "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"
      - "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"

  # unrestricted-wallet is NOT listed — no restrictions
```

### 3. Start the MCP server

```bash
make run-http
```

Verify in the logs:
```
Policy engine loaded: 1 wallet(s) configured
```

---

## Test Cases

### A. Tool Registration (sign_transaction / broadcast_transaction removed)

#### A1. sign_transaction should not exist

```
Use gotron sign_transaction with transaction_hex "aabbcc" and wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Tool not found / not registered |

#### A2. broadcast_transaction should not exist

```
Use gotron broadcast_transaction with signed_transaction_hex "aabbcc"
```

| Field | Expected |
|-------|----------|
| Result | Tool not found / not registered |

---

### B. Whitelist Enforcement

#### B1. Transfer to non-whitelisted address (DENY)

```
Build a transfer_trx of 1 TRX from <policy-wallet-address> to TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Error |
| Message | Contains "not in the whitelist" |

#### B2. Transfer to whitelisted address (ALLOW — if under all limits)

```
Build a transfer_trx of 2 TRX from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Success |
| Response | Contains `txid`, `success: true` |

---

### C. Per-Transaction TRX Limit

#### C1. Amount within per-TX limit (ALLOW)

```
Build a transfer_trx of 5 TRX from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | See test D1 — 5 TRX triggers approval threshold first |

> Note: 5 TRX equals `approval_required_above_trx`, so use 4 TRX to test per-TX limit cleanly.

```
Build a transfer_trx of 4 TRX from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Success (4 ≤ 10 per-TX, 4 ≤ 5 approval threshold) |

#### C2. Amount exceeds per-TX limit (DENY)

```
Build a transfer_trx of 15 TRX from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Error |
| Message | Contains "exceeds per-tx limit" |

---

### D. Approval Threshold

#### D1. Amount above approval threshold (APPROVAL REQUIRED)

```
Build a transfer_trx of 8 TRX from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | JSON (not error) |
| `status` | `"approval_required"` |
| `reason` | Contains "approval required" |

#### D2. Amount at or below threshold (ALLOW)

```
Build a transfer_trx of 3 TRX from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Success |

---

### E. Daily TRX Limit

Run these tests in sequence within the same day (UTC). The daily limit is 25 TRX.

#### E1. First transfer: 4 TRX (ALLOW)

```
Build a transfer_trx of 4 TRX from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Success |
| Running total | ~4 TRX |

#### E2. Repeat 4 TRX transfers until daily limit approached

Repeat E1 five more times (total: 24 TRX spent).

#### E3. Transfer that exceeds remaining daily budget (DENY)

```
Build a transfer_trx of 3 TRX from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Error |
| Message | Contains "would exceed daily TRX limit" |

---

### F. Per-Token Limits (TRC20)

> Requires `policy-wallet` to hold nile USDT (`TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf`).
> If no nile USDT available, skip this section and verify via unit tests.

#### F1. TRC20 transfer within per-TX token limit (ALLOW)

```
Build a transfer_trc20 of 30 USDT from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF contract_address TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Success (30 ≤ 50 per-TX token limit) |

#### F2. TRC20 transfer exceeds per-TX token limit (DENY)

```
Build a transfer_trc20 of 60 USDT from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF contract_address TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Error |
| Message | Contains "per-tx token limit" |

#### F3. TRC20 daily token limit accumulation (DENY)

After F1 succeeds, repeat 30 USDT transfers until daily token limit of 200 is approached, then:

```
Build a transfer_trc20 of 30 USDT from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF contract_address TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf, then sign_and_broadcast with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Error (after ~6-7 transfers) |
| Message | Contains "daily token limit" |

---

### G. Unrestricted Wallet (No Policy)

#### G1. Wallet without policy has no restrictions

```
Build a transfer_trx of 100 TRX from <unrestricted-wallet-address> to TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t, then sign_and_broadcast with wallet "unrestricted-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Success (no policy = no limits) |

> Note: This will fail if `unrestricted-wallet` has insufficient TRX, which is expected on testnet. The key check is that no policy error is returned — the failure should be a network/balance error, not a policy denial.

---

### H. Fail-Closed on Unknown Transaction Types

#### H1. Build a non-standard transaction and attempt sign_and_broadcast

This is hard to test manually since all MCP transaction builders produce known types. Verify via unit tests that:
- Unknown contract types return "unable to decode transaction — denied by policy"
- `DecodeContractData` failure with active policy = denied

---

### I. sign_and_confirm (Same Policy Enforcement)

#### I1. sign_and_confirm with whitelisted address, within limits

```
Build a transfer_trx of 2 TRX from <policy-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_confirm with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Success with confirmation |
| Response | Contains `confirmed: true`, `block_number` |

#### I2. sign_and_confirm with non-whitelisted address (DENY)

```
Build a transfer_trx of 2 TRX from <policy-wallet-address> to TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t, then sign_and_confirm with wallet "policy-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Error |
| Message | Contains "not in the whitelist" |

---

### J. Broadcast Failure Rollback

#### J1. Verify daily budget is not consumed on broadcast failure

This is hard to trigger manually. Verify via unit tests or by:
1. Check current daily spend (run a few small transfers, note the count)
2. Build a transfer with an expired transaction (wait >60 seconds after building)
3. Attempt sign_and_broadcast — should fail at broadcast
4. The daily spend should NOT have increased (next transfer should still be within limit)

---

## Summary Checklist

| # | Test | Category | Expected |
|---|------|----------|----------|
| A1 | sign_transaction removed | Registration | Tool not found |
| A2 | broadcast_transaction removed | Registration | Tool not found |
| B1 | Non-whitelisted address | Whitelist | Denied |
| B2 | Whitelisted address | Whitelist | Allowed |
| C1 | Within per-TX limit | Per-TX | Allowed |
| C2 | Exceeds per-TX limit | Per-TX | Denied |
| D1 | Above approval threshold | Approval | approval_required |
| D2 | Below approval threshold | Approval | Allowed |
| E1-E2 | Accumulate daily spend | Daily | Allowed |
| E3 | Exceeds daily limit | Daily | Denied |
| F1 | TRC20 within per-TX token limit | Token Limits | Allowed |
| F2 | TRC20 exceeds per-TX token limit | Token Limits | Denied |
| F3 | TRC20 exceeds daily token limit | Token Limits | Denied |
| G1 | Wallet without policy | No Policy | Unrestricted |
| H1 | Unknown TX type | Fail-Closed | Denied |
| I1 | sign_and_confirm allowed | Confirmation | Confirmed |
| I2 | sign_and_confirm denied | Confirmation | Denied |
| J1 | Broadcast failure rollback | Rollback | Budget preserved |
