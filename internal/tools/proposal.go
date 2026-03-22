package tools

import (
	"context"
	"fmt"
	"slices"
	"sort"

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
			mcp.WithNumber("limit", mcp.Description("Max proposals to return (default: 5)")),
			mcp.WithNumber("offset", mcp.Description("Skip first N proposals (default: 0, for pagination)")),
			mcp.WithString("order", mcp.Description("Sort order by proposal ID: 'desc' (default, newest first) or 'asc' (oldest first)")),
		),
		handleListProposals(pool),
	)
}

func handleListProposals(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		limit := req.GetInt("limit", 5)
		offset := req.GetInt("offset", 0)
		order := req.GetString("order", "desc")
		if limit <= 0 {
			limit = 5
		}
		if offset < 0 {
			offset = 0
		}
		if order != "desc" && order != "asc" {
			return mcp.NewToolResultError("list_proposals: order must be 'asc' or 'desc'"), nil
		}

		conn := pool.Client()
		proposals, err := conn.ProposalsListCtx(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list_proposals: %v", err)), nil
		}

		// Sort by proposal ID (copy to avoid mutating SDK response)
		items := slices.Clone(proposals.Proposals)
		sort.SliceStable(items, func(i, j int) bool {
			if order == "desc" {
				return items[i].ProposalId > items[j].ProposalId
			}
			return items[i].ProposalId < items[j].ProposalId
		})

		var list []map[string]any
		for _, p := range items {
			proposerAddr := address.BytesToAddress(p.ProposerAddress)

			list = append(list, map[string]any{
				"proposal_id":     p.ProposalId,
				"proposer":        proposerAddr.String(),
				"parameters":      p.Parameters,
				"expiration_time": p.ExpirationTime,
				"create_time":     p.CreateTime,
				"approval_count":  len(p.Approvals),
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
