package tools

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterProposalTools registers the list_proposals tool.
func RegisterProposalTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("list_proposals",
			mcp.WithDescription("List all governance proposals on the TRON network"),
		),
		handleListProposals(pool),
	)
}

func handleListProposals(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		conn := pool.Client()
		proposals, err := conn.ProposalsList()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list_proposals: %v", err)), nil
		}

		var list []map[string]any
		for _, p := range proposals.Proposals {
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

		result := map[string]any{
			"proposals": list,
			"count":     len(list),
		}

		return mcp.NewToolResultJSON(result)
	}
}
