package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterProposalTools registers the list_proposals tool.
func RegisterProposalTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("list_proposals",
			mcp.WithDescription("List governance proposals on the TRON network with pagination support. Returns newest first by default."),
			mcp.WithNumber("limit", mcp.Description("Max proposals to return (default: 10)")),
			mcp.WithNumber("offset", mcp.Description("Skip first N proposals (default: 0, for pagination)")),
			mcp.WithString("order", mcp.Description("Sort order by proposal ID: 'desc' (default, newest first) or 'asc' (oldest first)")),
		),
		handleListProposals(pool),
	)
}

func handleListProposals(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		limit := req.GetInt("limit", 10)
		offset := req.GetInt("offset", 0)
		order := req.GetString("order", "desc")

		conn := pool.Client()
		proposals, err := conn.ProposalsListCtx(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list_proposals: %v", err)), nil
		}

		// Sort by proposal ID
		items := proposals.Proposals
		if order == "desc" {
			slices.Reverse(items)
		}

		var list []map[string]any
		for _, p := range items {
			proposerAddr := address.HexToAddress(hex.EncodeToString(p.ProposerAddress))

			var approvals []string
			for _, a := range p.Approvals {
				addr := address.HexToAddress(hex.EncodeToString(a))
				approvals = append(approvals, addr.String())
			}

			list = append(list, map[string]any{
				"proposal_id":     p.ProposalId,
				"proposer":        proposerAddr.String(),
				"parameters":      p.Parameters,
				"expiration_time": p.ExpirationTime,
				"create_time":     p.CreateTime,
				"approvals":       approvals,
				"state":           p.State.String(),
			})
		}

		// Apply pagination
		total := len(list)
		offset = min(offset, total)
		end := min(offset+limit, total)
		page := list[offset:end]

		result := map[string]any{
			"proposals": page,
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
