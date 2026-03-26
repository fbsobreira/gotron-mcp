package tools

import (
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/trongrid"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
)

func TestRegisterAllTools(t *testing.T) {
	s := server.NewMCPServer("test", "1.0")
	pool := newMockPool(t, &mockWalletServer{})

	RegisterAccountTools(s, pool)
	RegisterAddressTools(s)
	RegisterBlockTools(s, pool)
	RegisterContractReadTools(s, pool)
	RegisterContractWriteTools(s, pool)
	RegisterNetworkTools(s, pool, "mainnet", "mock:50051")
	RegisterProposalTools(s, pool)
	RegisterResourceTools(s, pool)
	wm := newTestWalletManager(t)
	RegisterWalletTools(s, wm)
	RegisterSignTools(s, pool, wm, nil)
	trc20Cache := RegisterTokenTools(s, pool)
	RegisterTransferTools(s, pool, trc20Cache)
	RegisterWitnessReadTools(s, pool)
	RegisterWitnessWriteTools(s, pool)
	RegisterHistoryTools(s, &mockHistoryClient{
		txResp:    &trongrid.Response[trongrid.Transaction]{},
		trc20Resp: &trongrid.Response[trongrid.TRC20Transfer]{},
		eventResp: &trongrid.Response[trongrid.ContractEvent]{},
	})

	tools := s.ListTools()

	// Expected: 2 account + 1 address + 1 block + 5 contract read + 1 contract write +
	// 8 network (5 existing + 3 pending pool) + 2 proposal (list + get) + 5 resource +
	// 4 sign (sign_transaction, sign_and_broadcast, sign_and_confirm, broadcast_transaction) +
	// 1 policy (get_wallet_policy) +
	// 2 wallet (create_wallet, list_wallets) +
	// 3 token + 2 transfer + 1 witness read + 1 witness write + 3 history (TronGrid REST) = 42
	const expectedToolCount = 42
	assert.Len(t, tools, expectedToolCount, "registered tool count")

	representative := []string{
		"get_account",
		"validate_address",
		"get_block",
		"trigger_constant_contract",
		"trigger_contract",
		"get_network",
		"get_transaction",
		"list_proposals",
		"get_proposal",
		"freeze_balance",
		"sign_transaction",
		"sign_and_broadcast",
		"sign_and_confirm",
		"broadcast_transaction",
		"create_wallet",
		"list_wallets",
		"get_trc20_balance",
		"transfer_trx",
		"transfer_trc20",
		"list_witnesses",
		"vote_witness",
		"get_pending_transactions",
		"is_transaction_pending",
		"get_pending_by_address",
		"estimate_trc20_energy",
		"get_transaction_history",
		"get_trc20_transfers",
		"get_contract_events",
		"delegate_resource",
		"undelegate_resource",
		"withdraw_expire_unfreeze",
	}
	for _, name := range representative {
		assert.NotNil(t, tools[name], "expected tool %q to be registered", name)
	}
}
