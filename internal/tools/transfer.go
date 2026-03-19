package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/fbsobreira/gotron-sdk/pkg/txbuilder"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

// RegisterTransferTools registers transfer_trx and transfer_trc20 (local mode only).
func RegisterTransferTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("transfer_trx",
			mcp.WithDescription("Create an unsigned TRX transfer transaction. Returns transaction hex for signing."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Sender address (base58, starts with T)")),
			mcp.WithString("to", mcp.Required(), mcp.Description("Recipient address (base58, starts with T)")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in TRX (e.g., '100.5')")),
			mcp.WithString("memo", mcp.Description("Optional memo to attach to the transaction")),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID for multi-sig transactions")),
		),
		handleTransferTRX(pool),
	)

	s.AddTool(
		mcp.NewTool("transfer_trc20",
			mcp.WithDescription("Create an unsigned TRC20 token transfer transaction. Returns transaction hex for signing."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Sender address (base58, starts with T)")),
			mcp.WithString("to", mcp.Required(), mcp.Description("Recipient address (base58, starts with T)")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("TRC20 contract address (base58, starts with T)")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in token units (human-readable, e.g., '100.5')")),
			mcp.WithNumber("fee_limit", mcp.Description("Fee limit in TRX, range 0-15000 (default: 100)")),
		),
		handleTransferTRC20(pool),
	)
}

func handleTransferTRX(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		amountStr := req.GetString("amount", "")
		conn := pool.Client()

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}
		if err := validateAddress(to); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid to address: %v", err)), nil
		}

		sun, err := util.TRXToSun(amountStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid amount: %v", err)), nil
		}
		if sun <= 0 {
			return mcp.NewToolResultError("amount must be greater than zero"), nil
		}

		opts := builderOptions(req)
		tx, err := txbuilder.New(conn).Transfer(from, to, sun, opts...).Build(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("transfer_trx: %v", err)), nil
		}

		txBytes, err := proto.Marshal(tx.Transaction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("transfer_trx: failed to serialize: %v", err)), nil
		}

		result := map[string]any{
			"transaction_hex": hex.EncodeToString(txBytes),
			"txid":            hex.EncodeToString(tx.Txid),
			"from":            from,
			"to":              to,
			"amount_trx":      amountStr,
			"amount_sun":      sun,
			"type":            "TransferContract",
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleTransferTRC20(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		contract := req.GetString("contract_address", "")
		amountStr := req.GetString("amount", "")
		feeLimit := req.GetInt("fee_limit", 100)
		conn := pool.Client()

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}
		if err := validateAddress(to); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid to address: %v", err)), nil
		}
		if err := validateAddress(contract); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		decimals, err := conn.TRC20GetDecimalsCtx(ctx, contract)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("transfer_trc20: failed to get decimals: %v", err)), nil
		}
		if decimals == nil || decimals.Sign() < 0 || decimals.Cmp(big.NewInt(77)) > 0 {
			return mcp.NewToolResultError(fmt.Sprintf("transfer_trc20: invalid decimals value: %s", decimals)), nil
		}
		dec := int(decimals.Int64())

		amount, err := util.ParseTRC20Amount(amountStr, dec)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid amount: %v", err)), nil
		}
		if amount.Sign() <= 0 {
			return mcp.NewToolResultError("amount must be greater than zero"), nil
		}

		if feeLimit < 0 || feeLimit > 15000 {
			return mcp.NewToolResultError("fee_limit must be between 0 and 15000 TRX"), nil
		}
		feeLimitSun := int64(feeLimit) * 1_000_000

		tx, err := conn.TRC20SendCtx(ctx, from, to, contract, amount, feeLimitSun)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("transfer_trc20: %v", err)), nil
		}

		txBytes, err := proto.Marshal(tx.Transaction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("transfer_trc20: failed to serialize: %v", err)), nil
		}

		result := map[string]any{
			"transaction_hex":  hex.EncodeToString(txBytes),
			"txid":             hex.EncodeToString(tx.Txid),
			"from":             from,
			"to":               to,
			"contract_address": contract,
			"amount":           amountStr,
			"amount_raw":       amount.String(),
			"fee_limit_trx":    feeLimit,
			"type":             "TriggerSmartContract",
		}

		return mcp.NewToolResultJSON(result)
	}
}
