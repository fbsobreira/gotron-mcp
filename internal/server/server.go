package server

import (
	"github.com/fbsobreira/gotron-mcp/internal/config"
	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/resources"
	"github.com/fbsobreira/gotron-mcp/internal/tools"
	"github.com/fbsobreira/gotron-mcp/internal/trongrid"
	"github.com/fbsobreira/gotron-mcp/internal/version"
	"github.com/mark3labs/mcp-go/server"
)

// New creates a configured MCP server with tools registered based on mode.
func New(cfg *config.Config, pool *nodepool.Pool) *server.MCPServer {
	s := server.NewMCPServer(
		"gotron-mcp",
		version.Version,
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(false, false),
		server.WithInstructions(`GoTRON MCP server for TRON blockchain interaction.

Available capabilities:
- Query accounts, balances, and resources (get_account, get_account_resources)
- Query blocks (get_block). Pass include_transactions: true to get transaction IDs and types
- TRC20 token balances and metadata (get_trc20_balance, get_trc20_token_info)
- Call read-only smart contract methods with decoded results (trigger_constant_contract)
- Get contract ABI with automatic proxy resolution (get_contract_abi)
- Human-readable contract method listing (list_contract_methods)
- Decode ABI-encoded output or revert reasons from contract calls (decode_abi_output)
- Estimate energy costs (estimate_energy). Automatically falls back to secondary node if primary does not support this RPC
- Validate and convert addresses between base58 and hex formats (validate_address)
- Network info, chain parameters, energy/bandwidth prices (get_network, get_chain_parameters)
- Transaction lookup (get_transaction)
- Transaction history, TRC20 transfers, contract events via TronGrid REST API (get_transaction_history, get_trc20_transfers, get_contract_events)
- Governance: list witnesses, proposals (list_witnesses, list_proposals)
- Build unsigned transfer transactions (transfer_trx, transfer_trc20)
- Staking operations (freeze_balance, unfreeze_balance)
- Vote for super representatives (vote_witness)
- Smart contract write calls (trigger_contract)
- Sign and broadcast transactions via keystore (sign_transaction, broadcast_transaction) [local mode + keystore]

Knowledge base resources available at gotron://knowledge/ for TRON concepts and SDK usage guides.`),
	)

	// Knowledge base resources
	resources.RegisterResources(s)

	// Always register read-only tools
	tools.RegisterAccountTools(s, pool)
	tools.RegisterBlockTools(s, pool)
	tools.RegisterTokenTools(s, pool)
	tools.RegisterContractReadTools(s, pool)
	tools.RegisterNetworkTools(s, pool, cfg.Network, cfg.Node)
	tools.RegisterAddressTools(s)
	tools.RegisterWitnessReadTools(s, pool)
	tools.RegisterProposalTools(s, pool)

	// TronGrid REST API tools (transaction history, TRC20 transfers, contract events)
	tgClient := trongrid.NewClient(cfg.Network, cfg.APIKey)
	tools.RegisterHistoryTools(s, tgClient)

	// Transaction builders — always available (return unsigned tx hex)
	tools.RegisterTransferTools(s, pool)
	tools.RegisterResourceTools(s, pool)
	tools.RegisterWitnessWriteTools(s, pool)
	tools.RegisterContractWriteTools(s, pool)

	// Sign/broadcast — local mode with keystore only
	if !cfg.IsHostedMode() && cfg.Keystore != "" {
		tools.RegisterSignTools(s, pool, cfg.Keystore)
	}

	return s
}
