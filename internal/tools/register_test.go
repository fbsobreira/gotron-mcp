package tools

import (
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/trongrid"
	"github.com/mark3labs/mcp-go/server"
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
	RegisterSignTools(s, pool, t.TempDir())
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
	// 8 network (5 existing + 3 pending pool) + 1 proposal + 5 resource + 2 sign + 3 token + 2 transfer +
	// 1 witness read + 1 witness write + 3 history (TronGrid REST) = 36
	const expectedToolCount = 36
	if len(tools) != expectedToolCount {
		t.Errorf("registered tool count = %d, want %d", len(tools), expectedToolCount)
	}

	representative := []string{
		"get_account",
		"validate_address",
		"get_block",
		"trigger_constant_contract",
		"trigger_contract",
		"get_network",
		"get_transaction",
		"list_proposals",
		"freeze_balance",
		"sign_transaction",
		"broadcast_transaction",
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
		if tools[name] == nil {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}
