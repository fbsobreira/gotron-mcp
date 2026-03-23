package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/policy"
	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/client/transaction"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
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
		handleGetWalletPolicy(pe),
	)

	// sign_and_broadcast and sign_and_confirm always available — they enforce policy
	s.AddTool(
		mcp.NewTool("sign_and_broadcast",
			mcp.WithDescription("Sign and broadcast a transaction in one step. Enforces wallet policy (spend limits, whitelist) when configured."),
			mcp.WithString("transaction_hex", mcp.Required(), mcp.Description("Unsigned transaction hex")),
			mcp.WithString("wallet", mcp.Required(), mcp.Description("Wallet name or address")),
		),
		handleSignAndBroadcast(pool, wm, pe),
	)

	s.AddTool(
		mcp.NewTool("sign_and_confirm",
			mcp.WithDescription("Sign, broadcast, and wait for on-chain confirmation. Enforces wallet policy when configured."),
			mcp.WithString("transaction_hex", mcp.Required(), mcp.Description("Unsigned transaction hex")),
			mcp.WithString("wallet", mcp.Required(), mcp.Description("Wallet name or address")),
		),
		handleSignAndConfirm(pool, wm, pe),
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
			handleSignTransaction(wm),
		)

		s.AddTool(
			mcp.NewTool("broadcast_transaction",
				mcp.WithDescription("Broadcast a signed transaction to the TRON network"),
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

func handleSignTransaction(wm *wallet.Manager) server.ToolHandlerFunc {
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

		progress.Send(2, "Signing with wallet...")
		s, err := wm.GetSigner(walletName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_transaction: %v", err)), nil
		}

		signedTx, err := s.Sign(tx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to sign transaction: %v", err)), nil
		}

		signedBytes, err := proto.Marshal(signedTx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to serialize signed transaction: %v", err)), nil
		}

		result := map[string]any{
			"signed_transaction_hex": hex.EncodeToString(signedBytes),
			"wallet":                 walletName,
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleSignAndBroadcast(pool *nodepool.Pool, wm *wallet.Manager, pe *policy.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		progress := newProgressReporter(ctx, req, 4)
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
		} else if pe != nil {
			// Fail closed: unknown TX type with active policy = deny
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: unable to decode transaction (%v) — denied by policy", decErr)), nil
		}

		// Policy check
		reserved := false
		if intent != nil && pe != nil {
			progress.Send(2, "Checking policy...")
			result, pErr := pe.Check(intent)
			if pErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: policy check failed: %v", pErr)), nil
			}
			if !result.Allowed {
				if result.ApprovalRequired {
					return mcp.NewToolResultJSON(map[string]any{
						"status": "approval_required",
						"reason": result.Reason,
					})
				}
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: policy denied: %s", result.Reason)), nil
			}
			reserved = true
			defer func() {
				if reserved {
					pe.ReleaseReserve(intent)
				}
			}()
		}

		progress.Send(3, "Signing with wallet...")
		s, err := wm.GetSigner(walletName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: %v", err)), nil
		}

		signedTx, err := s.Sign(tx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to sign transaction: %v", err)), nil
		}

		txid, err := computeTxID(signedTx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		progress.Send(4, "Broadcasting to network...")
		conn := pool.Client()
		ret, err := conn.BroadcastCtx(ctx, signedTx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: %v", err)), nil
		}
		if !ret.Result {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: broadcast rejected: %s %s", ret.Code.String(), string(ret.Message))), nil
		}

		// Broadcast succeeded — keep the reservation
		reserved = false

		// Record audit
		if intent != nil && pe != nil {
			if err := pe.RecordAudit(intent, txid); err != nil {
				log.Printf("ERROR: failed to record audit for wallet %q txid %s: %v", intent.WalletName, txid, err)
			}
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

		return mcp.NewToolResultJSON(result)
	}
}

func handleSignAndConfirm(pool *nodepool.Pool, wm *wallet.Manager, pe *policy.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		const maxConfirmAttempts = 20
		// 4 setup steps + polling attempts + 1 confirmation step
		progress := newProgressReporter(ctx, req, 5+maxConfirmAttempts)

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

		// Decode contract data for policy enforcement
		var intent *policy.Intent
		decoded, decErr := transaction.DecodeContractData(tx)
		if decErr == nil {
			var intentErr error
			intent, intentErr = policy.IntentFromContractData(walletName, decoded)
			if intentErr != nil && pe != nil {
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: unable to extract intent: %v — denied by policy", intentErr)), nil
			}
		} else if pe != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: unable to decode transaction (%v) — denied by policy", decErr)), nil
		}

		// Policy check
		reserved := false
		if intent != nil && pe != nil {
			progress.Send(2, "Checking policy...")
			result, pErr := pe.Check(intent)
			if pErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: policy check failed: %v", pErr)), nil
			}
			if !result.Allowed {
				if result.ApprovalRequired {
					return mcp.NewToolResultJSON(map[string]any{
						"status": "approval_required",
						"reason": result.Reason,
					})
				}
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: policy denied: %s", result.Reason)), nil
			}
			reserved = true
			defer func() {
				if reserved {
					pe.ReleaseReserve(intent)
				}
			}()
		}

		progress.Send(3, "Signing with wallet...")
		s, err := wm.GetSigner(walletName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: %v", err)), nil
		}

		signedTx, err := s.Sign(tx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to sign transaction: %v", err)), nil
		}

		txid, err := computeTxID(signedTx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		progress.Send(4, "Broadcasting to network...")
		conn := pool.Client()
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

			progress.Send(5+attempt, fmt.Sprintf("Waiting for confirmation (attempt %d/%d)...", attempt+1, maxConfirmAttempts))

			info, infoErr := conn.GetTransactionInfoByIDCtx(ctx, txid)
			if infoErr != nil {
				if strings.Contains(infoErr.Error(), "not found") {
					continue // not indexed yet
				}
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: %v", infoErr)), nil
			}
			if info != nil && info.BlockNumber > 0 {
				progress.Send(5+maxConfirmAttempts, fmt.Sprintf("Confirmed in block %d", info.BlockNumber))
				return mcp.NewToolResultJSON(map[string]any{
					"txid":           txid,
					"success":        info.GetResult() != core.TransactionInfo_FAILED,
					"confirmed":      true,
					"block_number":   info.BlockNumber,
					"fee":            info.Fee,
					"energy_used":    info.Receipt.GetEnergyUsageTotal(),
					"bandwidth_used": info.Receipt.GetNetUsage(),
				})
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

		ret, err := pool.Client().BroadcastCtx(ctx, tx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("broadcast_transaction: %v", err)), nil
		}

		return mcp.NewToolResultJSON(map[string]any{
			"transaction_id": txid,
			"success":        ret.Result,
			"code":           ret.Code.String(),
			"message":        string(ret.Message),
		})
	}
}

func handleGetWalletPolicy(pe *policy.Engine) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		walletName := req.GetString("wallet", "")
		if walletName == "" {
			return mcp.NewToolResultError("wallet is required"), nil
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
			"wallet":         walletName,
			"policy_enabled": true,
			"has_policy":     true,
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
