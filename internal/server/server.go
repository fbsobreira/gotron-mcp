package server

import (
	"log"

	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/config"
	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/policy"
	"github.com/fbsobreira/gotron-mcp/internal/resources"
	"github.com/fbsobreira/gotron-mcp/internal/tools"
	"github.com/fbsobreira/gotron-mcp/internal/trongrid"
	"github.com/fbsobreira/gotron-mcp/internal/version"
	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/standards/trc20"
	"github.com/mark3labs/mcp-go/server"
)

// New creates a configured MCP server with tools registered based on mode.
// It returns the MCP server, an optional wallet manager, and an optional policy engine.
// The caller should defer wm.Close() and pe.Close() when non-nil.
func New(cfg *config.Config, pool *nodepool.Pool) (*server.MCPServer, *wallet.Manager, *policy.Engine) {
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
- Call read-only smart contract methods with decoded results (trigger_constant_contract). Supports pre-packed calldata, call_value for payable simulations, and TRC10 token_value
- Get contract ABI with automatic proxy resolution (get_contract_abi)
- Human-readable contract method listing (list_contract_methods)
- Decode ABI-encoded output or revert reasons from contract calls (decode_abi_output)
- Estimate energy costs (estimate_energy). Automatically falls back to secondary node if primary does not support this RPC
- Validate and convert addresses between base58, hex, and Ethereum 0x formats (validate_address)
- Network info, chain parameters, energy/bandwidth prices (get_network, get_chain_parameters)
- Transaction lookup (get_transaction)
- Transaction history, TRC20 transfers, contract events via TronGrid REST API (get_transaction_history, get_trc20_transfers, get_contract_events)
- Comprehensive account overview in one call (analyze_account)
- Transfer cost estimation with energy/bandwidth breakdown (analyze_transfer_cost)
- TRC20 approval safety: check allowances and revoke approvals (get_trc20_allowance, revoke_approval)
- Account permissions and multi-sig structure (get_account_permissions)
- Resource delegation info and delegatable amounts (get_delegated_resources, get_delegatable_amount)
- Governance: list witnesses, proposals (list_witnesses, list_proposals)
- Build unsigned transfer transactions (transfer_trx, transfer_trc20)
- Staking operations (freeze_balance, unfreeze_balance, withdraw_expire_unfreeze)
- Resource delegation (delegate_resource, undelegate_resource)
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
	trc20Cache := tools.RegisterTokenTools(s, pool)
	tools.RegisterContractReadTools(s, pool)
	tools.RegisterNetworkTools(s, pool, cfg.Network, cfg.Node)
	tools.RegisterAddressTools(s)
	tools.RegisterWitnessReadTools(s, pool)
	tools.RegisterProposalTools(s, pool)
	tools.RegisterDelegationTools(s, pool)
	tools.RegisterAnalyzeTools(s, pool)
	tools.RegisterCostTools(s, pool, trc20Cache)
	tools.RegisterApprovalReadTools(s, pool, trc20Cache)

	// TronGrid REST API tools (transaction history, TRC20 transfers, contract events)
	tgClient := trongrid.NewClient(cfg.Network, cfg.APIKey)
	tools.RegisterHistoryTools(s, tgClient)

	// Transaction builders — always available (return unsigned tx hex)
	tools.RegisterTransferTools(s, pool, trc20Cache)
	tools.RegisterResourceTools(s, pool)
	tools.RegisterWitnessWriteTools(s, pool)
	tools.RegisterContractWriteTools(s, pool)
	tools.RegisterApprovalWriteTools(s, pool, trc20Cache)

	// Sign/broadcast — local mode with wallet manager
	var wm *wallet.Manager
	var pe *policy.Engine
	if !cfg.IsHostedMode() && cfg.KeystoreDir != "" && cfg.KeystorePass != "" {
		var err error
		wm, err = wallet.NewManager(cfg.KeystoreDir, cfg.KeystorePass)
		if err != nil {
			log.Printf("warning: failed to create wallet manager: %v", err)
			wm = nil
		} else {
			tools.RegisterWalletTools(s, wm)

			pe = initPolicyEngine(cfg, pool)

			if cfg.RequirePolicy && pe == nil {
				log.Printf("warning: --require-policy is set but no policy loaded — sign tools disabled")
			} else {
				tools.RegisterSignTools(s, pool, wm, pe)
			}
		}
	}

	return s, wm, pe
}

// initPolicyEngine loads the policy config, resolves token decimals, and creates
// the bbolt-backed policy engine. Returns nil if policy is not configured or fails.
func initPolicyEngine(cfg *config.Config, pool *nodepool.Pool) *policy.Engine {
	policyCfg, err := policy.LoadConfig(cfg.PolicyConfig)
	if err != nil {
		log.Printf("warning: failed to load policy config: %v", err)
		return nil
	}
	if !policyCfg.Enabled || len(policyCfg.Wallets) == 0 {
		return nil
	}

	// Resolve token decimals from the network
	policyCfg.ResolveDecimals(func(contractAddr string) (int, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		token := trc20.New(pool.Client(), contractAddr)
		d, dErr := token.Decimals(ctx)
		return int(d), dErr
	})

	// Create state directory
	if err := os.MkdirAll(cfg.StateDir, 0700); err != nil {
		log.Printf("warning: failed to create state dir: %v", err)
		return nil
	}

	store, err := policy.NewStore(filepath.Join(cfg.StateDir, "state.db"))
	if err != nil {
		log.Printf("warning: failed to open policy store: %v", err)
		return nil
	}

	pe := policy.NewEngine(policyCfg, store)
	log.Printf("Policy engine loaded: %d wallet(s) configured", len(policyCfg.Wallets))
	return pe
}
