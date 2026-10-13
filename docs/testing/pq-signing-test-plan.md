# Post-Quantum Signing Test Plan

Manual test procedure for TIP-899 post-quantum wallets and signing (GOT-96).
Run these tests with an AI agent connected to the MCP server via Claude Code or Claude Desktop.

TIP-899 is a draft that is live only on the Nile testnet. These tests cover the
**self-owner path**: a fresh PQ address spends its own TRX through the default
owner permission. They do not cover an existing ECDSA account that has a PQ key
added to an active permission (`permission_id` 2 or higher). That path stays
unexercised on chain until GOT-98, or until a tronctl migration is done by hand.

## Prerequisites

- GoTRON MCP built from the `feat/GOT-96-pq-signing` branch in **both** flavours:
  - Default build: `make build`. ML-DSA-44 only.
  - FN-DSA build, which needs liboqs (`brew install liboqs`, or the GoTRON SDK `scripts/build-liboqs.sh`):
    ```bash
    CGO_ENABLED=1 go build -tags falcon -o bin/gotron-mcp-pq ./cmd/gotron-mcp
    go version -m bin/gotron-mcp-pq | grep -E 'tags|CGO_ENABLED'   # expect -tags=falcon, CGO_ENABLED=1
    ```
- A Nile wallet with ~200 TRX to fund the PQ address (https://nileex.io/join/getJoinPage).
- A **throwaway** keystore directory and passphrase. Never point these tests at a real keystore.

## Setup

### 1. Register the FN-DSA build in local (stdio) mode

The wallet and sign tools are registered only in local mode with a keystore.

```bash
export PQ_KS=$(mktemp -d)/wallets
claude mcp add gotron-pq -- "$PWD/bin/gotron-mcp-pq" \
  --network nile \
  --keystore-dir "$PQ_KS" \
  --keystore-pass "pq-smoke-test-only"
```

No policy config is needed. Without one, every sign shows the elicitation confirmation.

### 2. Check the capabilities

```
Use gotron-pq get_pq_capabilities
```

Expected:
- `node.fn_dsa_512: true`. Nile has FN-DSA-512 on; `ml_dsa_44` is currently off.
- `build.fn_dsa_512: true`, `build.ml_dsa_44: true`
- `can_sign.fn_dsa_512: true`
- no `hint`

If `node.fn_dsa_512` is false, governance has switched it off. Stop and record that.

## Tests

### T1. Create an FN-DSA wallet

```
Use gotron-pq create_wallet with name "pq-fn" and key_type "fn_dsa_512"
```

Expected: the result has `key_type: "fn_dsa_512"`, a `T...` address and three `notes`
(Nile only, fund first, larger transactions). A keystore file exists at
`$PQ_KS/pq-keys/pq-fn/UTC--*.json` with mode `0600`, and its `scheme` field is `FN_DSA_512`.

### T2. List the wallets

```
Use gotron-pq list_wallets
```

Expected: `pq-fn` is listed with `key_type: "fn_dsa_512"`.

### T3. Sign before funding (fails before any prompt)

```
Use gotron-pq transfer_trx from "pq-fn" to <your funded Nile address> amount 1
Use gotron-pq sign_and_broadcast with that transaction_hex and wallet "pq-fn"
```

Expected: the build step may already fail because the account does not exist. If it
does not, `sign_and_broadcast` must fail with "owner account T... is not on chain yet;
a new address must receive TRX before it can send", and **no confirmation prompt is shown**.

### T4. Fund the PQ address

Send ~100 Nile TRX from your funded wallet to the `pq-fn` address, using tronctl,
TronLink or the existing ECDSA MCP wallet. Wait for confirmation.

### T5. Transfer from the PQ wallet

```
Use gotron-pq transfer_trx from "pq-fn" to <your funded Nile address> amount 1
Use gotron-pq sign_and_broadcast with that transaction_hex and wallet "pq-fn"
```

Expected:
- The confirmation prompt includes `Signature: FN-DSA-512 (post-quantum, TIP-899)`, the bandwidth note and an `Estimated bandwidth: about N bytes` line (about 1,800 for a transfer). Accept it.
- The result has `success: true`, `key_type: "fn_dsa_512"` and `signed_size_bytes` around 1,700. A plain ECDSA transfer is about 200 bytes.

```
Use gotron-pq get_transaction with the txid
```

Expected: `result: SUCCESS`. A new account has too little free bandwidth for a
transaction this size, so the fee burns about 1.76 TRX (`receipt.net_fee`).

`get_transaction` does not return signatures, so check them on the node:

```bash
curl -s -X POST https://nile.trongrid.io/wallet/gettransactionbyid \
  -H 'content-type: application/json' -d '{"value":"<txid>"}'
```

Expected: one `pq_auth_sig` entry with scheme `FN_DSA_512` and **no** `signature` field.

### T6. sign_and_confirm

Repeat T5 with `sign_and_confirm`. Expected: `confirmed: true` with block number and
fee or bandwidth details, and `key_type: "fn_dsa_512"`.

### T6b. Sign, then broadcast separately

Build another transfer, then:

```
Use gotron-pq sign_transaction with that transaction_hex and wallet "pq-fn"
Use gotron-pq broadcast_transaction with the signed_transaction_hex
```

Expected: `success: true` and
`pq_signers: [{"scheme": "FN_DSA_512", "address": "<pq-fn address>"}]`.

### T7. Default build: FN-DSA is reported, not crashed

Register `bin/gotron-mcp` (the default build) against a **second** throwaway keystore
the same way, then:

```
Use gotron get_pq_capabilities
Use gotron create_wallet with name "pq-fn2" and key_type "fn_dsa_512"
```

Expected:
- `build.fn_dsa_512: false`, plus the hint "This node allows FN-DSA-512, but this build cannot sign it; ...".
- `create_wallet` fails with "this build cannot create FN-DSA-512 keys; it needs the gotron-mcp-pq build, or build locally ...". No `pq-keys/pq-fn2` folder is created.

### T8. ML-DSA-44 is off on Nile

```
Use gotron create_wallet with name "pq-ml" and key_type "ml_dsa_44"
```

Fund it with a little TRX, then build a transfer from it and call `sign_and_broadcast`.
Expected while Nile has ML-DSA-44 off: the call fails with "node does not allow ML-DSA-44
signatures ...", and there is no confirmation prompt and no broadcast.

### T9. Mainnet reports no PQ support

Register either build with `--network mainnet` and **no keystore**, then:

```
Use gotron get_pq_capabilities
```

Expected: every `node` flag is false, with the hint "This node has no TIP-899
post-quantum support (mainnet today). Use a Nile node." `create_wallet` and the sign
tools are not registered without a keystore.

## Cleanup

```bash
claude mcp remove gotron-pq
claude mcp remove gotron   # if registered for T7-T9
rm -rf "$(dirname "$PQ_KS")"
```
