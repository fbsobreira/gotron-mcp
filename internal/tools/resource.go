package tools

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/fbsobreira/gotron-sdk/pkg/txbuilder"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

// RegisterResourceTools registers freeze_balance, unfreeze_balance,
// delegate_resource, undelegate_resource, and withdraw_expire_unfreeze (local mode only).
func RegisterResourceTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("freeze_balance",
			mcp.WithDescription("Stake TRX for energy or bandwidth (Stake 2.0). Returns unsigned transaction hex for signing."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Account address (base58, starts with T)")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in TRX to stake (e.g., '100.5')")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("Resource type"), mcp.Enum("ENERGY", "BANDWIDTH")),
			mcp.WithString("memo", mcp.Description("Optional memo to attach to the transaction")),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID for multi-sig transactions")),
		),
		handleFreezeBalance(pool),
	)

	s.AddTool(
		mcp.NewTool("unfreeze_balance",
			mcp.WithDescription("Unstake TRX (Stake 2.0). Returns unsigned transaction hex for signing. Note: unstaked TRX has a 14-day withdrawal period."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Account address (base58, starts with T)")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in TRX to unstake (e.g., '100.5')")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("Resource type"), mcp.Enum("ENERGY", "BANDWIDTH")),
			mcp.WithString("memo", mcp.Description("Optional memo to attach to the transaction")),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID for multi-sig transactions")),
		),
		handleUnfreezeBalance(pool),
	)

	s.AddTool(
		mcp.NewTool("delegate_resource",
			mcp.WithDescription("Delegate energy or bandwidth to another address (Stake 2.0). Returns unsigned transaction hex for signing."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Delegator address (base58, starts with T)")),
			mcp.WithString("to", mcp.Required(), mcp.Description("Recipient address (base58, starts with T)")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("Resource type"), mcp.Enum("ENERGY", "BANDWIDTH")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in TRX to delegate (e.g., '100.5')")),
			mcp.WithNumber("lock_period", mcp.Description("Lock period in blocks (~3 seconds each). If set, delegation cannot be undone until the lock expires.")),
			mcp.WithString("memo", mcp.Description("Optional memo to attach to the transaction")),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID for multi-sig transactions")),
		),
		handleDelegateResource(pool),
	)

	s.AddTool(
		mcp.NewTool("undelegate_resource",
			mcp.WithDescription("Reclaim previously delegated energy or bandwidth (Stake 2.0). Returns unsigned transaction hex for signing."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Delegator address (base58, starts with T)")),
			mcp.WithString("to", mcp.Required(), mcp.Description("Address to reclaim from (base58, starts with T)")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("Resource type"), mcp.Enum("ENERGY", "BANDWIDTH")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in TRX to undelegate (e.g., '100.5')")),
			mcp.WithString("memo", mcp.Description("Optional memo to attach to the transaction")),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID for multi-sig transactions")),
		),
		handleUndelegateResource(pool),
	)

	s.AddTool(
		mcp.NewTool("withdraw_expire_unfreeze",
			mcp.WithDescription("Withdraw TRX that has completed the 14-day unstaking period (Stake 2.0). Returns unsigned transaction hex for signing."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Account address (base58, starts with T)")),
			mcp.WithString("memo", mcp.Description("Optional memo to attach to the transaction")),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID for multi-sig transactions")),
		),
		handleWithdrawExpireUnfreeze(pool),
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
		conn := pool.Client()

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

		opts := builderOptions(req)
		tx, err := txbuilder.New(conn).FreezeV2(from, sun, resource, opts...).Build(ctx)
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
		conn := pool.Client()

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

		opts := builderOptions(req)
		tx, err := txbuilder.New(conn).UnfreezeV2(from, sun, resource, opts...).Build(ctx)
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

func handleDelegateResource(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		amountStr := req.GetString("amount", "")
		resourceStr := req.GetString("resource", "")
		lockPeriod := int64(req.GetInt("lock_period", 0))
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

		resource, err := parseResourceCode(resourceStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if lockPeriod < 0 {
			return mcp.NewToolResultError("delegate_resource: lock_period must be non-negative"), nil
		}

		opts := builderOptions(req)
		dt := txbuilder.New(conn).DelegateResource(from, to, resource, sun, opts...)
		if lockPeriod > 0 {
			dt = dt.Lock(lockPeriod)
		}
		tx, err := dt.Build(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delegate_resource: %v", err)), nil
		}

		txBytes, err := proto.Marshal(tx.Transaction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delegate_resource: failed to serialize: %v", err)), nil
		}

		result := map[string]any{
			"transaction_hex": hex.EncodeToString(txBytes),
			"txid":            hex.EncodeToString(tx.Txid),
			"from":            from,
			"to":              to,
			"amount_trx":      amountStr,
			"amount_sun":      sun,
			"resource":        resourceStr,
			"type":            "DelegateResourceContract",
		}
		if lockPeriod > 0 {
			result["lock_period"] = lockPeriod
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleUndelegateResource(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		amountStr := req.GetString("amount", "")
		resourceStr := req.GetString("resource", "")
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

		resource, err := parseResourceCode(resourceStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		opts := builderOptions(req)
		tx, err := txbuilder.New(conn).UnDelegateResource(from, to, resource, sun, opts...).Build(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("undelegate_resource: %v", err)), nil
		}

		txBytes, err := proto.Marshal(tx.Transaction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("undelegate_resource: failed to serialize: %v", err)), nil
		}

		result := map[string]any{
			"transaction_hex": hex.EncodeToString(txBytes),
			"txid":            hex.EncodeToString(tx.Txid),
			"from":            from,
			"to":              to,
			"amount_trx":      amountStr,
			"amount_sun":      sun,
			"resource":        resourceStr,
			"type":            "UnDelegateResourceContract",
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleWithdrawExpireUnfreeze(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		conn := pool.Client()

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}

		opts := builderOptions(req)
		tx, err := txbuilder.New(conn).WithdrawExpireUnfreeze(from, 0, opts...).Build(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("withdraw_expire_unfreeze: %v", err)), nil
		}

		txBytes, err := proto.Marshal(tx.Transaction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("withdraw_expire_unfreeze: failed to serialize: %v", err)), nil
		}

		result := map[string]any{
			"transaction_hex": hex.EncodeToString(txBytes),
			"txid":            hex.EncodeToString(tx.Txid),
			"from":            from,
			"type":            "WithdrawExpireUnfreezeContract",
		}

		return mcp.NewToolResultJSON(result)
	}
}
