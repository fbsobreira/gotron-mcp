# Telegram Approval System Test Plan

Manual test procedure for the Telegram approval backend (#75).
Run these tests with an AI agent connected to the MCP server.

## Prerequisites

- GoTRON MCP built from `feature/75-telegram-approval` branch
- A Telegram bot created via [@BotFather](https://t.me/BotFather)
- Your Telegram user ID (get it from [@userinfobot](https://t.me/userinfobot))
- A Telegram chat/group where the bot is a member
- The chat ID (send a message in the chat, then visit `https://api.telegram.org/bot<TOKEN>/getUpdates` to find `chat.id`)
- A nile testnet wallet with TRX (use https://nileex.io/join/getJoinPage)
- Network set to `nile`

## Setup

### 1. Create the Telegram bot

1. Open Telegram, search for `@BotFather`
2. Send `/newbot`, follow prompts
3. Save the bot token (e.g., `7123456789:AAH...`)
4. Add the bot to your approval chat/group

### 2. Get your user ID and chat ID

1. Message `@userinfobot` — it replies with your user ID
2. Send a message in your approval chat
3. Visit `https://api.telegram.org/bot<TOKEN>/getUpdates`
4. Find `"chat":{"id": <number>}` — that's your chat ID

### 3. Configure policy.yaml

Save to `~/.gotron-mcp/policy.yaml` (or your configured path):

```yaml
enabled: true

approval:
  method: telegram
  telegram:
    bot_token_env: GOTRON_MCP_TELEGRAM_BOT_TOKEN
    chat_id: <YOUR_CHAT_ID>
    authorized_users: [<YOUR_USER_ID>]
    timeout_seconds: 120

wallets:
  test-wallet:
    token_limits:
      TRX:
        per_tx_limit_units: 100
        daily_limit_units: 500
        approval_required_above_units: 10
    whitelist:
      - "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"
```

### 4. Set environment variable

```bash
export GOTRON_MCP_TELEGRAM_BOT_TOKEN="<YOUR_BOT_TOKEN>"
```

### 5. Build and configure the MCP server

```bash
make build
```

The Telegram approval system requires **local (stdio) mode** with keystore — sign tools are not available in HTTP hosted mode.

Configure Claude Code `.mcp.json`:

```json
{
  "mcpServers": {
    "gotron": {
      "type": "stdio",
      "command": "/path/to/gotron-mcp/bin/gotron-mcp",
      "args": [
        "--network", "nile",
        "--keystore-dir", "/path/to/.gotron-mcp/wallets",
        "--policy-config", "/path/to/policy.yaml"
      ],
      "env": {
        "GOTRON_MCP_KEYSTORE_PASSPHRASE": "<YOUR_PASSPHRASE>",
        "GOTRON_MCP_TELEGRAM_BOT_TOKEN": "<YOUR_BOT_TOKEN>"
      }
    }
  }
}
```

Since stdio mode doesn't show logs directly, verify the setup by asking your agent:

```
Use gotron get_wallet_policy for wallet "test-wallet"
```

Expected: `policy_enabled: true`, `has_policy: true` with your configured limits.

### 6. Create test wallet

```
Use gotron create_wallet with name "test-wallet"
```

Fund the wallet with ~100 nile TRX.

---

## Test Cases

### A. Approval Flow — Happy Path

#### A1. Transfer below approval threshold (no Telegram message)

```
Build a transfer_trx of 5 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "test-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Success — transaction broadcasts immediately |
| Telegram | No message sent (below threshold) |
| Response | Contains `txid`, `success: true` |

#### A2. Transfer above approval threshold — approve

```
Build a transfer_trx of 20 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "test-wallet"
```

| Step | Expected |
|------|----------|
| 1. Agent calls sign_and_broadcast | Tool shows "Requesting approval..." |
| 2. Telegram message arrives | Shows approval request with wallet, type, from, to, amount, expiry countdown |
| 3. Tap "✅ Approve" on Telegram | Toast shows "✅ Transaction approved!" |
| 4. Agent receives result | Transaction broadcasts, returns `txid` |
| 5. Telegram follow-up | New message shows "✅ Transaction Broadcast Successful" with TxID and TronScan link |

**Verify in the Telegram message:**
- 🔔 Transaction Approval Request header with separator line
- 💼 Wallet name
- 📋 Contract type
- 📤 From address
- 📥 To address
- 💰 Amount
- ⏰ Expiry countdown (e.g., "Expires in 1m50s")
- ⚠️ Warning footer about signing upon approval
- Two inline buttons: ✅ Approve | ❌ Reject

**Verify after approval:**
- Original message updated to show "✅ APPROVED by @yourusername at HH:MM:SS UTC"
- Buttons removed from the original message
- Follow-up message with TxID

#### A3. Transfer above approval threshold — reject

```
Build a transfer_trx of 20 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "test-wallet"
```

| Step | Expected |
|------|----------|
| 1. Telegram message arrives | Same approval request as A2 |
| 2. Tap "❌ Reject" on Telegram | Toast shows "❌ Transaction rejected." |
| 3. Agent receives result | Returns `{"status": "approval_rejected", "reason": "..."}` |
| 4. Original message updated | Shows "❌ REJECTED by @yourusername" |
| 5. No follow-up broadcast message | Transaction was not signed or broadcast |

---

### B. Timeout Behavior

#### B1. Approval expires — no response from user

```
Build a transfer_trx of 20 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "test-wallet"
```

**Do NOT tap any button.** Wait for the timeout (120 seconds per config, or the TX expiry — whichever is sooner).

| Field | Expected |
|-------|----------|
| Result | `{"status": "approval_rejected", "reason": "...timed out..."}` |
| Telegram | Original message updated to show "⏰ EXPIRED — approval timed out" |
| Transaction | Not signed or broadcast |

> **Note:** TRON transactions expire ~60 seconds after creation. The approval timeout respects the TX expiry minus a 10-second buffer for signing. If the TX expires before the configured timeout, the approval will expire earlier.

#### B2. Transaction already expired before approval request

```
Build a transfer_trx, wait 60+ seconds, then try sign_and_broadcast
```

| Field | Expected |
|-------|----------|
| Result | Error: "transaction expires too soon for approval" |
| Telegram | No message sent |

---

### C. Authorization

#### C1. Unauthorized user tries to approve

1. Add another person to the approval chat (not in `authorized_users`)
2. Trigger an approval request (A2)
3. Have the other person tap "✅ Approve"

| Field | Expected |
|-------|----------|
| Toast | "⛔ You are not authorized to approve transactions." |
| Transaction | Still pending — not approved |
| Your button | Still works — you can still approve/reject |

#### C2. Authorized user in wrong chat

1. If the bot is in multiple chats, trigger an approval from the wrong chat

| Field | Expected |
|-------|----------|
| Result | Callback silently ignored — approval stays pending |

---

### D. Policy + Approval Integration

#### D1. Whitelist still enforced before approval

```
Build a transfer_trx of 20 TRX from <test-wallet-address> to TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t, then sign_and_broadcast with wallet "test-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Error: "not in the whitelist" |
| Telegram | No message — whitelist check runs BEFORE approval |

#### D2. Per-TX limit still enforced before approval

```
Build a transfer_trx of 200 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "test-wallet"
```

| Field | Expected |
|-------|----------|
| Result | Error: "exceeds per-tx token limit" |
| Telegram | No message — per-TX check runs BEFORE approval |

#### D3. Daily limit reserved BEFORE approval prompt

1. Send 5 TRX (below threshold, allowed — consumes daily budget)
2. Send 20 TRX (above threshold — triggers approval)
3. Check `get_wallet_policy` for remaining budget

| Field | Expected |
|-------|----------|
| Remaining budget | Should show 20 TRX already reserved (even before you tap Approve) |

After approving:
- Budget stays reserved (spend committed)

After rejecting:
- Budget should be restored (spend released)

#### D4. Daily limit hit blocks approval-required TX

1. Send enough small TXs (below threshold) to approach the 500 TRX daily limit
2. Send 20 TRX (above threshold)

| Field | Expected |
|-------|----------|
| Result | Error: "would exceed daily token limit" |
| Telegram | No message — daily limit check denies BEFORE approval |

---

### E. sign_and_confirm with Approval

#### E1. sign_and_confirm with approval — approve

```
Build a transfer_trx of 20 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_confirm with wallet "test-wallet"
```

Same flow as A2 but with confirmation polling after broadcast:

| Field | Expected |
|-------|----------|
| Telegram | Approval request → Approve → Broadcast notification |
| Response | Contains `confirmed: true`, `block_number` |

---

### F. get_wallet_policy Shows Approval Config

```
Use gotron get_wallet_policy for wallet "test-wallet"
```

| Field | Expected |
|-------|----------|
| `policy_enabled` | `true` |
| `has_policy` | `true` |
| `token_limits.TRX.approval_required_above_units` | `10` |
| `remaining_today` | Shows spent/remaining TRX budget |

---

### G. No Approval Backend Configured

Remove the `approval` section from policy.yaml, restart.

```
Build a transfer_trx of 20 TRX from <test-wallet-address> to TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF, then sign_and_broadcast with wallet "test-wallet"
```

| Field | Expected |
|-------|----------|
| Result | `{"status": "approval_rejected", "reason": "...approval required..."}` |
| Telegram | No message — no approver configured means rejection |

---

## Summary Checklist

| # | Test | Category | Expected |
|---|------|----------|----------|
| A1 | Below threshold | Flow | No Telegram, broadcasts immediately |
| A2 | Above threshold, approve | Flow | Telegram message → approve → broadcast → notification |
| A3 | Above threshold, reject | Flow | Telegram message → reject → no broadcast |
| B1 | Timeout | Timeout | Expires, message updated |
| B2 | TX already expired | Timeout | Immediate error, no Telegram |
| C1 | Unauthorized user | Auth | Toast error, TX still pending |
| C2 | Wrong chat | Auth | Callback ignored |
| D1 | Whitelist blocks before approval | Policy | Denied, no Telegram |
| D2 | Per-TX limit blocks before approval | Policy | Denied, no Telegram |
| D3 | Daily limit reserved before approval | Policy | Budget reserved during approval prompt |
| D4 | Daily limit blocks approval TX | Policy | Denied, no Telegram |
| E1 | sign_and_confirm + approve | Confirm | Full flow with confirmation |
| F1 | get_wallet_policy shows threshold | Inspection | approval_required_above_units visible |
| G1 | No approver configured | Fallback | Static rejection |
