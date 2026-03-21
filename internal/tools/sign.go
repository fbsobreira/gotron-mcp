package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
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
func RegisterSignTools(s *server.MCPServer, pool *nodepool.Pool, wm *wallet.Manager) {
	s.AddTool(
		mcp.NewTool("sign_transaction",
			mcp.WithDescription("Sign an unsigned transaction using a managed wallet. Returns signed transaction hex for broadcasting."),
			mcp.WithString("transaction_hex", mcp.Required(), mcp.Description("Unsigned transaction hex")),
			mcp.WithString("wallet", mcp.Required(), mcp.Description("Wallet name or address")),
		),
		handleSignTransaction(wm),
	)

	s.AddTool(
		mcp.NewTool("sign_and_broadcast",
			mcp.WithDescription("Sign and broadcast a transaction in one step. Decodes the transaction for future policy enforcement."),
			mcp.WithString("transaction_hex", mcp.Required(), mcp.Description("Unsigned transaction hex")),
			mcp.WithString("wallet", mcp.Required(), mcp.Description("Wallet name or address")),
		),
		handleSignAndBroadcast(pool, wm),
	)

	s.AddTool(
		mcp.NewTool("sign_and_confirm",
			mcp.WithDescription("Sign, broadcast, and wait for on-chain confirmation. Polls until the transaction is confirmed or timeout."),
			mcp.WithString("transaction_hex", mcp.Required(), mcp.Description("Unsigned transaction hex")),
			mcp.WithString("wallet", mcp.Required(), mcp.Description("Wallet name or address")),
		),
		handleSignAndConfirm(pool, wm),
	)

	s.AddTool(
		mcp.NewTool("broadcast_transaction",
			mcp.WithDescription("Broadcast a signed transaction to the TRON network"),
			mcp.WithString("signed_transaction_hex", mcp.Required(), mcp.Description("Signed transaction hex")),
		),
		handleBroadcastTransaction(pool),
	)
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
		txHex := req.GetString("transaction_hex", "")
		walletName := req.GetString("wallet", "")

		if walletName == "" {
			return mcp.NewToolResultError("wallet is required"), nil
		}

		tx, err := parseAndValidateTx(txHex)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

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

func handleSignAndBroadcast(pool *nodepool.Pool, wm *wallet.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		txHex := req.GetString("transaction_hex", "")
		walletName := req.GetString("wallet", "")

		if walletName == "" {
			return mcp.NewToolResultError("wallet is required"), nil
		}

		tx, err := parseAndValidateTx(txHex)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Decode contract data for future policy enforcement
		var contractType string
		var contractData map[string]any
		if decoded, decErr := transaction.DecodeContractData(tx); decErr == nil {
			contractType = decoded.Type
			contractData = decoded.Fields
		}

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

		conn := pool.Client()
		ret, err := conn.BroadcastCtx(ctx, signedTx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_broadcast: %v", err)), nil
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

func handleSignAndConfirm(pool *nodepool.Pool, wm *wallet.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		txHex := req.GetString("transaction_hex", "")
		walletName := req.GetString("wallet", "")

		if walletName == "" {
			return mcp.NewToolResultError("wallet is required"), nil
		}

		tx, err := parseAndValidateTx(txHex)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Decode contract data for future policy enforcement
		if decoded, decErr := transaction.DecodeContractData(tx); decErr == nil {
			_ = decoded // logged for future policy use
		}

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

		conn := pool.Client()
		ret, err := conn.BroadcastCtx(ctx, signedTx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: broadcast failed: %v", err)), nil
		}
		if !ret.Result {
			return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: broadcast rejected: %s %s", ret.Code.String(), string(ret.Message))), nil
		}

		// Poll for confirmation
		const maxConfirmAttempts = 20
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for attempt := 0; attempt < maxConfirmAttempts; attempt++ {
			select {
			case <-ctx.Done():
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: context cancelled waiting for confirmation of %s", txid)), nil
			case <-ticker.C:
			}

			info, infoErr := conn.GetTransactionInfoByIDCtx(ctx, txid)
			if infoErr != nil {
				if strings.Contains(infoErr.Error(), "not found") {
					continue // not indexed yet
				}
				return mcp.NewToolResultError(fmt.Sprintf("sign_and_confirm: %v", infoErr)), nil
			}
			if info != nil && info.BlockNumber > 0 {
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
		conn := pool.Client()
		if txHex == "" {
			return mcp.NewToolResultError("signed_transaction_hex is required"), nil
		}
		if len(txHex) > maxHexInputLen {
			return mcp.NewToolResultError("signed_transaction_hex exceeds maximum length"), nil
		}

		txBytes, err := hex.DecodeString(txHex)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid signed_transaction_hex: %v", err)), nil
		}

		var tx core.Transaction
		if err := proto.Unmarshal(txBytes, &tx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse transaction: %v", err)), nil
		}

		ret, err := conn.BroadcastCtx(ctx, &tx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("broadcast_transaction: %v", err)), nil
		}

		rawData, err := proto.Marshal(tx.RawData)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to compute txid: %v", err)), nil
		}
		txHash := sha256.Sum256(rawData)

		result := map[string]any{
			"transaction_id": hex.EncodeToString(txHash[:]),
			"success":        ret.Result,
			"code":           ret.Code.String(),
			"message":        string(ret.Message),
		}

		return mcp.NewToolResultJSON(result)
	}
}
