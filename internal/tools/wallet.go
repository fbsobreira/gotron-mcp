package tools

import (
	"context"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterWalletTools registers create_wallet and list_wallets.
func RegisterWalletTools(s *server.MCPServer, wm *wallet.Manager) {
	s.AddTool(
		mcp.NewTool("create_wallet",
			mcp.WithDescription("Create a new named wallet in the MCP keystore. Returns the wallet name and TRON address."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Wallet name (alphanumeric, hyphens, underscores)")),
		),
		handleCreateWallet(wm),
	)

	s.AddTool(
		mcp.NewTool("list_wallets",
			mcp.WithDescription("List all wallets managed by this MCP server with their names and addresses."),
		),
		handleListWallets(wm),
	)
}

func handleCreateWallet(wm *wallet.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}

		addr, err := wm.CreateWallet(name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create_wallet: %v", err)), nil
		}

		return mcp.NewToolResultJSON(map[string]any{
			"name":    name,
			"address": addr,
		})
	}
}

func handleListWallets(wm *wallet.Manager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wallets, err := wm.ListWallets()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list_wallets: %v", err)), nil
		}

		items := make([]map[string]any, len(wallets))
		for i, w := range wallets {
			items[i] = map[string]any{
				"name":    w.Name,
				"address": w.Address,
			}
		}

		return mcp.NewToolResultJSON(map[string]any{
			"wallets": items,
			"count":   len(items),
		})
	}
}
