package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/fbsobreira/gotron-sdk/pkg/contract"
	"github.com/fbsobreira/gotron-sdk/pkg/standards/trc20"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

// trc20CacheSize is the max number of token metadata entries cached in memory.
const trc20CacheSize = 256

// RegisterTokenTools registers TRC20 balance and token info tools.
// Returns the shared metadata cache for use by other TRC20 tools (e.g., transfer_trc20).
func RegisterTokenTools(s *server.MCPServer, pool *nodepool.Pool) *trc20.MetadataCache {
	cache := trc20.NewMetadataCache(trc20CacheSize)

	s.AddTool(
		mcp.NewTool("get_trc20_balance",
			mcp.WithDescription("Get TRC20 token balance for an account"),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON base58 address")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("TRC20 contract address")),
		),
		handleGetTRC20Balance(pool, cache),
	)

	s.AddTool(
		mcp.NewTool("get_trc20_token_info",
			mcp.WithDescription("Get TRC20 token metadata (name, symbol, decimals, total supply)"),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("TRC20 contract address")),
		),
		handleGetTRC20TokenInfo(pool, cache),
	)

	s.AddTool(
		mcp.NewTool("estimate_trc20_energy",
			mcp.WithDescription("Estimate energy cost for a TRC20 transfer without creating a transaction. Dry-runs the transfer to check energy requirements."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Sender address (base58, starts with T)")),
			mcp.WithString("to", mcp.Required(), mcp.Description("Recipient address (base58, starts with T)")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("TRC20 contract address (base58, starts with T)")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount in human-readable units (e.g., '100.5' for 100.5 USDT)")),
		),
		handleEstimateTRC20Energy(pool, cache),
	)

	return cache
}

func trc20Token(conn contract.Client, contractAddr string, cache *trc20.MetadataCache) *trc20.Token {
	return trc20.New(conn, contractAddr, trc20.WithCache(cache))
}

func handleGetTRC20Balance(pool *nodepool.Pool, cache *trc20.MetadataCache) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		contractAddr := req.GetString("contract_address", "")

		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := validateAddress(contractAddr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		bal, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*trc20.Balance, error) {
			return trc20Token(pool.Client(), contractAddr, cache).BalanceOf(ctx, addr)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_balance: %v", err)), nil
		}

		name, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (string, error) {
			return trc20Token(pool.Client(), contractAddr, cache).Name(ctx)
		})
		if err != nil {
			name = ""
		}

		decimals, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (uint8, error) {
			return trc20Token(pool.Client(), contractAddr, cache).Decimals(ctx)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_balance: failed to get decimals: %v", err)), nil
		}

		result := map[string]any{
			"address":          addr,
			"contract_address": contractAddr,
			"balance":          bal.Display,
			"balance_raw":      bal.Raw.String(),
			"token_name":       name,
			"token_symbol":     bal.Symbol,
			"decimals":         decimals,
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleGetTRC20TokenInfo(pool *nodepool.Pool, cache *trc20.MetadataCache) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		contractAddr := req.GetString("contract_address", "")
		if err := validateAddress(contractAddr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		info, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*trc20.TokenInfo, error) {
			return trc20Token(pool.Client(), contractAddr, cache).Info(ctx)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_token_info: %v", err)), nil
		}

		result := map[string]any{
			"contract_address": contractAddr,
			"name":             info.Name,
			"symbol":           info.Symbol,
			"decimals":         info.Decimals,
			"total_supply":     info.TotalSupply.String(),
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleEstimateTRC20Energy(pool *nodepool.Pool, cache *trc20.MetadataCache) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		contractAddr := req.GetString("contract_address", "")
		amountStr := req.GetString("amount", "")

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}
		if err := validateAddress(to); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid to address: %v", err)), nil
		}
		if err := validateAddress(contractAddr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		token := trc20Token(pool.Client(), contractAddr, cache)
		decimals, err := token.Decimals(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("estimate_trc20_energy: failed to get decimals: %v", err)), nil
		}

		amount, err := util.ParseTRC20Amount(amountStr, int(decimals))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid amount: %v", err)), nil
		}
		if amount.Sign() <= 0 {
			return mcp.NewToolResultError("amount must be greater than zero"), nil
		}

		energy, err := token.Transfer(from, to, amount).EstimateEnergy(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("estimate_trc20_energy: %v", err)), nil
		}

		result := map[string]any{
			"from":             from,
			"to":               to,
			"contract_address": contractAddr,
			"amount":           amountStr,
			"estimated_energy": energy,
		}

		return mcp.NewToolResultJSON(result)
	}
}

// handleTransferTRC20 builds an unsigned TRC20 transfer using the shared trc20Token helper.
func handleTransferTRC20(pool *nodepool.Pool, cache *trc20.MetadataCache) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		contractAddr := req.GetString("contract_address", "")
		amountStr := req.GetString("amount", "")
		feeLimit := req.GetInt("fee_limit", 100)
		conn := pool.Client()

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}
		if err := validateAddress(to); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid to address: %v", err)), nil
		}
		if err := validateAddress(contractAddr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		token := trc20Token(conn, contractAddr, cache)
		decimals, err := token.Decimals(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("transfer_trc20: failed to get decimals: %v", err)), nil
		}

		amount, err := util.ParseTRC20Amount(amountStr, int(decimals))
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

		call := token.Transfer(from, to, amount, contract.WithFeeLimit(feeLimitSun))
		if call.Err() != nil {
			return mcp.NewToolResultError(fmt.Sprintf("transfer_trc20: %v", call.Err())), nil
		}

		// Apply optional permission_id for multi-sig
		args := req.GetArguments()
		if _, has := args["permission_id"]; has {
			pid := req.GetInt("permission_id", 0)
			if pid < 0 || pid > math.MaxInt32 {
				return mcp.NewToolResultError("transfer_trc20: permission_id must be between 0 and 2147483647"), nil
			}
			call = call.WithPermissionID(int32(pid))
		}

		tx, err := call.Build(ctx)
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
			"contract_address": contractAddr,
			"amount":           amountStr,
			"amount_raw":       amount.String(),
			"fee_limit_trx":    feeLimit,
			"type":             "TriggerSmartContract",
		}

		return mcp.NewToolResultJSON(result)
	}
}
