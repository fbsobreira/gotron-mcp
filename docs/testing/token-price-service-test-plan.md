# Token Price Service Test Plan

Manual test procedure for the token price service and USD policy limits (#85).
Run these tests with an AI agent connected to the MCP server.

## Prerequisites

- GoTRON MCP built from `feature/85-token-price-service` branch
- Network access to CoinGecko API (no key needed for free tier)
- A nile testnet wallet with TRX for policy tests

## Setup

```bash
make build
```

---

## Test Cases

### A. get_token_price Tool

#### A1. TRX price

```
Use gotron get_token_price for token "TRX"
```

| Field | Expected |
|-------|----------|
| `token` | `"TRX"` |
| `usd_price` | Non-zero positive number (e.g., ~0.10-0.30) |
| `currency` | `"USD"` |

#### A2. USDT price (by contract address)

```
Use gotron get_token_price for token "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
```

| Field | Expected |
|-------|----------|
| `usd_price` | Close to 1.0 (stablecoin) |

#### A3. USDC price

```
Use gotron get_token_price for token "TEkxiTehnzSmSe2XqrBj4w32RUN966rdz8"
```

| Field | Expected |
|-------|----------|
| `usd_price` | Close to 1.0 |

#### A4. BTT price

```
Use gotron get_token_price for token "TAFjULxiVgT4qWk6UZwjqwZXTSaGaqnVp4"
```

| Field | Expected |
|-------|----------|
| `usd_price` | Very small positive number |

#### A5. Unknown token

```
Use gotron get_token_price for token "TUnknownContractAddress123456789"
```

| Field | Expected |
|-------|----------|
| Result | Error: "no price data for contract..." |

#### A6. Case insensitive TRX

```
Use gotron get_token_price for token "trx"
```

| Field | Expected |
|-------|----------|
| Result | Same as A1 (case insensitive) |

#### A7. Empty token

```
Use gotron get_token_price for token ""
```

| Field | Expected |
|-------|----------|
| Result | Error: "token is required" |

---

### B. Caching Behavior

#### B1. Repeated calls use cache

```
Use gotron get_token_price for token "TRX"
```

Call twice quickly. Second call should return instantly (cached, no CoinGecko hit).

| Field | Expected |
|-------|----------|
| Both responses | Same price |
| Second call | Noticeably faster |

#### B2. Cache expires after TTL

Wait 60+ seconds between calls.

| Field | Expected |
|-------|----------|
| Price | May differ slightly (fresh fetch) |

---

### C. USD Policy Limits — per_tx_limit_usd

#### C1. Setup policy with USD per-TX limit

```yaml
# policy.yaml
enabled: true
wallets:
  test-wallet:
    per_tx_limit_usd: 10
    token_limits:
      TRX:
        per_tx_limit_units: 10000    # high enough not to trigger
        daily_limit_units: 50000
    whitelist:
      - "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"
```

#### C2. Transfer below USD limit

Calculate: if TRX = $0.12, then 50 TRX = $6 (below $10 limit).

```
Build a transfer_trx of 50 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "test-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Success — $6 < $10 per-TX limit |

#### C3. Transfer above USD limit

Calculate: if TRX = $0.12, then 100 TRX = $12 (above $10 limit).

```
Build a transfer_trx of 100 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "test-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Denied: "transaction value $12.00 exceeds per-TX USD limit of $10" |

> **Note:** The exact amounts depend on the current TRX price. Adjust amounts based on the price returned by `get_token_price`.

---

### D. USD Policy Limits — approval_required_above_usd

#### D1. Setup policy with USD approval threshold

```yaml
# policy.yaml
enabled: true

approval:
  method: telegram
  telegram:
    bot_token_env: GOTRON_MCP_TELEGRAM_BOT_TOKEN
    chat_id: <YOUR_CHAT_ID>
    authorized_users: [<YOUR_USER_ID>]

wallets:
  test-wallet:
    approval_required_above_usd: 5
    token_limits:
      TRX:
        per_tx_limit_units: 10000
        daily_limit_units: 50000
    whitelist:
      - "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"
```

#### D2. Transfer below USD approval threshold

```
Build a transfer_trx of 20 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "test-wallet"
```

If 20 TRX = $2.40 (below $5 threshold):

| Field | Expected |
|-------|----------|
| Result | Success — no approval needed |

#### D3. Transfer above USD approval threshold

```
Build a transfer_trx of 100 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "test-wallet"
```

If 100 TRX = $12 (above $5 threshold):

| Field | Expected |
|-------|----------|
| Result | Approval required (Telegram message or hint if no approver) |
| Message | "transaction value $12.00 exceeds USD approval threshold of $5" |

---

### E. No Price Provider (Graceful Degradation)

If CoinGecko is unreachable or the price service isn't configured, USD limits should be skipped (not enforced).

#### E1. USD limits with no internet

Disconnect network, then try a transfer that would exceed `per_tx_limit_usd`.

| Field | Expected |
|-------|----------|
| Result | Success — USD check skipped when price unavailable |
| Unit limits | Still enforced (don't depend on price service) |

---

### F. get_wallet_policy Shows USD Limits

```
Use gotron get_wallet_policy for wallet "test-wallet"
```

| Field | Expected |
|-------|----------|
| `per_tx_limit_usd` | Shows configured USD limit |
| `approval_required_above_usd` | Shows configured USD threshold |

---

## Summary Checklist

| # | Test | Category | Expected |
|---|------|----------|----------|
| A1 | TRX price | Price tool | Non-zero USD price |
| A2 | USDT price | Price tool | ~$1.00 |
| A3 | USDC price | Price tool | ~$1.00 |
| A4 | BTT price | Price tool | Small positive |
| A5 | Unknown token | Price tool | Error |
| A6 | Case insensitive | Price tool | Works |
| A7 | Empty token | Price tool | Error |
| B1 | Cache hit | Caching | Instant second call |
| B2 | Cache expiry | Caching | Fresh fetch after TTL |
| C2 | Below USD per-TX | USD limit | Allowed |
| C3 | Above USD per-TX | USD limit | Denied |
| D2 | Below USD approval | USD approval | Allowed |
| D3 | Above USD approval | USD approval | Approval required |
| E1 | No internet | Degradation | USD skipped, unit limits work |
| F1 | Policy shows USD | Inspection | USD fields visible |
