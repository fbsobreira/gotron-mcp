package tools

import (
	"context"
	"fmt"
	"math/big"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterTokenTools registers TRC20 balance and token info tools.
func RegisterTokenTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("get_trc20_balance",
			mcp.WithDescription("Get TRC20 token balance for an account"),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON base58 address")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("TRC20 contract address")),
		),
		handleGetTRC20Balance(pool),
	)

	s.AddTool(
		mcp.NewTool("get_trc20_token_info",
			mcp.WithDescription("Get TRC20 token metadata (name, symbol, decimals)"),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("TRC20 contract address")),
		),
		handleGetTRC20TokenInfo(pool),
	)
}

func handleGetTRC20Balance(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		contract := req.GetString("contract_address", "")
		grpc := pool.Client()

		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := validateAddress(contract); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		balance, err := retry.Do(func() (*big.Int, error) {
			return grpc.TRC20ContractBalance(addr, contract)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_balance: %v", err)), nil
		}

		decimals, err := retry.Do(func() (*big.Int, error) {
			return grpc.TRC20GetDecimals(contract)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_balance: failed to get decimals: %v", err)), nil
		}

		name, err := retry.Do(func() (string, error) {
			return grpc.TRC20GetName(contract)
		})
		if err != nil {
			name = ""
		}
		symbol, err := retry.Do(func() (string, error) {
			return grpc.TRC20GetSymbol(contract)
		})
		if err != nil {
			symbol = ""
		}

		if decimals == nil || decimals.Sign() < 0 || decimals.Cmp(big.NewInt(77)) > 0 {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_balance: invalid decimals value: %s", decimals)), nil
		}
		dec := int(decimals.Int64())

		result := map[string]any{
			"address":          addr,
			"contract_address": contract,
			"balance":          util.FormatTRC20Amount(balance, dec),
			"balance_raw":      balance.String(),
			"token_name":       name,
			"token_symbol":     symbol,
			"decimals":         dec,
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleGetTRC20TokenInfo(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		contract := req.GetString("contract_address", "")
		grpc := pool.Client()
		if err := validateAddress(contract); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		name, err := grpc.TRC20GetName(contract)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_token_info: failed to get name: %v", err)), nil
		}

		symbol, err := grpc.TRC20GetSymbol(contract)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_token_info: failed to get symbol: %v", err)), nil
		}

		decimals, err := grpc.TRC20GetDecimals(contract)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_token_info: failed to get decimals: %v", err)), nil
		}

		result := map[string]any{
			"contract_address": contract,
			"name":             name,
			"symbol":           symbol,
			"decimals":         decimals.Int64(),
		}

		return mcp.NewToolResultJSON(result)
	}
}
