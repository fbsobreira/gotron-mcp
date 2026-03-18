package tools

import (
	"context"
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

	GetAccountFunc             func(context.Context, *core.Account) (*core.Account, error)
	GetAccountResourceFunc     func(context.Context, *core.Account) (*api.AccountResourceMessage, error)
	GetNowBlock2Func           func(context.Context, *api.EmptyMessage) (*api.BlockExtention, error)
	GetBlockByNum2Func         func(context.Context, *api.NumberMessage) (*api.BlockExtention, error)
	GetTransactionByIdFunc     func(context.Context, *api.BytesMessage) (*core.Transaction, error)
	GetTransactionInfoByIdFunc func(context.Context, *api.BytesMessage) (*core.TransactionInfo, error)
	GetNodeInfoFunc            func(context.Context, *api.EmptyMessage) (*core.NodeInfo, error)
	GetEnergyPricesFunc        func(context.Context, *api.EmptyMessage) (*api.PricesResponseMessage, error)
	GetBandwidthPricesFunc     func(context.Context, *api.EmptyMessage) (*api.PricesResponseMessage, error)
	ListWitnessesFunc          func(context.Context, *api.EmptyMessage) (*api.WitnessList, error)
	ListProposalsFunc          func(context.Context, *api.EmptyMessage) (*api.ProposalList, error)
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
