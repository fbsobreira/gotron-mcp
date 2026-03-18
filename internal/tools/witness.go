package tools

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

// RegisterWitnessReadTools registers list_witnesses.
func RegisterWitnessReadTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("list_witnesses",
			mcp.WithDescription("List super representatives (witnesses) on the TRON network with pagination support."),
			mcp.WithNumber("limit", mcp.Description("Max witnesses to return (default: 10)")),
			mcp.WithNumber("offset", mcp.Description("Skip first N witnesses (default: 0, for pagination)")),
		),
		handleListWitnesses(pool),
	)
}

// RegisterWitnessWriteTools registers vote_witness (local mode only).
func RegisterWitnessWriteTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("vote_witness",
			mcp.WithDescription("Vote for super representatives. Returns unsigned transaction hex for signing. Requires staked TRX (1 TRX staked = 1 vote)."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Voter address (base58, starts with T)")),
			mcp.WithObject("votes", mcp.Required(), mcp.Description("Map of witness address to vote count, e.g., {\"TKSXDA...\": 100, \"TLyqz...\": 50}")),
		),
		handleVoteWitness(pool),
	)
}

func handleListWitnesses(pool *nodepool.Pool) server.ToolHandlerFunc {
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
		witnesses, err := conn.ListWitnessesCtx(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list_witnesses: %v", err)), nil
		}

		var list []map[string]any
		for _, w := range witnesses.Witnesses {
			addr := address.HexToAddress(hex.EncodeToString(w.Address))
			list = append(list, map[string]any{
				"address":          addr.String(),
				"vote_count":       w.VoteCount,
				"url":              w.Url,
				"total_produced":   w.TotalProduced,
				"total_missed":     w.TotalMissed,
				"latest_block_num": w.LatestBlockNum,
				"is_jobs":          w.IsJobs,
			})
		}

		// Apply pagination
		total := len(list)
		offset = min(offset, total)
		end := min(offset+limit, total)
		page := list[offset:end]

		result := map[string]any{
			"witnesses": page,
			"total":     total,
			"returned":  len(page),
		}
		if end < total {
			result["has_more"] = true
			result["next_offset"] = end
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleVoteWitness(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		conn := pool.Client()
		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}

		args := req.GetArguments()
		votesRaw, ok := args["votes"]
		if !ok {
			return mcp.NewToolResultError("votes parameter is required"), nil
		}

		votesMap, ok := votesRaw.(map[string]any)
		if !ok {
			return mcp.NewToolResultError("votes must be an object mapping witness addresses to vote counts"), nil
		}

		witnessVotes := make(map[string]int64)
		for addr, count := range votesMap {
			if err := validateAddress(addr); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid witness address %q: %v", addr, err)), nil
			}
			switch v := count.(type) {
			case float64:
				if v != float64(int64(v)) || v <= 0 {
					return mcp.NewToolResultError(fmt.Sprintf("invalid vote count for %s: must be a positive integer", addr)), nil
				}
				witnessVotes[addr] = int64(v)
			case int64:
				if v <= 0 {
					return mcp.NewToolResultError(fmt.Sprintf("invalid vote count for %s: must be a positive integer", addr)), nil
				}
				witnessVotes[addr] = v
			default:
				return mcp.NewToolResultError(fmt.Sprintf("invalid vote count for %s: must be a number", addr)), nil
			}
		}

		tx, err := conn.VoteWitnessAccountCtx(ctx, from, witnessVotes)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("vote_witness: %v", err)), nil
		}

		txBytes, err := proto.Marshal(tx.Transaction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("vote_witness: failed to serialize: %v", err)), nil
		}

		result := map[string]any{
			"transaction_hex": hex.EncodeToString(txBytes),
			"txid":            hex.EncodeToString(tx.Txid),
			"from":            from,
			"votes":           witnessVotes,
			"type":            "VoteWitnessContract",
		}

		return mcp.NewToolResultJSON(result)
	}
}
