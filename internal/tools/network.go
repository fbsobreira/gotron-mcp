package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterNetworkTools registers transaction, chain parameters, price, and network info tools.
func RegisterNetworkTools(s *server.MCPServer, pool *nodepool.Pool, network, node string) {
	s.AddTool(
		mcp.NewTool("get_network",
			mcp.WithDescription("Get current MCP server connection info: network name, node address, and latest block"),
		),
		handleGetNetwork(pool, network, node),
	)

	s.AddTool(
		mcp.NewTool("get_transaction",
			mcp.WithDescription("Get transaction details by transaction ID"),
			mcp.WithString("transaction_id", mcp.Required(), mcp.Description("Transaction ID (hex string)")),
		),
		handleGetTransaction(pool),
	)

	s.AddTool(
		mcp.NewTool("get_chain_parameters",
			mcp.WithDescription("Get TRON network node info and current energy/bandwidth prices"),
		),
		handleGetChainParameters(pool),
	)

	s.AddTool(
		mcp.NewTool("get_energy_prices",
			mcp.WithDescription("Get current and historical energy prices on the TRON network"),
		),
		handleGetEnergyPrices(pool),
	)

	s.AddTool(
		mcp.NewTool("get_bandwidth_prices",
			mcp.WithDescription("Get current and historical bandwidth prices on the TRON network"),
		),
		handleGetBandwidthPrices(pool),
	)
}

func handleGetTransaction(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		txID := req.GetString("transaction_id", "")
		if txID == "" {
			return mcp.NewToolResultError("transaction_id is required"), nil
		}

		tx, err := retry.Do(func() (*core.Transaction, error) {
			return pool.Client().GetTransactionByID(txID)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_transaction: %v", err)), nil
		}

		info, err := retry.Do(func() (*core.TransactionInfo, error) {
			return pool.Client().GetTransactionInfoByID(txID)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_transaction: failed to get info: %v", err)), nil
		}

		result := map[string]any{
			"transaction_id":  txID,
			"block_number":    info.BlockNumber,
			"block_timestamp": info.BlockTimeStamp,
			"fee":             info.Fee,
			"result":          normalizeResult(info.Result.String()),
		}

		if info.Receipt != nil {
			result["receipt"] = map[string]any{
				"energy_usage":       info.Receipt.EnergyUsage,
				"energy_fee":         info.Receipt.EnergyFee,
				"net_usage":          info.Receipt.NetUsage,
				"net_fee":            info.Receipt.NetFee,
				"energy_usage_total": info.Receipt.EnergyUsageTotal,
			}
		}

		if tx.RawData != nil && len(tx.RawData.Contract) > 0 {
			contract := tx.RawData.Contract[0]
			result["contract_type"] = contract.Type.String()
		}

		if len(info.ContractResult) > 0 {
			result["contract_result"] = hex.EncodeToString(info.ContractResult[0])
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleGetChainParameters(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		conn := pool.Client()
		nodeInfo, err := conn.GetNodeInfo()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_chain_parameters: %v", err)), nil
		}

		result := map[string]any{
			"node_info": map[string]any{
				"begin_sync_num": nodeInfo.BeginSyncNum,
				"block":          nodeInfo.Block,
				"solidity_block": nodeInfo.SolidityBlock,
			},
		}

		energyPrices, err := conn.GetEnergyPriceHistory()
		if err == nil {
			result["energy_prices"] = energyPrices
		}

		bwPrices, err := conn.GetBandwidthPriceHistory()
		if err == nil {
			result["bandwidth_prices"] = bwPrices
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleGetEnergyPrices(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		conn := pool.Client()
		prices, err := conn.GetEnergyPriceHistory()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_energy_prices: %v", err)), nil
		}

		result := map[string]any{
			"prices": prices,
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleGetNetwork(pool *nodepool.Pool, network, _ string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		conn, activeNode := pool.ClientAndNode()
		result := map[string]any{
			"network": network,
			"node":    activeNode,
		}

		block, err := conn.GetNowBlock()
		if err == nil && block.BlockHeader != nil && block.BlockHeader.RawData != nil {
			result["latest_block"] = block.BlockHeader.RawData.Number
			result["block_timestamp"] = block.BlockHeader.RawData.Timestamp
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleGetBandwidthPrices(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		conn := pool.Client()
		prices, err := conn.GetBandwidthPriceHistory()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_bandwidth_prices: %v", err)), nil
		}

		result := map[string]any{
			"prices": prices,
		}

		return mcp.NewToolResultJSON(result)
	}
}

// normalizeResult fixes the TRON protocol typo "SUCESS" → "SUCCESS".
func normalizeResult(s string) string {
	return strings.ReplaceAll(s, "SUCESS", "SUCCESS")
}
