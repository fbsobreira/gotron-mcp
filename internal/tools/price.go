package tools

import (
	"context"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/price"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterPriceTools registers the get_token_price tool (read-only).
func RegisterPriceTools(s *server.MCPServer, priceSvc *price.Service) {
	if priceSvc == nil {
		return
	}
	s.AddTool(
		mcp.NewTool("get_token_price",
			mcp.WithDescription("Get the current USD price of TRX or a TRC20 token. Uses CoinGecko with caching."),
			mcp.WithString("token", mcp.Required(), mcp.Description("Token to price: 'TRX' or a TRC20 contract address (e.g., TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t for USDT)")),
		),
		handleGetTokenPrice(priceSvc),
	)
}

func handleGetTokenPrice(priceSvc *price.Service) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		token := req.GetString("token", "")
		if token == "" {
			return mcp.NewToolResultError("token is required"), nil
		}

		usdPrice, err := priceSvc.GetTokenPrice(ctx, token)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_token_price: %v", err)), nil
		}

		result := map[string]any{
			"token":     token,
			"usd_price": usdPrice,
			"currency":  "USD",
		}

		return mcp.NewToolResultJSON(result)
	}
}
