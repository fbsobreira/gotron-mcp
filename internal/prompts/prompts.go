package prompts

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterPrompts registers all MCP prompts for common TRON workflows.
func RegisterPrompts(s *server.MCPServer) {
	s.AddPrompt(
		mcp.NewPrompt("account-overview",
			mcp.WithPromptDescription("Get a complete overview of a TRON account: balance, resources, permissions, and delegations"),
			mcp.WithArgument("address", mcp.RequiredArgument(), mcp.ArgumentDescription("TRON base58 address (starts with T)")),
		),
		handleAccountOverview,
	)

	s.AddPrompt(
		mcp.NewPrompt("transfer-checklist",
			mcp.WithPromptDescription("Pre-transfer checklist: verify balances, estimate costs, and check permissions before sending TRX or TRC20"),
			mcp.WithArgument("from", mcp.RequiredArgument(), mcp.ArgumentDescription("Sender TRON address")),
			mcp.WithArgument("to", mcp.RequiredArgument(), mcp.ArgumentDescription("Recipient TRON address")),
			mcp.WithArgument("amount", mcp.RequiredArgument(), mcp.ArgumentDescription("Amount to send (e.g., '100')")),
			mcp.WithArgument("token", mcp.ArgumentDescription("TRC20 contract address (leave empty for TRX)")),
		),
		handleTransferChecklist,
	)

	s.AddPrompt(
		mcp.NewPrompt("staking-status",
			mcp.WithPromptDescription("Check staking status: frozen resources, voting power, delegations, and delegatable amounts"),
			mcp.WithArgument("address", mcp.RequiredArgument(), mcp.ArgumentDescription("TRON base58 address (starts with T)")),
		),
		handleStakingStatus,
	)

	s.AddPrompt(
		mcp.NewPrompt("contract-interaction",
			mcp.WithPromptDescription("Explore a smart contract: get ABI, list methods, and call read-only functions"),
			mcp.WithArgument("contract_address", mcp.RequiredArgument(), mcp.ArgumentDescription("Smart contract address")),
			mcp.WithArgument("caller", mcp.ArgumentDescription("Caller address for simulations (optional)")),
		),
		handleContractInteraction,
	)
}

func handleAccountOverview(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	addr := req.Params.Arguments["address"]
	return &mcp.GetPromptResult{
		Description: "Complete TRON account overview",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`Give me a complete overview of the TRON account %s. Use these tools in order:

1. get_account — show TRX balance and account type
2. get_account_resources — show energy and bandwidth usage/limits
3. get_account_permissions — show permission structure (owner, active, witness)
4. get_delegated_resources — show any resource delegations to/from this account

Summarize the findings in a clear format. Flag anything notable (multi-sig setup, low resources, active delegations).`, addr),
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

	var transferType string
	if token != "" {
		transferType = fmt.Sprintf("TRC20 token %s", token)
	} else {
		transferType = "TRX"
	}

	return &mcp.GetPromptResult{
		Description: "Pre-transfer verification checklist",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`I want to transfer %s %s from %s to %s. Run this checklist before building the transaction:

1. get_account on the sender (%s) — verify sufficient balance
2. get_account_resources on the sender — check bandwidth/energy availability
3. validate_address on the recipient (%s) — confirm it's a valid address
4. get_account on the recipient — check if the account is activated
5. If TRC20: get_trc20_token_info on %s — verify token contract and decimals
6. If TRC20: estimate_energy — estimate the energy cost

Report: sender balance, available resources, estimated cost, and any warnings (insufficient balance, unactivated recipient, low energy).`, amount, transferType, from, to, from, to, token),
				},
			},
		},
	}, nil
}

func handleStakingStatus(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	addr := req.Params.Arguments["address"]
	return &mcp.GetPromptResult{
		Description: "Staking and delegation status",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`Check the complete staking status for %s:

1. get_account — show frozen balances and tronPower (voting power)
2. get_account_resources — show energy/bandwidth from staking
3. get_delegated_resources — show delegations to/from this account
4. get_delegatable_amount with resource ENERGY — how much energy can still be delegated
5. get_delegatable_amount with resource BANDWIDTH — how much bandwidth can still be delegated

Summarize: total staked, voting power, resource usage, active delegations, and remaining delegatable amounts.`, addr),
				},
			},
		},
	}, nil
}

func handleContractInteraction(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	contract := req.Params.Arguments["contract_address"]
	caller := req.Params.Arguments["caller"]
	if caller == "" {
		caller = "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb" // zero address for read-only
	}

	return &mcp.GetPromptResult{
		Description: "Smart contract exploration",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`Explore the smart contract at %s:

1. get_contract_abi — get the contract ABI (with proxy resolution)
2. list_contract_methods — show a human-readable list of all methods
3. If it's a token contract, try: get_trc20_token_info — get name, symbol, decimals, total supply

Use %s as the caller address for any read-only calls.

Show the contract's methods grouped by type (read-only vs write) and highlight the most commonly used ones.`, contract, caller),
				},
			},
		},
	}, nil
}
