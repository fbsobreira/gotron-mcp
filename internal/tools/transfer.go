package tools

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/standards/trc20"
	"github.com/fbsobreira/gotron-sdk/pkg/txbuilder"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

// resolveFromAddress resolves a wallet name or address to a base58 address.
// If wm is available, tries wallet name lookup first. Returns a clear error
// hint if the input is neither a valid address nor a known wallet name.
func resolveFromAddress(wm *wallet.Manager, nameOrAddr string) (string, error) {
	if wm != nil {
		if addr, err := wm.ResolveAddress(nameOrAddr); err == nil {
			if vErr := validateAddress(addr); vErr == nil {
				return addr, nil
			}
		}
	}
	if err := validateAddress(nameOrAddr); err != nil {
		if wm != nil {
			return "", fmt.Errorf("invalid from: %q is not a valid TRON address or known wallet name — use list_wallets to find the address", nameOrAddr)
		}
		return "", fmt.Errorf("invalid from address: %v", err)
	}
	return nameOrAddr, nil
}

// RegisterTransferTools registers transfer_trx and transfer_trc20.
// The wallet manager is optional — when provided, allows using wallet names in the "from" field.
// The cache is shared with RegisterTokenTools for TRC20 metadata caching.
func RegisterTransferTools(s *server.MCPServer, pool *nodepool.Pool, cache *trc20.MetadataCache, wm ...*wallet.Manager) {
	var walletMgr *wallet.Manager
	if len(wm) > 0 {
		walletMgr = wm[0]
	}

	fromDesc := "Sender address (base58) or wallet name"
	s.AddTool(
		mcp.NewTool("transfer_trx",
			mcp.WithDescription("Create an unsigned TRX transfer transaction. Returns transaction hex for signing."),
			mcp.WithString("from", mcp.Required(), mcp.Description(fromDesc)),
			mcp.WithString("to", mcp.Required(), mcp.Description("Recipient address (base58, starts with T)")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in TRX (e.g., '100.5')")),
			mcp.WithString("memo", mcp.Description("Optional memo to attach to the transaction")),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID for multi-sig transactions")),
		),
		handleTransferTRX(pool, walletMgr),
	)

	s.AddTool(
		mcp.NewTool("transfer_trc20",
			mcp.WithDescription("Create an unsigned TRC20 token transfer transaction. Returns transaction hex for signing."),
			mcp.WithString("from", mcp.Required(), mcp.Description(fromDesc)),
			mcp.WithString("to", mcp.Required(), mcp.Description("Recipient address (base58, starts with T)")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("TRC20 contract address (base58, starts with T)")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in token units (human-readable, e.g., '100.5')")),
			mcp.WithNumber("fee_limit", mcp.Description("Fee limit in TRX, range 0-15000 (default: 100)")),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID for multi-sig transactions")),
		),
		handleTransferTRC20(pool, cache, walletMgr),
	)
}

func handleTransferTRX(pool *nodepool.Pool, wm *wallet.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		fromInput := req.GetString("from", "")
		to := req.GetString("to", "")
		amountStr := req.GetString("amount", "")
		conn := pool.Client()

		from, err := resolveFromAddress(wm, fromInput)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("transfer_trx: %v", err)), nil
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
