package tools

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterBlockTools registers the get_block tool.
func RegisterBlockTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("get_block",
			mcp.WithDescription("Get a TRON block by number or latest. Use include_transactions to get transaction details with pagination."),
			mcp.WithNumber("block_number", mcp.Description("Block number (omit for latest)")),
			mcp.WithBoolean("include_transactions", mcp.Description("Include transaction IDs and types (default: false)")),
			mcp.WithNumber("limit", mcp.Description("Max transactions to return when include_transactions is true (default: 50)")),
			mcp.WithNumber("offset", mcp.Description("Skip first N transactions (default: 0, for pagination)")),
			mcp.WithString("transaction_type", mcp.Description("Filter transactions by type (e.g., 'TransferContract', 'TriggerSmartContract')")),
		),
		handleGetBlock(pool),
	)
}

func handleGetBlock(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		blockNum := req.GetInt("block_number", -1)
		includeTxs := req.GetBool("include_transactions", false)
		limit := req.GetInt("limit", 50)
		offset := req.GetInt("offset", 0)
		txTypeFilter := req.GetString("transaction_type", "")
		grpc := pool.Client()

		var (
			block *api.BlockExtention
			err   error
		)

		if blockNum < 0 {
			block, err = retry.Do(func() (*api.BlockExtention, error) {
				return grpc.GetNowBlock()
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get_block: failed to get latest block: %v", err)), nil
			}
		} else {
			block, err = retry.Do(func() (*api.BlockExtention, error) {
				return grpc.GetBlockByNum(int64(blockNum))
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get_block: failed to get block %d: %v", blockNum, err)), nil
			}
		}

		result := map[string]any{
			"block_id":          hex.EncodeToString(block.Blockid),
			"transaction_count": len(block.Transactions),
		}

		if block.BlockHeader != nil && block.BlockHeader.RawData != nil {
			result["block_number"] = block.BlockHeader.RawData.Number
			result["timestamp"] = block.BlockHeader.RawData.Timestamp
			result["witness_address"] = client.BlockExtentionWitnessBase58(block)
		}

		if includeTxs && len(block.Transactions) > 0 {
			// Build type summary
			typeCounts := make(map[string]int)
			var filtered []*api.TransactionExtention
			for _, tx := range block.Transactions {
				txType := ""
				if tx.Transaction != nil && tx.Transaction.RawData != nil && len(tx.Transaction.RawData.Contract) > 0 {
					txType = tx.Transaction.RawData.Contract[0].Type.String()
				}
				typeCounts[txType]++
				if txTypeFilter == "" || txType == txTypeFilter {
					filtered = append(filtered, tx)
				}
			}

			result["transaction_types"] = typeCounts

			// Apply pagination
			total := len(filtered)
			if offset > total {
				offset = total
			}
			end := offset + limit
			if end > total {
				end = total
			}
			page := filtered[offset:end]

			var txs []map[string]any
			for _, tx := range page {
				txInfo := map[string]any{
					"txid": hex.EncodeToString(tx.Txid),
				}
				if tx.Transaction != nil && tx.Transaction.RawData != nil && len(tx.Transaction.RawData.Contract) > 0 {
					txInfo["type"] = tx.Transaction.RawData.Contract[0].Type.String()
				}
				txs = append(txs, txInfo)
			}
			result["transactions"] = txs
			result["transactions_returned"] = len(txs)
			result["transactions_filtered"] = total
			if end < total {
				result["has_more"] = true
				result["next_offset"] = end
			}
		}

		return mcp.NewToolResultJSON(result)
	}
}
