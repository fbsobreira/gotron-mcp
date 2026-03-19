package resources

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

//go:embed knowledge/tron-overview.md
var tronOverview string

//go:embed knowledge/topics/accounts.md
var topicAccounts string

//go:embed knowledge/topics/tokens.md
var topicTokens string

//go:embed knowledge/topics/transfers.md
var topicTransfers string

//go:embed knowledge/topics/staking.md
var topicStaking string

//go:embed knowledge/topics/contracts.md
var topicContracts string

//go:embed knowledge/topics/governance.md
var topicGovernance string

//go:embed knowledge/topics/blocks.md
var topicBlocks string

//go:embed knowledge/topics/sdk.md
var topicSDK string

var topics = map[string]struct {
	name    string
	desc    string
	content string
}{
	"accounts":   {"TRON Accounts & Addresses", "Account queries, address validation, balance lookups", topicAccounts},
	"tokens":     {"TRC20 Tokens", "Token metadata, balances, and TRC20 transfers", topicTokens},
	"transfers":  {"TRX Transfers & Transactions", "Sending TRX, transaction signing, broadcasting, and lookups", topicTransfers},
	"staking":    {"Staking & Resources", "Energy, bandwidth, freezing/unfreezing TRX, resource delegation", topicStaking},
	"contracts":  {"Smart Contracts", "Contract calls, ABI, energy estimation, parameter encoding", topicContracts},
	"governance": {"Governance & Voting", "Super representatives, voting, proposals", topicGovernance},
	"blocks":     {"Blocks & Network", "Block queries, chain parameters, energy/bandwidth prices", topicBlocks},
	"sdk":        {"GoTRON High-Level SDK", "Fluent transaction builder, contract call builder, signer interface, TRC20 typed wrapper, receipt type", topicSDK},
}

// RegisterResources registers knowledge base resources on the MCP server.
func RegisterResources(s *server.MCPServer) {
	// Overview resource
	s.AddResource(
		mcp.NewResource(
			"gotron://knowledge/tron-overview",
			"TRON Blockchain Overview",
			mcp.WithResourceDescription("Overview of the TRON blockchain: addresses, resources (energy/bandwidth), staking, smart contracts, and governance"),
			mcp.WithMIMEType("text/markdown"),
		),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "gotron://knowledge/tron-overview",
					MIMEType: "text/markdown",
					Text:     tronOverview,
				},
			}, nil
		},
	)

	// Individual topic resources
	for slug, topic := range topics {
		uri := fmt.Sprintf("gotron://knowledge/topics/%s", slug)
		content := topic.content
		s.AddResource(
			mcp.NewResource(
				uri,
				topic.name,
				mcp.WithResourceDescription(topic.desc),
				mcp.WithMIMEType("text/markdown"),
			),
			func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      req.Params.URI,
						MIMEType: "text/markdown",
						Text:     content,
					},
				}, nil
			},
		)
	}

	// Topic lookup template
	s.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"gotron://knowledge/topics/{topic}",
			"TRON Knowledge Base Topic",
			mcp.WithTemplateDescription("Look up a specific TRON topic. Available topics: accounts, tokens, transfers, staking, contracts, governance, blocks, sdk"),
			mcp.WithTemplateMIMEType("text/markdown"),
		),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			uri := req.Params.URI
			slug := uri[strings.LastIndex(uri, "/")+1:]

			topic, ok := topics[slug]
			if !ok {
				available := make([]string, 0, len(topics))
				for k := range topics {
					available = append(available, k)
				}
				return nil, fmt.Errorf("unknown topic %q, available: %s", slug, strings.Join(available, ", "))
			}

			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      uri,
					MIMEType: "text/markdown",
					Text:     topic.content,
				},
			}, nil
		},
	)
}
