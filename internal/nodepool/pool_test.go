package nodepool

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// newTestPool creates a pool with mock nodes for testing failover logic.
// The clients are not connected — only use for testing pool switching, not gRPC calls.
func newTestPool(primaryAddr, fallbackAddr string) *Pool {
	p := &Pool{}
	p.primary = &node{client: client.NewGrpcClient(primaryAddr), address: primaryAddr}
	p.active.Store(p.primary)
	if fallbackAddr != "" {
		p.fallback = &node{client: client.NewGrpcClient(fallbackAddr), address: fallbackAddr}
	}
	return p
}

func TestClient_ReturnsPrimary(t *testing.T) {
	p := newTestPool("primary:50051", "")
	assert.Equal(t, p.primary.client, p.Client(), "Client() should return primary client")
}

func TestActiveNode_ReturnsPrimaryAddress(t *testing.T) {
	p := newTestPool("primary:50051", "")
	assert.Equal(t, "primary:50051", p.ActiveNode())
}

func TestClientAndNode_ConsistentSnapshot(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	c, addr := p.ClientAndNode()
	assert.Equal(t, p.primary.client, c, "ClientAndNode() client should be primary")
	assert.Equal(t, "primary:50051", addr)
}

func TestFallbackClient_Nil(t *testing.T) {
	p := newTestPool("primary:50051", "")
	assert.Nil(t, p.FallbackClient(), "FallbackClient() should be nil when no fallback configured")
}

func TestFallbackClient_ReturnsFallback(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	assert.Equal(t, p.fallback.client, p.FallbackClient(), "FallbackClient() should return fallback client")
}

func TestFailover_SwitchesToFallback(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")

	switched := p.Failover()
	assert.True(t, switched, "Failover() should return true")
	assert.Equal(t, p.fallback.client, p.Client(), "Client() should return fallback after failover")
	assert.Equal(t, "fallback:50051", p.ActiveNode())
}

func TestFailover_NoFallback(t *testing.T) {
	p := newTestPool("primary:50051", "")

	switched := p.Failover()
	assert.False(t, switched, "Failover() should return false when no fallback")
	assert.Equal(t, p.primary.client, p.Client(), "Client() should still return primary")
}

func TestFailover_AlreadyOnFallback(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	p.Failover() // first failover

	switched := p.Failover()
	assert.False(t, switched, "Failover() should return false when already on fallback")
}

func TestRecover_SwitchesBackToPrimary(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	p.Failover()

	recovered := p.Recover()
	assert.True(t, recovered, "Recover() should return true")
	assert.Equal(t, p.primary.client, p.Client(), "Client() should return primary after recover")
	assert.Equal(t, "primary:50051", p.ActiveNode())
}

func TestRecover_AlreadyOnPrimary(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")

	recovered := p.Recover()
	assert.False(t, recovered, "Recover() should return false when already on primary")
}

func TestFailoverThenRecover_Cycle(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")

	// Start on primary
	require.Equal(t, "primary:50051", p.ActiveNode(), "should start on primary")

	// Failover to fallback
	p.Failover()
	require.Equal(t, "fallback:50051", p.ActiveNode(), "should be on fallback")

	// Recover to primary
	p.Recover()
	require.Equal(t, "primary:50051", p.ActiveNode(), "should be back on primary")

	// Failover again
	p.Failover()
	require.Equal(t, "fallback:50051", p.ActiveNode(), "should be on fallback again")
}

func TestClientAndNode_AfterFailover(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	p.Failover()

	c, addr := p.ClientAndNode()
	assert.Equal(t, p.fallback.client, c, "ClientAndNode() client should be fallback after failover")
	assert.Equal(t, "fallback:50051", addr)
}

func TestNewFromClient(t *testing.T) {
	c := client.NewGrpcClient("test:50051")
	p := NewFromClient(c, "test:50051")
	assert.Equal(t, c, p.Client(), "NewFromClient should return pool with provided client")
	assert.Equal(t, "test:50051", p.ActiveNode())
	assert.Nil(t, p.FallbackClient(), "NewFromClient should not have fallback")
}

func TestSetAPIKey(t *testing.T) {
	p := newTestPool("primary:50051", "")
	err := p.SetAPIKey("test-key")
	assert.NoError(t, err, "SetAPIKey()")
}

func TestSetAPIKey_WithFallback(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	err := p.SetAPIKey("test-key")
	assert.NoError(t, err, "SetAPIKey() with fallback")
}

func TestStop(t *testing.T) {
	p := newTestPool("primary:50051", "")
	// Stop should not panic on unconnected clients
	p.Stop()
}

func TestStop_WithFallback(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	p.Stop()
}

func TestConcurrentFailoverRecover(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				p.Failover()
			}
		}()
		go func() {
			defer wg.Done()
			for range iterations {
				p.Recover()
			}
		}()
	}
	wg.Wait()

	// After concurrent operations, pool must be in a valid state
	c, addr := p.ClientAndNode()
	assert.True(t, addr == "primary:50051" || addr == "fallback:50051", "ActiveNode() = %q, want primary or fallback", addr)
	assert.NotNil(t, c, "Client() should not be nil")
	if addr == "primary:50051" {
		assert.Equal(t, p.primary.client, c, "client/node mismatch: addr is primary but client is not")
	}
	if addr == "fallback:50051" {
		assert.Equal(t, p.fallback.client, c, "client/node mismatch: addr is fallback but client is not")
	}
}

const bufSize = 1024 * 1024

type mockWalletServer struct {
	api.UnimplementedWalletServer
	healthy bool
}

func (m *mockWalletServer) GetNowBlock2(_ context.Context, _ *api.EmptyMessage) (*api.BlockExtention, error) {
	if !m.healthy {
		return nil, context.DeadlineExceeded
	}
	return &api.BlockExtention{
		BlockHeader: &core.BlockHeader{
			RawData: &core.BlockHeaderRaw{Number: 12345},
		},
	}, nil
}

// newBufconnClient creates a gRPC client connected to a bufconn mock server.
func newBufconnClient(t *testing.T, mock *mockWalletServer) *client.GrpcClient {
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
	require.NoError(t, err, "failed to create mock client")
	t.Cleanup(func() { _ = conn.Close() })

	c := client.NewGrpcClient("bufconn")
	c.Conn = conn
	c.Client = api.NewWalletClient(conn)
	return c
}

// bufconnDialOpts returns gRPC dial options that connect to the given bufconn listener.
func bufconnDialOpts(lis *bufconn.Listener) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
}

// startMockServer creates a bufconn listener with a registered mock wallet server.
func startMockServer(t *testing.T, mock *mockWalletServer) *bufconn.Listener {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	api.RegisterWalletServer(srv, mock)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.GracefulStop()
		_ = lis.Close()
	})
	return lis
}

func TestNew_Success(t *testing.T) {
	lis := startMockServer(t, &mockWalletServer{healthy: true})
	p, err := New("passthrough:///bufconn", bufconnDialOpts(lis))
	require.NoError(t, err, "New()")
	assert.NotNil(t, p.Client(), "Client() should not be nil")
	assert.Equal(t, "passthrough:///bufconn", p.ActiveNode())
	p.Stop()
}

func TestAddFallback_Success(t *testing.T) {
	primaryLis := startMockServer(t, &mockWalletServer{healthy: true})
	fallbackLis := startMockServer(t, &mockWalletServer{healthy: true})

	p, err := New("passthrough:///primary", bufconnDialOpts(primaryLis))
	require.NoError(t, err, "New()")
	defer p.Stop()

	err = p.AddFallback("passthrough:///fallback", bufconnDialOpts(fallbackLis))
	require.NoError(t, err, "AddFallback()")
	assert.NotNil(t, p.FallbackClient(), "FallbackClient() should not be nil after AddFallback")
}

func TestCheckHealth_Healthy(t *testing.T) {
	c := newBufconnClient(t, &mockWalletServer{healthy: true})
	p := NewFromClient(c, "bufconn")
	assert.True(t, p.CheckHealth(), "CheckHealth() should return true when node is healthy")
}

func TestCheckHealth_Unhealthy_NoFallback(t *testing.T) {
	c := newBufconnClient(t, &mockWalletServer{healthy: false})
	p := NewFromClient(c, "bufconn")
	assert.False(t, p.CheckHealth(), "CheckHealth() should return false when node is unhealthy")
}

func TestCheckHealth_Unhealthy_WithFallback(t *testing.T) {
	primary := newBufconnClient(t, &mockWalletServer{healthy: false})
	fallback := newBufconnClient(t, &mockWalletServer{healthy: true})

	p := NewFromClient(primary, "primary")
	p.fallback = &node{client: fallback, address: "fallback"}

	assert.False(t, p.CheckHealth(), "CheckHealth() should return false when primary is unhealthy")
	// Should have failed over to fallback
	assert.Equal(t, "fallback", p.ActiveNode(), "should have failed over")
}

func TestCheckHealth_OnFallback_PrimaryRecovers(t *testing.T) {
	primary := newBufconnClient(t, &mockWalletServer{healthy: true})
	fallback := newBufconnClient(t, &mockWalletServer{healthy: true})

	p := NewFromClient(primary, "primary")
	p.fallback = &node{client: fallback, address: "fallback"}
	p.Failover() // move to fallback

	assert.True(t, p.CheckHealth(), "CheckHealth() should return true when fallback is healthy")
	// Should have recovered to primary since primary is also healthy
	assert.Equal(t, "primary", p.ActiveNode(), "should have recovered to primary")
}
