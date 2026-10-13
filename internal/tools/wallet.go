package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/pqc"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterWalletTools registers create_wallet and list_wallets.
func RegisterWalletTools(s *server.MCPServer, wm *wallet.Manager) {
	s.AddTool(
		mcp.NewTool("create_wallet",
			mcp.WithDescription("Create a new named wallet in the MCP keystore. Returns the wallet name, TRON address and key_type; post-quantum wallets also return notes."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Wallet name (alphanumeric, hyphens, underscores)")),
			mcp.WithString("key_type",
				mcp.Enum(string(wallet.KeyTypeECDSA), string(wallet.KeyTypeMLDSA44), string(wallet.KeyTypeFNDSA512)),
				mcp.Description("Signature key type (default ecdsa). ml_dsa_44 and fn_dsa_512 are TIP-899 post-quantum keys, live only on Nile today; fn_dsa_512 needs the gotron-mcp-pq build. Check get_pq_capabilities first."),
			),
		),
		handleCreateWallet(wm),
	)

	s.AddTool(
		mcp.NewTool("list_wallets",
			mcp.WithDescription("List all wallets managed by this MCP server with their names, addresses and key types (ecdsa, ml_dsa_44 or fn_dsa_512)."),
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

		keyTypeArg := ""
		if raw, ok := req.GetArguments()["key_type"]; ok && raw != nil {
			str, isStr := raw.(string)
			if !isStr {
				return mcp.NewToolResultError("create_wallet: key_type must be a string"), nil
			}
			keyTypeArg = str
		}
		kt, err := wallet.ParseKeyType(keyTypeArg)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create_wallet: %v", err)), nil
		}

		addr, err := wm.CreateWalletWithKeyType(name, kt)
		if err != nil {
			if errors.Is(err, pqc.ErrFalconUnavailable) {
				return mcp.NewToolResultError("create_wallet: this build cannot create FN-DSA-512 keys; it needs the gotron-mcp-pq build, or build locally with liboqs installed: CGO_ENABLED=1 go build -tags falcon ./cmd/gotron-mcp. ML-DSA-44 works in every build."), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("create_wallet: %v", err)), nil
		}

		result := map[string]any{
			"name":     name,
			"address":  addr,
			"key_type": string(kt),
		}
		if kt.IsPQ() {
			result["notes"] = []string{
				"TIP-899 post-quantum keys work only on networks that enable the scheme (today: Nile testnet). Check with get_pq_capabilities.",
				"A new address must receive TRX before it can send transactions.",
				"Post-quantum signatures make transactions about 10-22x larger, so they use more bandwidth.",
			}
		}
		return mcp.NewToolResultJSON(result)
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
			item := map[string]any{
				"name":     w.Name,
				"address":  w.Address,
				"key_type": string(w.KeyType),
			}
			if w.Duplicate {
				item["duplicate_name"] = true
			}
			items[i] = item
		}

		return mcp.NewToolResultJSON(map[string]any{
			"wallets": items,
			"count":   len(items),
		})
	}
}
