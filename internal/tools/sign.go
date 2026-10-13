package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/policy"
	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/client/transaction"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/fbsobreira/gotron-sdk/pkg/signer"
	trc20Std "github.com/fbsobreira/gotron-sdk/pkg/standards/trc20"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

const maxHexInputLen = 1 << 20 // 1 MB max hex string input

// RegisterSignTools registers sign_transaction, sign_and_broadcast, sign_and_confirm,
// and broadcast_transaction (local mode + wallet manager only).
// The policy engine is optional — pass nil to disable policy enforcement.
// When a policy engine is active, sign_transaction and broadcast_transaction are
// NOT registered to prevent policy bypass via the two-step sign+broadcast path.
func RegisterSignTools(s *server.MCPServer, pool *nodepool.Pool, wm *wallet.Manager, pe *policy.Engine) {
	// Policy inspection tool
	s.AddTool(
		mcp.NewTool("get_wallet_policy",
			mcp.WithDescription("Show the active policy for a wallet: spend limits, token limits, whitelist, and approval thresholds. Returns 'no policy' if the wallet is unrestricted."),
			mcp.WithString("wallet", mcp.Required(), mcp.Description("Wallet name")),
		),
		handleGetWalletPolicy(pe, wm),
	)

	// One-shot limit override — agent calls this when a limit denial is hit
	if pe != nil {
		s.AddTool(
			mcp.NewTool("request_limit_override",
				mcp.WithDescription("Request a one-time limit override via Telegram approval when a transaction exceeds policy limits. Use this after sign_and_broadcast returns a policy denial. The transaction is rebuilt and broadcast after human approval."),
				mcp.WithString("transaction_hex", mcp.Required(), mcp.Description("The unsigned transaction hex that was denied")),
				mcp.WithString("wallet", mcp.Required(), mcp.Description("Wallet name")),
				mcp.WithString("reason", mcp.Required(), mcp.Description("Why this transaction needs to exceed the limit")),
			),
			handleRequestLimitOverride(pool, wm, pe),
		)
	}

	// sign_and_broadcast and sign_and_confirm always available — they enforce policy
	s.AddTool(
		mcp.NewTool("sign_and_broadcast",
			mcp.WithDescription("Sign and broadcast a transaction in one step. Enforces wallet policy (spend limits, whitelist) when configured. If approval is required, the transaction is rebuilt fresh after approval — the final txid may differ from the original transaction."),
			mcp.WithString("transaction_hex", mcp.Required(), mcp.Description("Unsigned transaction hex")),
			mcp.WithString("wallet", mcp.Required(), mcp.Description("Wallet name or address")),
			mcp.WithString("reason", mcp.Description("Reason for this transaction — shown in approval messages")),
		),
		handleSignAndBroadcast(s, pool, wm, pe),
	)

	s.AddTool(
		mcp.NewTool("sign_and_confirm",
			mcp.WithDescription("Sign, broadcast, and wait for on-chain confirmation. Enforces wallet policy when configured. If approval is required, the transaction is rebuilt fresh after approval — the final txid may differ from the original transaction."),
			mcp.WithString("transaction_hex", mcp.Required(), mcp.Description("Unsigned transaction hex")),
			mcp.WithString("wallet", mcp.Required(), mcp.Description("Wallet name or address")),
			mcp.WithString("reason", mcp.Description("Reason for this transaction — shown in approval messages")),
		),
		handleSignAndConfirm(s, pool, wm, pe),
	)

	// sign_transaction and broadcast_transaction only available when no policy engine
	// is active — they bypass policy enforcement by design (separate sign + broadcast).
	if pe == nil {
		s.AddTool(
			mcp.NewTool("sign_transaction",
				mcp.WithDescription("Sign an unsigned transaction using a managed wallet. Returns signed transaction hex for broadcasting."),
				mcp.WithString("transaction_hex", mcp.Required(), mcp.Description("Unsigned transaction hex")),
				mcp.WithString("wallet", mcp.Required(), mcp.Description("Wallet name or address")),
			),
			handleSignTransaction(pool, wm),
		)

		s.AddTool(
			mcp.NewTool("broadcast_transaction",
				mcp.WithDescription("Broadcast a signed transaction to the TRON network. Post-quantum (TIP-899) signatures are checked against the node first, and their schemes and addresses are returned as pq_signers."),
				mcp.WithString("signed_transaction_hex", mcp.Required(), mcp.Description("Signed transaction hex")),
			),
			handleBroadcastTransaction(pool),
		)
	}
}

// parseAndValidateTx validates and decodes a transaction hex string into a core.Transaction.
func parseAndValidateTx(txHex string) (*core.Transaction, error) {
	if txHex == "" {
		return nil, fmt.Errorf("transaction_hex is required")
	}
	if len(txHex) > maxHexInputLen {
		return nil, fmt.Errorf("transaction_hex exceeds maximum length")
	}
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction_hex: %v", err)
	}
	var tx core.Transaction
	if err := proto.Unmarshal(txBytes, &tx); err != nil {
		return nil, fmt.Errorf("failed to parse transaction: %v", err)
	}
	return &tx, nil
}

// computeTxID returns the SHA-256 hash of the transaction's raw data.
func computeTxID(tx *core.Transaction) (string, error) {
	rawData, err := proto.Marshal(tx.RawData)
	if err != nil {
		return "", fmt.Errorf("failed to compute txid: %v", err)
	}
	txHash := sha256.Sum256(rawData)
	return hex.EncodeToString(txHash[:]), nil
}

func handleSignTransaction(pool *nodepool.Pool, wm *wallet.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		progress := newProgressReporter(ctx, req, 2)
		txHex := req.GetString("transaction_hex", "")
		walletName := req.GetString("wallet", "")

		if walletName == "" {
			return mcp.NewToolResultError("wallet is required"), nil
		}

		progress.Send(1, "Validating transaction...")
		tx, err := parseAndValidateTx(txHex)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Look up the wallet and run the post-quantum preflight before signing.
		walletInfo, local, err := walletPreflight(ctx, pool.Client(), wm, "sign_transaction", walletName, tx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		progress.Send(2, "Signing with wallet...")
		signedTx, err := wm.SignTransaction(walletName, tx)
		if err != nil {
			return mcp.NewToolResultError(signErrorMessage("sign_transaction", err)), nil
		}

		signedBytes, err := proto.Marshal(signedTx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to serialize signed transaction: %v", err)), nil
		}

		result := map[string]any{
			"signed_transaction_hex": hex.EncodeToString(signedBytes),
			"wallet":                 walletName,
			"signed_size_bytes":      len(signedBytes),
		}
		if local {
			result["key_type"] = string(walletInfo.KeyType)
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleSignAndBroadcast(mcpSrv *server.MCPServer, pool *nodepool.Pool, wm *wallet.Manager, pe *policy.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		progress := newProgressReporter(ctx, req, 5)
		txHex := req.GetString("transaction_hex", "")
		walletName := wm.ResolveWalletName(req.GetString("wallet", ""))

		if walletName == "" {
			return mcp.NewToolResultError("wallet is required"), nil
		}

		progress.Send(1, "Validating transaction...")
		tx, err := parseAndValidateTx(txHex)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Look up the wallet and run the post-quantum preflight before any
		// policy reservation, approval or confirmation prompt.
		walletInfo, local, err := walletPreflight(ctx, pool.Client(), wm, "sign_and_broadcast", walletName, tx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		isPQ := local && walletInfo.KeyType.IsPQ()

		// Decode contract data for policy enforcement
		var contractType string
		var contractData map[string]any
		var intent *policy.Intent

		decoded, decErr := transaction.DecodeContractData(tx)
		if decErr == nil {
			contractType = decoded.Type
			contractData = decoded.Fields
			var intentErr error
			intent, intentErr = policy.IntentFromContractData(walletName, decoded)
			if intentErr != nil && pe != nil {
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: unable to extract intent: %v — denied by policy", intentErr)), nil
			}
			if intent != nil {
				intent.ContractData = contractData
				intent.Reason = req.GetString("reason", "")
				if tx.RawData != nil && tx.RawData.Expiration > 0 {
					intent.TxExpiry = time.UnixMilli(tx.RawData.Expiration)
				}
			}
		} else if pe != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: unable to decode transaction (%v) — denied by policy", decErr)), nil
		}

		// Policy check
		reserved := false
		if intent != nil && pe != nil {
			progress.Send(2, "Checking policy...")
			result, pErr := pe.Check(ctx, intent)
			if pErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: policy check failed: %v", pErr)), nil
			}
			// Check may reserve daily spend — arm rollback immediately
			reserved = result.Allowed || result.ApprovalRequired
			if reserved {
				defer func() {
					if reserved {
						pe.ReleaseReserve(intent)
					}
				}()
			}
			if !result.Allowed {
				if result.ApprovalRequired {
					if !pe.HasApprover() {
						return mcp.NewToolResultJSON(map[string]any{
							"status": "approval_required",
							"reason": result.Reason,
							"hint":   "No approval backend configured. Configure telegram approval in policy.yaml to enable interactive approval.",
						})
					}
					progress.Send(3, "Requesting approval...")
					approved, aErr := pe.RequestApproval(ctx, intent)
					if aErr != nil {
						return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: %v", aErr)), nil
					}
					if !approved {
						return mcp.NewToolResultJSON(map[string]any{
							"status": "approval_rejected",
							"reason": result.Reason,
						})
					}
					// Approved — rebuild TX fresh (original may have expired during approval)
					progress.Send(3, "Rebuilding transaction...")
					freshTx, rebuildErr := rebuildTransaction(ctx, pool, intent, txPermissionID(tx))
					if rebuildErr != nil {
						return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: failed to rebuild TX after approval: %v", rebuildErr)), nil
					}
					// The account may have changed during approval. A failure
					// here returns after Check, so the deferred rollback
					// releases the reservation.
					if isPQ {
						if pqErr := pqOwnerCheck(ctx, pool.Client(), walletInfo, freshTx); pqErr != nil {
							return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: rebuilt transaction: %v", pqErr)), nil
						}
					}
					tx = freshTx
				} else {
					return mcp.NewToolResultJSON(map[string]any{
						"status": "policy_denied",
						"reason": result.Reason,
						"hint":   "Use request_limit_override tool with a reason to request a one-time limit override via Telegram approval.",
					})
				}
			}
		}

		// Elicitation confirmation (only when no policy engine)
		if pe == nil && decoded != nil {
			progress.Send(3, "Requesting confirmation...")
			if confirmErr := requestBroadcastConfirmation(ctx, mcpSrv, pool, decoded, walletName, walletInfo.KeyType, tx); confirmErr != nil {
				return mcp.NewToolResultJSON(map[string]any{
					"status": "cancelled",
					"reason": confirmErr.Error(),
				})
			}
		}

		progress.Send(4, "Signing with wallet...")
		signedTx, err := wm.SignTransaction(walletName, tx)
		if err != nil {
			return mcp.NewToolResultError(signErrorMessage("sign_and_broadcast", err)), nil
		}

		txid, err := computeTxID(signedTx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		progress.Send(5, "Broadcasting to network...")
		conn := pool.Client()
		if err := checkSignedPQSchemes(ctx, conn, signedTx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: %v", err)), nil
		}
		ret, err := conn.BroadcastCtx(ctx, signedTx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: %v", err)), nil
		}
		if !ret.Result {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: broadcast rejected: %s %s", ret.Code.String(), string(ret.Message))), nil
		}

		// Broadcast succeeded — keep the reservation
		reserved = false

		// Record audit + notify approver
		if intent != nil && pe != nil {
			if err := pe.RecordAudit(intent, txid); err != nil {
				log.Printf("ERROR: failed to record audit for wallet %q txid %s: %v", intent.WalletName, txid, err)
			}
			pe.NotifyBroadcast(ctx, txid, true)
		}

		result := map[string]any{
			"txid":    txid,
			"success": ret.Result,
			"code":    ret.Code.String(),
			"message": string(ret.Message),
		}
		if contractType != "" {
			result["contract_type"] = contractType
		}
		if contractData != nil {
			result["contract_data"] = contractData
		}
		if local {
			result["key_type"] = string(walletInfo.KeyType)
		}
		result["signed_size_bytes"] = proto.Size(signedTx)

		return mcp.NewToolResultJSON(result)
	}
}

func handleSignAndConfirm(mcpSrv *server.MCPServer, pool *nodepool.Pool, wm *wallet.Manager, pe *policy.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		const maxConfirmAttempts = 20
		// 5 setup steps + polling attempts + 1 confirmation step
		progress := newProgressReporter(ctx, req, 6+maxConfirmAttempts)

		txHex := req.GetString("transaction_hex", "")
		walletName := wm.ResolveWalletName(req.GetString("wallet", ""))

		if walletName == "" {
			return mcp.NewToolResultError("wallet is required"), nil
		}

		progress.Send(1, "Validating transaction...")
		tx, err := parseAndValidateTx(txHex)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Look up the wallet and run the post-quantum preflight before any
		// policy reservation, approval or confirmation prompt.
		walletInfo, local, err := walletPreflight(ctx, pool.Client(), wm, "sign_and_confirm", walletName, tx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		isPQ := local && walletInfo.KeyType.IsPQ()

		// Decode contract data for policy enforcement
		var intent *policy.Intent
		decoded, decErr := transaction.DecodeContractData(tx)
		if decErr == nil {
			var intentErr error
			intent, intentErr = policy.IntentFromContractData(walletName, decoded)
			if intentErr != nil && pe != nil {
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: unable to extract intent: %v — denied by policy", intentErr)), nil
			}
			if intent != nil {
				intent.ContractData = decoded.Fields
				intent.Reason = req.GetString("reason", "")
				if tx.RawData != nil && tx.RawData.Expiration > 0 {
					intent.TxExpiry = time.UnixMilli(tx.RawData.Expiration)
				}
			}
		} else if pe != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: unable to decode transaction (%v) — denied by policy", decErr)), nil
		}

		// Policy check
		reserved := false
		if intent != nil && pe != nil {
			progress.Send(2, "Checking policy...")
			result, pErr := pe.Check(ctx, intent)
			if pErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: policy check failed: %v", pErr)), nil
			}
			// Check may reserve daily spend — arm rollback immediately
			reserved = result.Allowed || result.ApprovalRequired
			if reserved {
				defer func() {
					if reserved {
						pe.ReleaseReserve(intent)
					}
				}()
			}
			if !result.Allowed {
				if result.ApprovalRequired {
					if !pe.HasApprover() {
						return mcp.NewToolResultJSON(map[string]any{
							"status": "approval_required",
							"reason": result.Reason,
							"hint":   "No approval backend configured. Configure telegram approval in policy.yaml to enable interactive approval.",
						})
					}
					progress.Send(3, "Requesting approval...")
					approved, aErr := pe.RequestApproval(ctx, intent)
					if aErr != nil {
						return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: %v", aErr)), nil
					}
					if !approved {
						return mcp.NewToolResultJSON(map[string]any{
							"status": "approval_rejected",
							"reason": result.Reason,
						})
					}
					// Approved — rebuild TX fresh
					progress.Send(3, "Rebuilding transaction...")
					freshTx, rebuildErr := rebuildTransaction(ctx, pool, intent, txPermissionID(tx))
					if rebuildErr != nil {
						return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: failed to rebuild TX after approval: %v", rebuildErr)), nil
					}
					// The account may have changed during approval. A failure
					// here returns after Check, so the deferred rollback
					// releases the reservation.
					if isPQ {
						if pqErr := pqOwnerCheck(ctx, pool.Client(), walletInfo, freshTx); pqErr != nil {
							return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: rebuilt transaction: %v", pqErr)), nil
						}
					}
					tx = freshTx
				} else {
					return mcp.NewToolResultJSON(map[string]any{
						"status": "policy_denied",
						"reason": result.Reason,
						"hint":   "Use request_limit_override tool with a reason to request a one-time limit override via Telegram approval.",
					})
				}
			}
		}

		// Elicitation confirmation (only when no policy engine)
		if pe == nil && decoded != nil {
			progress.Send(3, "Requesting confirmation...")
			if confirmErr := requestBroadcastConfirmation(ctx, mcpSrv, pool, decoded, walletName, walletInfo.KeyType, tx); confirmErr != nil {
				return mcp.NewToolResultJSON(map[string]any{
					"status": "cancelled",
					"reason": confirmErr.Error(),
				})
			}
		}

		progress.Send(4, "Signing with wallet...")
		signedTx, err := wm.SignTransaction(walletName, tx)
		if err != nil {
			return mcp.NewToolResultError(signErrorMessage("sign_and_confirm", err)), nil
		}

		txid, err := computeTxID(signedTx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		progress.Send(5, "Broadcasting to network...")
		conn := pool.Client()
		if err := checkSignedPQSchemes(ctx, conn, signedTx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: %v", err)), nil
		}
		ret, err := conn.BroadcastCtx(ctx, signedTx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: broadcast failed: %v", err)), nil
		}
		if !ret.Result {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: broadcast rejected: %s %s", ret.Code.String(), string(ret.Message))), nil
		}

		// Broadcast succeeded — keep the reservation
		reserved = false

		// Record audit
		if intent != nil && pe != nil {
			if err := pe.RecordAudit(intent, txid); err != nil {
				log.Printf("ERROR: failed to record audit for wallet %q txid %s: %v", intent.WalletName, txid, err)
			}
		}

		// Poll for confirmation
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for attempt := 0; attempt < maxConfirmAttempts; attempt++ {
			select {
			case <-ctx.Done():
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: context cancelled waiting for confirmation of %s", txid)), nil
			case <-ticker.C:
			}

			progress.Send(6+attempt, fmt.Sprintf("Waiting for confirmation (attempt %d/%d)...", attempt+1, maxConfirmAttempts))

			info, infoErr := conn.GetTransactionInfoByIDCtx(ctx, txid)
			if infoErr != nil {
				if strings.Contains(infoErr.Error(), "not found") {
					continue // not indexed yet
				}
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: %v", infoErr)), nil
			}
			if info != nil && info.BlockNumber > 0 {
				progress.Send(6+maxConfirmAttempts, fmt.Sprintf("Confirmed in block %d", info.BlockNumber))
				result := map[string]any{
					"txid":              txid,
					"success":           info.GetResult() != core.TransactionInfo_FAILED,
					"confirmed":         true,
					"block_number":      info.BlockNumber,
					"fee":               info.Fee,
					"energy_used":       info.Receipt.GetEnergyUsageTotal(),
					"bandwidth_used":    info.Receipt.GetNetUsage(),
					"signed_size_bytes": proto.Size(signedTx),
				}
				if local {
					result["key_type"] = string(walletInfo.KeyType)
				}
				return mcp.NewToolResultJSON(result)
			}
		}

		return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: confirmation timeout for %s after %d attempts", txid, maxConfirmAttempts)), nil
	}
}

func handleBroadcastTransaction(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		txHex := req.GetString("signed_transaction_hex", "")
		if txHex == "" {
			return mcp.NewToolResultError("signed_transaction_hex is required"), nil
		}

		tx, err := parseAndValidateTx(txHex)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("broadcast_transaction: %v", err)), nil
		}

		txid, err := computeTxID(tx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		pqSigs, err := signer.PQSignatures(tx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("broadcast_transaction: %v", err)), nil
		}
		if err := checkSignedPQSchemes(ctx, pool.Client(), tx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("broadcast_transaction: %v", err)), nil
		}
		ret, err := pool.Client().BroadcastCtx(ctx, tx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("broadcast_transaction: %v", err)), nil
		}
		pqSigners := make([]map[string]string, 0, len(pqSigs))
		for _, sig := range pqSigs {
			pqSigners = append(pqSigners, map[string]string{"scheme": sig.Scheme.String(), "address": sig.Address.String()})
		}

		return mcp.NewToolResultJSON(map[string]any{
			"transaction_id": txid,
			"success":        ret.Result,
			"code":           ret.Code.String(),
			"message":        string(ret.Message),
			"pq_signers":     pqSigners,
		})
	}
}

// rebuildTransaction creates a fresh transaction from the intent after approval.
// This ensures the TX has a valid on-chain expiry (TRON TXs expire ~60s).
// The node builds the transaction with permission 0, so permissionID (the
// original transaction's) is set on its contract. That changes the raw data,
// so the node's Txid is stale: callers compute the txid from the signed tx.
func rebuildTransaction(ctx context.Context, pool *nodepool.Pool, intent *policy.Intent, permissionID int32) (*core.Transaction, error) {
	if intent.Action != "TransferContract" {
		return nil, fmt.Errorf("cannot rebuild %s transactions — approval only supported for TransferContract", intent.Action)
	}
	if err := validateAddress(intent.FromAddr); err != nil {
		return nil, fmt.Errorf("invalid from address for rebuild: %w", err)
	}
	if err := validateAddress(intent.ToAddr); err != nil {
		return nil, fmt.Errorf("invalid to address for rebuild: %w", err)
	}

	ext, err := pool.Client().TransferCtx(ctx, intent.FromAddr, intent.ToAddr, intent.AmountSUN)
	if err != nil {
		return nil, fmt.Errorf("rebuilding transfer: %w", err)
	}
	contracts := ext.GetTransaction().GetRawData().GetContract()
	if len(contracts) != 1 || contracts[0] == nil {
		return nil, fmt.Errorf("rebuilding transfer: node returned %d contracts, want 1", len(contracts))
	}
	contracts[0].PermissionId = permissionID
	return ext.Transaction, nil
}

// formatTokenAmount formats a raw big.Int token amount with decimal point insertion.
// For example, formatTokenAmount(big.NewInt(1000000), 6) returns "1.000000".
func formatTokenAmount(raw *big.Int, decimals int) string {
	if raw == nil {
		return "0"
	}
	s := raw.String()
	if decimals <= 0 {
		return s
	}
	// Pad with leading zeros if needed
	for len(s) <= decimals {
		s = "0" + s
	}
	intPart := s[:len(s)-decimals]
	fracPart := s[len(s)-decimals:]
	// Trim trailing zeros from fractional part for cleaner display
	fracPart = strings.TrimRight(fracPart, "0")
	if fracPart == "" {
		return intPart
	}
	return intPart + "." + fracPart
}

// requestBroadcastConfirmation asks the user to confirm a transaction before broadcast
// via MCP elicitation. Returns nil to proceed, or an error to cancel.
// Gracefully skips if the client doesn't support elicitation or decoded is nil.
func requestBroadcastConfirmation(
	ctx context.Context,
	s *server.MCPServer,
	pool *nodepool.Pool,
	decoded *transaction.ContractData,
	walletName string,
	keyType wallet.KeyType,
	tx *core.Transaction,
) error {
	if decoded == nil || s == nil {
		return nil
	}

	// Build human-readable confirmation message from decoded contract fields.
	// For TRC20 transfers, decode ABI data and resolve decimals for human-readable amounts.
	var msg strings.Builder
	msg.WriteString("Confirm transaction before broadcast:\n\n")
	fmt.Fprintf(&msg, "Type: %s\n", decoded.Type)
	if from, ok := decoded.Fields["owner_address"]; ok {
		fmt.Fprintf(&msg, "From: %v (wallet: %q)\n", from, walletName)
	}
	if keyType.IsPQ() {
		fmt.Fprintf(&msg, "Signature: %s (post-quantum, TIP-899)\n", pqDisplayName(keyType))
		msg.WriteString("Note: post-quantum signatures make the transaction about 10-22x larger, so it uses more bandwidth\n")
		if scheme, ok := keyType.PQScheme(); ok && tx != nil {
			if bw, err := transaction.EstimateBandwidth(tx, scheme); err == nil {
				fmt.Fprintf(&msg, "Estimated bandwidth: about %d bytes\n", bw)
			}
		}
	}

	// Try to decode TRC20 transfer details from ABI data
	trc20Decoded := false
	if decoded.Type == "TriggerSmartContract" {
		if intent, err := policy.IntentFromContractData(walletName, decoded); err == nil && intent.TokenID != "TRX" {
			fmt.Fprintf(&msg, "To: %s\n", intent.ToAddr)
			fmt.Fprintf(&msg, "Token contract: %s\n", intent.TokenID)
			// Try to resolve token decimals and symbol for human-readable display
			if pool != nil {
				token := trc20Std.New(pool.Client(), intent.TokenID)
				if symbol, sErr := token.Symbol(ctx); sErr == nil {
					if decimals, dErr := token.Decimals(ctx); dErr == nil && decimals > 0 && intent.RawTokenAmt != nil {
						// Use big.Int string with decimal point insertion for exact precision
						fmt.Fprintf(&msg, "Amount: %s %s\n", formatTokenAmount(intent.RawTokenAmt, int(decimals)), symbol)
					} else if intent.RawTokenAmt != nil {
						fmt.Fprintf(&msg, "Amount: %s raw units (%s)\n", intent.RawTokenAmt.String(), symbol)
					} else {
						fmt.Fprintf(&msg, "Amount: %.0f raw units (%s)\n", intent.TokenAmount, symbol)
					}
				} else if intent.RawTokenAmt != nil {
					fmt.Fprintf(&msg, "Amount: %s raw units\n", intent.RawTokenAmt.String())
				} else {
					fmt.Fprintf(&msg, "Amount: %.0f raw units\n", intent.TokenAmount)
				}
			} else if intent.RawTokenAmt != nil {
				fmt.Fprintf(&msg, "Amount: %s raw units\n", intent.RawTokenAmt.String())
			} else {
				fmt.Fprintf(&msg, "Amount: %.0f raw units\n", intent.TokenAmount)
			}
			trc20Decoded = true
		}
	}

	if !trc20Decoded {
		if to, ok := decoded.Fields["to_address"]; ok {
			fmt.Fprintf(&msg, "To: %v\n", to)
		}
		if contract, ok := decoded.Fields["contract_address"]; ok {
			fmt.Fprintf(&msg, "Contract: %v\n", contract)
		}
		if amount, ok := decoded.Fields["amount"]; ok {
			switch v := amount.(type) {
			case string:
				// TransferContract: SDK already converts to human-readable TRX string
				fmt.Fprintf(&msg, "Amount: %s TRX\n", v)
			case int64:
				// TransferAssetContract (TRC10): raw units
				fmt.Fprintf(&msg, "Amount: %d (raw units)\n", v)
			case float64:
				fmt.Fprintf(&msg, "Amount: %g (raw units)\n", v)
			default:
				fmt.Fprintf(&msg, "Amount: %v\n", amount)
			}
		}
		if callValue, ok := decoded.Fields["call_value"]; ok {
			// SDK converts call_value via sunToTRX() — returns string
			if cv, isStr := callValue.(string); isStr && cv != "" && cv != "0.000000" {
				fmt.Fprintf(&msg, "Call Value: %s TRX\n", cv)
			}
		}
	}

	elicitReq := mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message: msg.String(),
			RequestedSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}

	result, err := s.RequestElicitation(ctx, elicitReq)
	if err != nil {
		if errors.Is(err, server.ErrElicitationNotSupported) || errors.Is(err, server.ErrNoActiveSession) {
			return nil
		}
		return fmt.Errorf("transaction broadcast cancelled: %w", err)
	}

	if result.Action == mcp.ElicitationResponseActionAccept {
		return nil
	}

	return fmt.Errorf("transaction broadcast cancelled by user")
}

func handleRequestLimitOverride(pool *nodepool.Pool, wm *wallet.Manager, pe *policy.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		txHex := req.GetString("transaction_hex", "")
		walletName := wm.ResolveWalletName(req.GetString("wallet", ""))
		reason := req.GetString("reason", "")

		if walletName == "" {
			return mcp.NewToolResultError("wallet is required"), nil
		}
		if reason == "" {
			return mcp.NewToolResultError("reason is required for limit override requests"), nil
		}

		tx, err := parseAndValidateTx(txHex)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Look up the wallet and run the post-quantum preflight before asking
		// the approver.
		walletInfo, local, err := walletPreflight(ctx, pool.Client(), wm, "request_limit_override", walletName, tx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		isPQ := local && walletInfo.KeyType.IsPQ()

		// Decode intent
		decoded, decErr := transaction.DecodeContractData(tx)
		if decErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("request_limit_override: unable to decode transaction: %v", decErr)), nil
		}
		intent, intentErr := policy.IntentFromContractData(walletName, decoded)
		if intentErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("request_limit_override: %v", intentErr)), nil
		}
		intent.ContractData = decoded.Fields
		intent.Reason = reason
		intent.IsOverride = true

		// Still enforce whitelist — override only bypasses amount limits
		wp := pe.GetPolicy(walletName)
		if wp != nil && len(wp.Whitelist) > 0 && intent.ToAddr != "" {
			allowed := false
			for _, addr := range wp.Whitelist {
				if addr == intent.ToAddr {
					allowed = true
					break
				}
			}
			if !allowed {
				return mcp.NewToolResultError(fmt.Sprintf("request_limit_override: destination %s is not in the whitelist", intent.ToAddr)), nil
			}
		}

		// Request approval via Telegram
		if !pe.HasApprover() {
			return mcp.NewToolResultError("request_limit_override: no approval backend configured — configure telegram approval in policy.yaml"), nil
		}
		approved, aErr := pe.RequestApproval(ctx, intent)
		if aErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("request_limit_override: %v", aErr)), nil
		}
		if !approved {
			return mcp.NewToolResultJSON(map[string]any{
				"status": "override_rejected",
				"reason": "limit override was rejected by approver",
			})
		}

		// Approved — rebuild TX fresh, sign, broadcast
		freshTx, rebuildErr := rebuildTransaction(ctx, pool, intent, txPermissionID(tx))
		if rebuildErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("request_limit_override: failed to rebuild TX: %v", rebuildErr)), nil
		}
		if isPQ {
			if pqErr := pqOwnerCheck(ctx, pool.Client(), walletInfo, freshTx); pqErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("request_limit_override: rebuilt transaction: %v", pqErr)), nil
			}
		}

		signedTx, err := wm.SignTransaction(walletName, freshTx)
		if err != nil {
			return mcp.NewToolResultError(signErrorMessage("request_limit_override", err)), nil
		}

		txid, err := computeTxID(signedTx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := checkSignedPQSchemes(ctx, pool.Client(), signedTx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("request_limit_override: %v", err)), nil
		}
		ret, err := pool.Client().BroadcastCtx(ctx, signedTx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("request_limit_override: broadcast failed: %v", err)), nil
		}
		if !ret.Result {
			return mcp.NewToolResultError(fmt.Sprintf("request_limit_override: broadcast rejected: %s %s", ret.Code.String(), string(ret.Message))), nil
		}

		// Record override spend (tracks daily budget without enforcing limits)
		pe.RecordOverrideSpend(intent)
		if err := pe.RecordAudit(intent, txid); err != nil {
			log.Printf("ERROR: failed to record override audit: %v", err)
		}
		pe.NotifyBroadcast(ctx, txid, true)

		result := map[string]any{
			"status":            "override_approved",
			"txid":              txid,
			"success":           true,
			"code":              ret.Code.String(),
			"message":           string(ret.Message),
			"override":          true,
			"signed_size_bytes": proto.Size(signedTx),
		}
		if local {
			result["key_type"] = string(walletInfo.KeyType)
		}
		return mcp.NewToolResultJSON(result)
	}
}

func handleGetWalletPolicy(pe *policy.Engine, wm *wallet.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		walletName := req.GetString("wallet", "")
		if walletName == "" {
			return mcp.NewToolResultError("wallet is required"), nil
		}
		if wm != nil {
			walletName = wm.ResolveWalletName(walletName)
		}

		if pe == nil {
			return mcp.NewToolResultJSON(map[string]any{
				"wallet":         walletName,
				"policy_enabled": false,
				"message":        "No policy engine configured — all wallets are unrestricted",
			})
		}

		wp := pe.GetPolicy(walletName)
		if wp == nil {
			return mcp.NewToolResultJSON(map[string]any{
				"wallet":         walletName,
				"policy_enabled": true,
				"has_policy":     false,
				"message":        "No policy configured for this wallet — unrestricted",
			})
		}

		result := map[string]any{
			"wallet":              walletName,
			"policy_enabled":      true,
			"has_policy":          true,
			"approver_configured": pe.HasApprover(),
		}

		// TRX limits
		if wp.PerTxLimitTRX > 0 {
			result["per_tx_limit_trx"] = wp.PerTxLimitTRX
		}
		if wp.DailyLimitTRX > 0 {
			result["daily_limit_trx"] = wp.DailyLimitTRX
		}
		if wp.ApprovalRequiredAboveTRX > 0 {
			result["approval_required_above_trx"] = wp.ApprovalRequiredAboveTRX
		}

		// USD limits
		if wp.PerTxLimitUSD > 0 {
			result["per_tx_limit_usd"] = wp.PerTxLimitUSD
		}
		if wp.DailyLimitUSD > 0 {
			result["daily_limit_usd"] = wp.DailyLimitUSD
		}
		if wp.ApprovalRequiredAboveUSD > 0 {
			result["approval_required_above_usd"] = wp.ApprovalRequiredAboveUSD
		}

		// Token limits
		if len(wp.TokenLimits) > 0 {
			tokenLimits := make(map[string]any, len(wp.TokenLimits))
			for token, tl := range wp.TokenLimits {
				entry := map[string]any{}
				if tl.PerTxLimitUnits > 0 {
					entry["per_tx_limit_units"] = tl.PerTxLimitUnits
				}
				if tl.DailyLimitUnits > 0 {
					entry["daily_limit_units"] = tl.DailyLimitUnits
				}
				if tl.PerTxLimitUSD > 0 {
					entry["per_tx_limit_usd"] = tl.PerTxLimitUSD
				}
				if tl.DailyLimitUSD > 0 {
					entry["daily_limit_usd"] = tl.DailyLimitUSD
				}
				if tl.ApprovalRequiredAboveUnits > 0 {
					entry["approval_required_above_units"] = tl.ApprovalRequiredAboveUnits
				}
				tokenLimits[token] = entry
			}
			result["token_limits"] = tokenLimits
		}

		// Whitelist
		if len(wp.Whitelist) > 0 {
			result["whitelist"] = wp.Whitelist
		}

		// Daily spend remaining
		remaining := pe.GetRemainingBudget(walletName)
		if len(remaining) > 0 {
			result["remaining_today"] = remaining
		}

		return mcp.NewToolResultJSON(result)
	}
}
