package tools

import (
	"testing"

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
	RegisterTokenTools(s, pool)
	RegisterTransferTools(s, pool)
	RegisterWitnessReadTools(s, pool)
	RegisterWitnessWriteTools(s, pool)

	tools := s.ListTools()

	// Expected: 2 account + 1 address + 1 block + 5 contract read + 1 contract write +
	// 8 network (5 existing + 3 pending pool) + 1 proposal + 2 resource + 2 sign + 2 token + 2 transfer +
	// 1 witness read + 1 witness write = 29
	const expectedToolCount = 29
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
	}
	for _, name := range representative {
		if tools[name] == nil {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}
