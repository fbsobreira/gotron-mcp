package tools

import (
	"context"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-sdk/pkg/pqc"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterPQTools registers the read-only TIP-899 post-quantum tools.
func RegisterPQTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("get_pq_capabilities",
			mcp.WithDescription("Report which TIP-899 post-quantum signature schemes the connected node enables and this build can sign (FN-DSA-512 needs the gotron-mcp-pq build). TIP-899 is a draft live only on Nile. can_sign reflects the node and this build; signing also needs local mode with a keystore."),
		),
		handleGetPQCapabilities(pool),
	)
}

func handleGetPQCapabilities(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		caps, err := pool.Client().GetPQCapabilitiesCtx(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_pq_capabilities: %v", err)), nil
		}

		buildFN := pqc.SchemeAvailable(core.PQScheme_FN_DSA_512) == nil
		buildML := pqc.SchemeAvailable(core.PQScheme_ML_DSA_44) == nil

		result := map[string]any{
			"node": map[string]any{
				"fn_dsa_512": caps.FNDSA512,
				"ml_dsa_44":  caps.MLDSA44,
			},
			"build": map[string]any{
				"fn_dsa_512": buildFN,
				"ml_dsa_44":  buildML,
			},
			"can_sign": map[string]any{
				"fn_dsa_512": caps.FNDSA512 && buildFN,
				"ml_dsa_44":  caps.MLDSA44 && buildML,
			},
		}
		switch {
		case !caps.FNDSA512 && !caps.MLDSA44:
			result["hint"] = "This node has no TIP-899 post-quantum support (mainnet today). Use a Nile node."
		case caps.FNDSA512 && !buildFN:
			result["hint"] = "This node allows FN-DSA-512, but this build cannot sign it; use the gotron-mcp-pq build, or build locally with liboqs installed: CGO_ENABLED=1 go build -tags falcon ./cmd/gotron-mcp."
		}

		return mcp.NewToolResultJSON(result)
	}
}
