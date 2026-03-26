package prompts

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterPrompts registers pre-built conversation starters for common workflows.
func RegisterPrompts(s *server.MCPServer) {
	s.AddPrompt(mcp.NewPrompt("account-overview",
		mcp.WithPromptDescription("Get a complete overview of a TRON account: balance, resources, permissions, and delegations"),
		mcp.WithArgument("address",
			mcp.ArgumentDescription("TRON account address (base58, starts with T)"),
			mcp.RequiredArgument(),
		),
	), handleAccountOverview)

	s.AddPrompt(mcp.NewPrompt("transfer-checklist",
		mcp.WithPromptDescription("Pre-flight checks before sending TRX or TRC20 tokens: balance, resources, address validation"),
		mcp.WithArgument("from",
			mcp.ArgumentDescription("Sender address or wallet name"),
			mcp.RequiredArgument(),
		),
		mcp.WithArgument("to",
			mcp.ArgumentDescription("Recipient address (base58, starts with T)"),
			mcp.RequiredArgument(),
		),
		mcp.WithArgument("amount",
			mcp.ArgumentDescription("Amount to send (e.g., '100.5')"),
			mcp.RequiredArgument(),
		),
		mcp.WithArgument("token",
			mcp.ArgumentDescription("Token to send: 'TRX' (default) or TRC20 contract address"),
		),
	), handleTransferChecklist)

	s.AddPrompt(mcp.NewPrompt("transaction-explain",
		mcp.WithPromptDescription("Look up a transaction and explain what it did in plain language"),
		mcp.WithArgument("txid",
			mcp.ArgumentDescription("Transaction ID (64-character hex hash)"),
			mcp.RequiredArgument(),
		),
	), handleTransactionExplain)

	s.AddPrompt(mcp.NewPrompt("staking-status",
		mcp.WithPromptDescription("Show the complete staking position: frozen TRX, delegations, votes, and pending unfreezes"),
		mcp.WithArgument("address",
			mcp.ArgumentDescription("TRON account address (base58, starts with T)"),
			mcp.RequiredArgument(),
		),
	), handleStakingStatus)
}

func handleAccountOverview(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	address := req.Params.Arguments["address"]
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}

	return &mcp.GetPromptResult{
		Description: "Complete TRON account overview",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(
						"Give me a complete overview of the TRON account %s. "+
							"Include: TRX balance, token balances, energy and bandwidth "+
							"(used/available/limit), account permissions (owner, active, "+
							"any multi-sig setup), and delegated resources (incoming and outgoing).",
						address,
					),
				},
			},
		},
	}, nil
}

func handleTransferChecklist(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	from := req.Params.Arguments["from"]
	to := req.Params.Arguments["to"]
	amount := req.Params.Arguments["amount"]
	token := req.Params.Arguments["token"]

	if from == "" || to == "" || amount == "" {
		return nil, fmt.Errorf("from, to, and amount are required")
	}
	if token == "" {
		token = "TRX"
	}

	return &mcp.GetPromptResult{
		Description: "Pre-flight transfer checklist",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(
						"I want to transfer %s %s from %s to %s. "+
							"Before building the transaction: validate both addresses, "+
							"check the sender's balance is sufficient, estimate the energy "+
							"and bandwidth cost, and verify the sender has enough resources "+
							"to cover fees. Report any issues. Only build the transfer if "+
							"everything checks out.",
						amount, token, from, to,
					),
				},
			},
		},
	}, nil
}

func handleTransactionExplain(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	txid := req.Params.Arguments["txid"]
	if txid == "" {
		return nil, fmt.Errorf("txid is required")
	}

	return &mcp.GetPromptResult{
		Description: "Transaction explanation",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(
						"Look up transaction %s and explain what it did. "+
							"Include: the contract type, sender and recipient, amount "+
							"transferred, success or failure status, and fee breakdown "+
							"(energy used, bandwidth used, TRX burned). Explain in plain language.",
						txid,
					),
				},
			},
		},
	}, nil
}

func handleStakingStatus(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	address := req.Params.Arguments["address"]
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}

	return &mcp.GetPromptResult{
		Description: "Staking position overview",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(
						"Show me the complete staking position for %s. "+
							"Include: frozen/staked TRX balance, energy and bandwidth from "+
							"staking, resources delegated to others, resources received from "+
							"others, delegatable amounts remaining, any pending unfreezes, "+
							"and current vote allocation if any.",
						address,
					),
				},
			},
		},
	}, nil
}
