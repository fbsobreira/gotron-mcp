package tools

import (
	"context"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterAccountTools registers get_account and get_account_resources tools.
func RegisterAccountTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("get_account",
			mcp.WithDescription("Get TRON account balance and details including TRX balance, bandwidth, energy, frozen resources, and account type"),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON base58 address (starts with T)")),
		),
		handleGetAccount(pool),
	)

	s.AddTool(
		mcp.NewTool("get_account_resources",
			mcp.WithDescription("Get energy and bandwidth usage and limits for a TRON account"),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON base58 address (starts with T)")),
		),
		handleGetAccountResources(pool),
	)
}

func handleGetAccount(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		conn := pool.Client()
		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		acc, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*core.Account, error) {
			return conn.GetAccountCtx(ctx, addr)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_account: failed to fetch account %s: %v", addr, err)), nil
		}

		isContract := acc.Type == 2 // AccountType_Contract = 2

		result := map[string]any{
			"address":      addr,
			"balance_trx":  util.SunToTRX(acc.Balance),
			"balance_sun":  acc.Balance,
			"is_contract":  isContract,
			"account_type": acc.Type.String(),
		}

		if len(acc.AccountName) > 0 {
			result["account_name"] = string(acc.AccountName)
		}
		if acc.CreateTime > 0 {
			result["create_time"] = acc.CreateTime
		}
		if acc.AccountResource != nil {
			if acc.AccountResource.EnergyUsage > 0 {
				result["energy_usage"] = acc.AccountResource.EnergyUsage
			}
			if acc.AccountResource.FrozenBalanceForEnergy != nil {
				result["frozen_balance_for_energy"] = acc.AccountResource.FrozenBalanceForEnergy.FrozenBalance
			}
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleGetAccountResources(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		conn := pool.Client()
		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		res, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*api.AccountResourceMessage, error) {
			return conn.GetAccountResourceCtx(ctx, addr)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_account_resources: failed to fetch resources for %s: %v", addr, err)), nil
		}

		result := map[string]any{
			"address":               addr,
			"energy_used":           res.EnergyUsed,
			"energy_limit":          res.EnergyLimit,
			"bandwidth_used":        res.NetUsed,
			"bandwidth_limit":       res.NetLimit,
			"free_bandwidth_used":   res.FreeNetUsed,
			"free_bandwidth_limit":  res.FreeNetLimit,
			"total_energy_limit":    res.TotalEnergyLimit,
			"total_bandwidth_limit": res.TotalNetLimit,
		}

		return mcp.NewToolResultJSON(result)
	}
}
