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
}
