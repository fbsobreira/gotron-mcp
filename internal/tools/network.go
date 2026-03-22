package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-sdk/pkg/client/transaction"
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
			mcp.WithString("transaction_id", mcp.Required(), mcp.Description("Transaction hash / txid (64-char hex string)")),
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

	s.AddTool(
		mcp.NewTool("get_pending_transactions",
			mcp.WithDescription("List pending transaction IDs and pool size from the mempool"),
			mcp.WithNumber("limit", mcp.Description("Max transaction IDs to return (default: 10)")),
			mcp.WithNumber("offset", mcp.Description("Skip first N transaction IDs (default: 0, for pagination)")),
		),
		handleGetPendingTransactions(pool),
	)

	s.AddTool(
		mcp.NewTool("is_transaction_pending",
			mcp.WithDescription("Check if a specific transaction is still in the pending pool (mempool)"),
			mcp.WithString("transaction_id", mcp.Required(), mcp.Description("Transaction hash / txid (64-char hex string)")),
		),
		handleIsTransactionPending(pool),
	)

	s.AddTool(
		mcp.NewTool("get_pending_by_address",
			mcp.WithDescription("Get pending transactions for a specific address from the mempool"),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON address (base58, starts with T)")),
		),
		handleGetPendingByAddress(pool),
	)
}

func handleGetTransaction(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		txID := req.GetString("transaction_id", "")
		if txID == "" {
			return mcp.NewToolResultError("transaction_id is required"), nil
		}

		tx, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*core.Transaction, error) {
			return pool.Client().GetTransactionByIDCtx(ctx, txID)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_transaction: %v", err)), nil
		}

		info, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*core.TransactionInfo, error) {
			return pool.Client().GetTransactionInfoByIDCtx(ctx, txID)
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

			if decoded, err := transaction.DecodeContractData(tx); err == nil {
				result["contract_data"] = decoded.Fields
			}
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
		nodeInfo, err := conn.GetNodeInfoCtx(ctx)
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

		energyPrices, err := conn.GetEnergyPriceHistoryCtx(ctx)
		if err == nil {
			result["energy_prices"] = energyPrices
		}

		bwPrices, err := conn.GetBandwidthPriceHistoryCtx(ctx)
		if err == nil {
			result["bandwidth_prices"] = bwPrices
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleGetEnergyPrices(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		conn := pool.Client()
		prices, err := conn.GetEnergyPriceHistoryCtx(ctx)
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

		block, err := conn.GetNowBlockCtx(ctx)
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
		prices, err := conn.GetBandwidthPriceHistoryCtx(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_bandwidth_prices: %v", err)), nil
		}

		result := map[string]any{
			"prices": prices,
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleGetPendingTransactions(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		limit := req.GetInt("limit", 10)
		offset := req.GetInt("offset", 0)
		if limit <= 0 {
			limit = 10
		}
		if offset < 0 {
			offset = 0
		}

		conn := pool.Client()

		size, err := conn.GetPendingSizeCtx(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_pending_transactions: failed to get pool size: %v", err)), nil
		}

		list, err := conn.GetTransactionListFromPendingCtx(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_pending_transactions: failed to list pending: %v", err)), nil
		}

		txIDs := list.GetTxId()
		if txIDs == nil {
			txIDs = []string{}
		}

		// Apply pagination
		total := len(txIDs)
		if offset > total {
			offset = total
		}
		remaining := total - offset
		if limit > remaining {
			limit = remaining
		}
		page := txIDs[offset : offset+limit]

		result := map[string]any{
			"pool_size":       size.GetNum(),
			"transaction_ids": page,
			"total":           total,
			"returned":        len(page),
		}
		if offset+limit < total {
			result["has_more"] = true
			result["next_offset"] = offset + limit
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleIsTransactionPending(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		txID := req.GetString("transaction_id", "")
		if txID == "" {
			return mcp.NewToolResultError("transaction_id is required"), nil
		}

		conn := pool.Client()
		pending, err := conn.IsTransactionPendingCtx(ctx, txID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("is_transaction_pending: %v", err)), nil
		}

		result := map[string]any{
			"transaction_id": txID,
			"pending":        pending,
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleGetPendingByAddress(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid address: %v", err)), nil
		}

		conn := pool.Client()
		txs, err := conn.GetPendingTransactionsByAddressCtx(ctx, addr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_pending_by_address: %v", err)), nil
		}

		decoded := make([]map[string]any, 0, len(txs))
		for _, tx := range txs {
			entry := map[string]any{}
			if tx.RawData != nil {
				if rawBytes, err := proto.Marshal(tx.RawData); err == nil {
					h := sha256.Sum256(rawBytes)
					entry["transaction_id"] = hex.EncodeToString(h[:])
				}
				if len(tx.RawData.Contract) > 0 {
					entry["contract_type"] = tx.RawData.Contract[0].Type.String()
				}
			}
			if cd, err := transaction.DecodeContractData(tx); err == nil {
				entry["contract_data"] = cd.Fields
			}
			decoded = append(decoded, entry)
		}

		result := map[string]any{
			"address":      addr,
			"count":        len(txs),
			"transactions": decoded,
		}

		return mcp.NewToolResultJSON(result)
	}
}

// normalizeResult fixes the TRON protocol typo "SUCESS" → "SUCCESS".
func normalizeResult(s string) string {
	return strings.ReplaceAll(s, "SUCESS", "SUCCESS")
}
