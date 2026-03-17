package tools

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

// RegisterResourceTools registers freeze_balance and unfreeze_balance (local mode only).
func RegisterResourceTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("freeze_balance",
			mcp.WithDescription("Stake TRX for energy or bandwidth (Stake 2.0). Returns unsigned transaction hex."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Account address")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in TRX to stake")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("Resource type"), mcp.Enum("ENERGY", "BANDWIDTH")),
		),
		handleFreezeBalance(pool),
	)

	s.AddTool(
		mcp.NewTool("unfreeze_balance",
			mcp.WithDescription("Unstake TRX (Stake 2.0). Returns unsigned transaction hex."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Account address")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in TRX to unstake")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("Resource type"), mcp.Enum("ENERGY", "BANDWIDTH")),
		),
		handleUnfreezeBalance(pool),
	)
}

func parseResourceCode(resource string) (core.ResourceCode, error) {
	switch resource {
	case "ENERGY":
		return core.ResourceCode_ENERGY, nil
	case "BANDWIDTH":
		return core.ResourceCode_BANDWIDTH, nil
	default:
		return 0, fmt.Errorf("invalid resource type %q: must be ENERGY or BANDWIDTH", resource)
	}
}

func handleFreezeBalance(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		amountStr := req.GetString("amount", "")
		resourceStr := req.GetString("resource", "")
		grpc := pool.Client()

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}

		sun, err := util.TRXToSun(amountStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid amount: %v", err)), nil
		}
		if sun <= 0 {
			return mcp.NewToolResultError("amount must be greater than zero"), nil
		}

		resource, err := parseResourceCode(resourceStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		tx, err := grpc.FreezeBalanceV2(from, resource, sun)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("freeze_balance: %v", err)), nil
		}

		txBytes, err := proto.Marshal(tx.Transaction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("freeze_balance: failed to serialize: %v", err)), nil
		}

		result := map[string]any{
			"transaction_hex": hex.EncodeToString(txBytes),
			"txid":            hex.EncodeToString(tx.Txid),
			"from":            from,
			"amount_trx":      amountStr,
			"amount_sun":      sun,
			"resource":        resourceStr,
			"type":            "FreezeBalanceV2Contract",
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleUnfreezeBalance(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		amountStr := req.GetString("amount", "")
		resourceStr := req.GetString("resource", "")
		grpc := pool.Client()

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}

		sun, err := util.TRXToSun(amountStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid amount: %v", err)), nil
		}
		if sun <= 0 {
			return mcp.NewToolResultError("amount must be greater than zero"), nil
		}

		resource, err := parseResourceCode(resourceStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		tx, err := grpc.UnfreezeBalanceV2(from, resource, sun)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("unfreeze_balance: %v", err)), nil
		}

		txBytes, err := proto.Marshal(tx.Transaction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("unfreeze_balance: failed to serialize: %v", err)), nil
		}

		result := map[string]any{
			"transaction_hex": hex.EncodeToString(txBytes),
			"txid":            hex.EncodeToString(tx.Txid),
			"from":            from,
			"amount_trx":      amountStr,
			"amount_sun":      sun,
			"resource":        resourceStr,
			"type":            "UnfreezeBalanceV2Contract",
		}

		return mcp.NewToolResultJSON(result)
	}
}
