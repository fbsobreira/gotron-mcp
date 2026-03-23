package tools

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// mockWalletServer embeds UnimplementedWalletServer so tests only override
// the methods they need.
type mockWalletServer struct {
	api.UnimplementedWalletServer

	GetAccountFunc                 func(context.Context, *core.Account) (*core.Account, error)
	GetAccountResourceFunc         func(context.Context, *core.Account) (*api.AccountResourceMessage, error)
	GetNowBlock2Func               func(context.Context, *api.EmptyMessage) (*api.BlockExtention, error)
	GetBlockByNum2Func             func(context.Context, *api.NumberMessage) (*api.BlockExtention, error)
	GetTransactionByIdFunc         func(context.Context, *api.BytesMessage) (*core.Transaction, error)
	GetTransactionInfoByIdFunc     func(context.Context, *api.BytesMessage) (*core.TransactionInfo, error)
	GetNodeInfoFunc                func(context.Context, *api.EmptyMessage) (*core.NodeInfo, error)
	GetEnergyPricesFunc            func(context.Context, *api.EmptyMessage) (*api.PricesResponseMessage, error)
	GetBandwidthPricesFunc         func(context.Context, *api.EmptyMessage) (*api.PricesResponseMessage, error)
	ListWitnessesFunc              func(context.Context, *api.EmptyMessage) (*api.WitnessList, error)
	ListProposalsFunc              func(context.Context, *api.EmptyMessage) (*api.ProposalList, error)
	TriggerConstantContractFunc    func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error)
	BroadcastTransactionFunc       func(context.Context, *core.Transaction) (*api.Return, error)
	FreezeBalanceV2Func            func(context.Context, *core.FreezeBalanceV2Contract) (*api.TransactionExtention, error)
	UnfreezeBalanceV2Func          func(context.Context, *core.UnfreezeBalanceV2Contract) (*api.TransactionExtention, error)
	CreateTransaction2Func         func(context.Context, *core.TransferContract) (*api.TransactionExtention, error)
	TriggerContractFunc            func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error)
	VoteWitnessAccount2Func        func(context.Context, *core.VoteWitnessContract) (*api.TransactionExtention, error)
	EstimateEnergyFunc             func(context.Context, *core.TriggerSmartContract) (*api.EstimateEnergyMessage, error)
	GetTransactionFromPendingFunc  func(context.Context, *api.BytesMessage) (*core.Transaction, error)
	GetTransactionListFromPendFunc func(context.Context, *api.EmptyMessage) (*api.TransactionIdList, error)
	GetPendingSizeFunc             func(context.Context, *api.EmptyMessage) (*api.NumberMessage, error)
	GetContractFunc                func(context.Context, *api.BytesMessage) (*core.SmartContract, error)
	DelegateResourceFunc           func(context.Context, *core.DelegateResourceContract) (*api.TransactionExtention, error)
	UnDelegateResourceFunc         func(context.Context, *core.UnDelegateResourceContract) (*api.TransactionExtention, error)
	WithdrawExpireUnfreezeFunc     func(context.Context, *core.WithdrawExpireUnfreezeContract) (*api.TransactionExtention, error)
	GetDelegatedResourceV2Func              func(context.Context, *api.DelegatedResourceMessage) (*api.DelegatedResourceList, error)
	GetDelegatedResourceAccountIndexV2Func func(context.Context, *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error)
	GetCanDelegatedMaxSizeFunc              func(context.Context, *api.CanDelegatedMaxSizeRequestMessage) (*api.CanDelegatedMaxSizeResponseMessage, error)
}

func (m *mockWalletServer) GetAccount(ctx context.Context, in *core.Account) (*core.Account, error) {
	if m.GetAccountFunc != nil {
		return m.GetAccountFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetAccount(ctx, in)
}

func (m *mockWalletServer) GetAccountResource(ctx context.Context, in *core.Account) (*api.AccountResourceMessage, error) {
	if m.GetAccountResourceFunc != nil {
		return m.GetAccountResourceFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetAccountResource(ctx, in)
}

func (m *mockWalletServer) GetNowBlock2(ctx context.Context, in *api.EmptyMessage) (*api.BlockExtention, error) {
	if m.GetNowBlock2Func != nil {
		return m.GetNowBlock2Func(ctx, in)
	}
	return m.UnimplementedWalletServer.GetNowBlock2(ctx, in)
}

func (m *mockWalletServer) GetBlockByNum2(ctx context.Context, in *api.NumberMessage) (*api.BlockExtention, error) {
	if m.GetBlockByNum2Func != nil {
		return m.GetBlockByNum2Func(ctx, in)
	}
	return m.UnimplementedWalletServer.GetBlockByNum2(ctx, in)
}

func (m *mockWalletServer) GetTransactionById(ctx context.Context, in *api.BytesMessage) (*core.Transaction, error) {
	if m.GetTransactionByIdFunc != nil {
		return m.GetTransactionByIdFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetTransactionById(ctx, in)
}

func (m *mockWalletServer) GetTransactionInfoById(ctx context.Context, in *api.BytesMessage) (*core.TransactionInfo, error) {
	if m.GetTransactionInfoByIdFunc != nil {
		return m.GetTransactionInfoByIdFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetTransactionInfoById(ctx, in)
}

func (m *mockWalletServer) GetNodeInfo(ctx context.Context, in *api.EmptyMessage) (*core.NodeInfo, error) {
	if m.GetNodeInfoFunc != nil {
		return m.GetNodeInfoFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetNodeInfo(ctx, in)
}

func (m *mockWalletServer) GetEnergyPrices(ctx context.Context, in *api.EmptyMessage) (*api.PricesResponseMessage, error) {
	if m.GetEnergyPricesFunc != nil {
		return m.GetEnergyPricesFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetEnergyPrices(ctx, in)
}

func (m *mockWalletServer) GetBandwidthPrices(ctx context.Context, in *api.EmptyMessage) (*api.PricesResponseMessage, error) {
	if m.GetBandwidthPricesFunc != nil {
		return m.GetBandwidthPricesFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetBandwidthPrices(ctx, in)
}

func (m *mockWalletServer) ListWitnesses(ctx context.Context, in *api.EmptyMessage) (*api.WitnessList, error) {
	if m.ListWitnessesFunc != nil {
		return m.ListWitnessesFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.ListWitnesses(ctx, in)
}

func (m *mockWalletServer) ListProposals(ctx context.Context, in *api.EmptyMessage) (*api.ProposalList, error) {
	if m.ListProposalsFunc != nil {
		return m.ListProposalsFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.ListProposals(ctx, in)
}

func (m *mockWalletServer) TriggerConstantContract(ctx context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
	if m.TriggerConstantContractFunc != nil {
		return m.TriggerConstantContractFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.TriggerConstantContract(ctx, in)
}

func (m *mockWalletServer) BroadcastTransaction(ctx context.Context, in *core.Transaction) (*api.Return, error) {
	if m.BroadcastTransactionFunc != nil {
		return m.BroadcastTransactionFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.BroadcastTransaction(ctx, in)
}

func (m *mockWalletServer) FreezeBalanceV2(ctx context.Context, in *core.FreezeBalanceV2Contract) (*api.TransactionExtention, error) {
	if m.FreezeBalanceV2Func != nil {
		return m.FreezeBalanceV2Func(ctx, in)
	}
	return m.UnimplementedWalletServer.FreezeBalanceV2(ctx, in)
}

func (m *mockWalletServer) UnfreezeBalanceV2(ctx context.Context, in *core.UnfreezeBalanceV2Contract) (*api.TransactionExtention, error) {
	if m.UnfreezeBalanceV2Func != nil {
		return m.UnfreezeBalanceV2Func(ctx, in)
	}
	return m.UnimplementedWalletServer.UnfreezeBalanceV2(ctx, in)
}

func (m *mockWalletServer) CreateTransaction2(ctx context.Context, in *core.TransferContract) (*api.TransactionExtention, error) {
	if m.CreateTransaction2Func != nil {
		return m.CreateTransaction2Func(ctx, in)
	}
	return m.UnimplementedWalletServer.CreateTransaction2(ctx, in)
}

func (m *mockWalletServer) TriggerContract(ctx context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
	if m.TriggerContractFunc != nil {
		return m.TriggerContractFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.TriggerContract(ctx, in)
}

func (m *mockWalletServer) VoteWitnessAccount2(ctx context.Context, in *core.VoteWitnessContract) (*api.TransactionExtention, error) {
	if m.VoteWitnessAccount2Func != nil {
		return m.VoteWitnessAccount2Func(ctx, in)
	}
	return m.UnimplementedWalletServer.VoteWitnessAccount2(ctx, in)
}

func (m *mockWalletServer) EstimateEnergy(ctx context.Context, in *core.TriggerSmartContract) (*api.EstimateEnergyMessage, error) {
	if m.EstimateEnergyFunc != nil {
		return m.EstimateEnergyFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.EstimateEnergy(ctx, in)
}

func (m *mockWalletServer) GetTransactionFromPending(ctx context.Context, in *api.BytesMessage) (*core.Transaction, error) {
	if m.GetTransactionFromPendingFunc != nil {
		return m.GetTransactionFromPendingFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetTransactionFromPending(ctx, in)
}

func (m *mockWalletServer) GetTransactionListFromPending(ctx context.Context, in *api.EmptyMessage) (*api.TransactionIdList, error) {
	if m.GetTransactionListFromPendFunc != nil {
		return m.GetTransactionListFromPendFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetTransactionListFromPending(ctx, in)
}

func (m *mockWalletServer) GetPendingSize(ctx context.Context, in *api.EmptyMessage) (*api.NumberMessage, error) {
	if m.GetPendingSizeFunc != nil {
		return m.GetPendingSizeFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetPendingSize(ctx, in)
}

func (m *mockWalletServer) GetContract(ctx context.Context, in *api.BytesMessage) (*core.SmartContract, error) {
	if m.GetContractFunc != nil {
		return m.GetContractFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetContract(ctx, in)
}

func (m *mockWalletServer) DelegateResource(ctx context.Context, in *core.DelegateResourceContract) (*api.TransactionExtention, error) {
	if m.DelegateResourceFunc != nil {
		return m.DelegateResourceFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.DelegateResource(ctx, in)
}

func (m *mockWalletServer) UnDelegateResource(ctx context.Context, in *core.UnDelegateResourceContract) (*api.TransactionExtention, error) {
	if m.UnDelegateResourceFunc != nil {
		return m.UnDelegateResourceFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.UnDelegateResource(ctx, in)
}

func (m *mockWalletServer) WithdrawExpireUnfreeze(ctx context.Context, in *core.WithdrawExpireUnfreezeContract) (*api.TransactionExtention, error) {
	if m.WithdrawExpireUnfreezeFunc != nil {
		return m.WithdrawExpireUnfreezeFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.WithdrawExpireUnfreeze(ctx, in)
}

func (m *mockWalletServer) GetDelegatedResourceV2(ctx context.Context, in *api.DelegatedResourceMessage) (*api.DelegatedResourceList, error) {
	if m.GetDelegatedResourceV2Func != nil {
		return m.GetDelegatedResourceV2Func(ctx, in)
	}
	return m.UnimplementedWalletServer.GetDelegatedResourceV2(ctx, in)
}

func (m *mockWalletServer) GetDelegatedResourceAccountIndexV2(ctx context.Context, in *api.BytesMessage) (*core.DelegatedResourceAccountIndex, error) {
	if m.GetDelegatedResourceAccountIndexV2Func != nil {
		return m.GetDelegatedResourceAccountIndexV2Func(ctx, in)
	}
	return m.UnimplementedWalletServer.GetDelegatedResourceAccountIndexV2(ctx, in)
}

func (m *mockWalletServer) GetCanDelegatedMaxSize(ctx context.Context, in *api.CanDelegatedMaxSizeRequestMessage) (*api.CanDelegatedMaxSizeResponseMessage, error) {
	if m.GetCanDelegatedMaxSizeFunc != nil {
		return m.GetCanDelegatedMaxSizeFunc(ctx, in)
	}
	return m.UnimplementedWalletServer.GetCanDelegatedMaxSize(ctx, in)
}

// newMockClient creates a GrpcClient connected to an in-memory gRPC server.
func newMockClient(t *testing.T, mock *mockWalletServer) *client.GrpcClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	api.RegisterWalletServer(srv, mock)

	go func() { _ = srv.Serve(lis) }()

	t.Cleanup(func() {
		srv.GracefulStop()
		_ = lis.Close()
	})

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	c := client.NewGrpcClient("bufconn")
	c.Conn = conn
	c.Client = api.NewWalletClient(conn)
	return c
}

// newMockPool creates a nodepool.Pool backed by a mock gRPC server.
func newMockPool(t *testing.T, mock *mockWalletServer) *nodepool.Pool {
	t.Helper()
	c := newMockClient(t, mock)
	return nodepool.NewFromClient(c, "mock:50051")
}

// callTool is a helper to invoke a tool handler with the given arguments.
func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error (not tool error): %v", err)
	}
	return result
}

// extractJSON parses a tool result's first TextContent as a JSON map.
func extractJSON(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty result content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &data); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	return data
}
