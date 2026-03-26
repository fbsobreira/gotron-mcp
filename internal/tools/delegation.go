package tools

import (
	"context"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterDelegationTools registers get_account_permissions, get_delegated_resources,
// and get_delegatable_amount (read-only tools).
func RegisterDelegationTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("get_account_permissions",
			mcp.WithDescription("Get the permission structure of a TRON account including owner, active, and witness permissions with multi-sig keys and thresholds"),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON base58 address (starts with T)")),
		),
		handleGetAccountPermissions(pool),
	)

	s.AddTool(
		mcp.NewTool("get_delegated_resources",
			mcp.WithDescription("Get resources delegated to and from an account (Stake 2.0 delegation info)"),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON base58 address (starts with T)")),
		),
		handleGetDelegatedResources(pool),
	)

	s.AddTool(
		mcp.NewTool("get_delegatable_amount",
			mcp.WithDescription("Get the maximum amount of energy or bandwidth an account can still delegate"),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON base58 address (starts with T)")),
			mcp.WithString("resource", mcp.Required(), mcp.Description("Resource type"), mcp.Enum("ENERGY", "BANDWIDTH")),
		),
		handleGetDelegatableAmount(pool),
	)
}

func handleGetAccountPermissions(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		acc, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*core.Account, error) {
			return pool.Client().GetAccountCtx(ctx, addr)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_account_permissions: %v", err)), nil
		}

		result := map[string]any{
			"address": addr,
		}

		if acc.OwnerPermission != nil {
			result["owner"] = formatPermission(acc.OwnerPermission)
		}
		if acc.WitnessPermission != nil {
			result["witness"] = formatPermission(acc.WitnessPermission)
		}
		if len(acc.ActivePermission) > 0 {
			active := make([]map[string]any, len(acc.ActivePermission))
			for i, p := range acc.ActivePermission {
				active[i] = formatPermission(p)
			}
			result["active"] = active
		}

		return mcp.NewToolResultJSON(result)
	}
}

// formatPermission converts a Permission proto to a JSON-friendly map.
func formatPermission(p *core.Permission) map[string]any {
	m := map[string]any{
		"id":        p.Id,
		"name":      p.PermissionName,
		"threshold": p.Threshold,
		"type":      p.Type.String(),
	}

	if len(p.Keys) > 0 {
		keys := make([]map[string]any, len(p.Keys))
		for i, k := range p.Keys {
			keys[i] = map[string]any{
				"address": address.BytesToAddress(k.Address).String(),
				"weight":  k.Weight,
			}
		}
		m["keys"] = keys
	}

	if len(p.Operations) > 0 {
		m["operations"] = fmt.Sprintf("%x", p.Operations)
	}

	return m
}

func handleGetDelegatedResources(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		delegatedTo, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) ([]*api.DelegatedResourceList, error) {
			return pool.Client().GetDelegatedResourcesV2Ctx(ctx, addr)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_delegated_resources: failed to get delegated resources: %v", err)), nil
		}

		receivedFrom, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) ([]*api.DelegatedResourceList, error) {
			return pool.Client().GetReceivedDelegatedResourcesV2Ctx(ctx, addr)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_delegated_resources: failed to get received resources: %v", err)), nil
		}

		result := map[string]any{
			"address":       addr,
			"delegated_to":  formatDelegatedResourceLists(delegatedTo),
			"received_from": formatDelegatedResourceLists(receivedFrom),
		}

		return mcp.NewToolResultJSON(result)
	}
}

// formatDelegatedResourceLists flattens a slice of DelegatedResourceList into
// a slice of JSON-friendly maps.
func formatDelegatedResourceLists(lists []*api.DelegatedResourceList) []map[string]any {
	var out []map[string]any
	for _, list := range lists {
		for _, dr := range list.DelegatedResource {
			entry := map[string]any{
				"from":          address.BytesToAddress(dr.From).String(),
				"to":            address.BytesToAddress(dr.To).String(),
				"energy_sun":    dr.FrozenBalanceForEnergy,
				"energy_trx":    util.SunToTRX(dr.FrozenBalanceForEnergy),
				"bandwidth_sun": dr.FrozenBalanceForBandwidth,
				"bandwidth_trx": util.SunToTRX(dr.FrozenBalanceForBandwidth),
			}
			if dr.ExpireTimeForEnergy > 0 {
				entry["expire_time_energy"] = dr.ExpireTimeForEnergy
			}
			if dr.ExpireTimeForBandwidth > 0 {
				entry["expire_time_bandwidth"] = dr.ExpireTimeForBandwidth
			}
			out = append(out, entry)
		}
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out
}

func handleGetDelegatableAmount(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		resourceStr := req.GetString("resource", "")
		resource, err := parseResourceCode(resourceStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		resp, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*api.CanDelegatedMaxSizeResponseMessage, error) {
			return pool.Client().GetCanDelegatedMaxSizeCtx(ctx, addr, int32(resource))
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_delegatable_amount: %v", err)), nil
		}

		result := map[string]any{
			"address":             addr,
			"resource":            resourceStr,
			"max_delegatable_sun": resp.MaxSize,
			"max_delegatable_trx": util.SunToTRX(resp.MaxSize),
		}

		return mcp.NewToolResultJSON(result)
	}
}
