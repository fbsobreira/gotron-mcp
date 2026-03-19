package nodepool

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
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
	if p.Client() != p.primary.client {
		t.Error("Client() should return primary client")
	}
}

func TestActiveNode_ReturnsPrimaryAddress(t *testing.T) {
	p := newTestPool("primary:50051", "")
	if got := p.ActiveNode(); got != "primary:50051" {
		t.Errorf("ActiveNode() = %q, want %q", got, "primary:50051")
	}
}

func TestClientAndNode_ConsistentSnapshot(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	c, addr := p.ClientAndNode()
	if c != p.primary.client {
		t.Error("ClientAndNode() client should be primary")
	}
	if addr != "primary:50051" {
		t.Errorf("ClientAndNode() addr = %q, want %q", addr, "primary:50051")
	}
}

func TestFallbackClient_Nil(t *testing.T) {
	p := newTestPool("primary:50051", "")
	if got := p.FallbackClient(); got != nil {
		t.Error("FallbackClient() should be nil when no fallback configured")
	}
}

func TestFallbackClient_ReturnsFallback(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	if p.FallbackClient() != p.fallback.client {
		t.Error("FallbackClient() should return fallback client")
	}
}

func TestFailover_SwitchesToFallback(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")

	switched := p.Failover()
	if !switched {
		t.Error("Failover() should return true")
	}
	if p.Client() != p.fallback.client {
		t.Error("Client() should return fallback after failover")
	}
	if got := p.ActiveNode(); got != "fallback:50051" {
		t.Errorf("ActiveNode() = %q, want %q", got, "fallback:50051")
	}
}

func TestFailover_NoFallback(t *testing.T) {
	p := newTestPool("primary:50051", "")

	switched := p.Failover()
	if switched {
		t.Error("Failover() should return false when no fallback")
	}
	if p.Client() != p.primary.client {
		t.Error("Client() should still return primary")
	}
}

func TestFailover_AlreadyOnFallback(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	p.Failover() // first failover

	switched := p.Failover()
	if switched {
		t.Error("Failover() should return false when already on fallback")
	}
}

func TestRecover_SwitchesBackToPrimary(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	p.Failover()

	recovered := p.Recover()
	if !recovered {
		t.Error("Recover() should return true")
	}
	if p.Client() != p.primary.client {
		t.Error("Client() should return primary after recover")
	}
	if got := p.ActiveNode(); got != "primary:50051" {
		t.Errorf("ActiveNode() = %q, want %q", got, "primary:50051")
	}
}

func TestRecover_AlreadyOnPrimary(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")

	recovered := p.Recover()
	if recovered {
		t.Error("Recover() should return false when already on primary")
	}
}

func TestFailoverThenRecover_Cycle(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")

	// Start on primary
	if p.ActiveNode() != "primary:50051" {
		t.Fatal("should start on primary")
	}

	// Failover to fallback
	p.Failover()
	if p.ActiveNode() != "fallback:50051" {
		t.Fatal("should be on fallback")
	}

	// Recover to primary
	p.Recover()
	if p.ActiveNode() != "primary:50051" {
		t.Fatal("should be back on primary")
	}

	// Failover again
	p.Failover()
	if p.ActiveNode() != "fallback:50051" {
		t.Fatal("should be on fallback again")
	}
}

func TestClientAndNode_AfterFailover(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	p.Failover()

	c, addr := p.ClientAndNode()
	if c != p.fallback.client {
		t.Error("ClientAndNode() client should be fallback after failover")
	}
	if addr != "fallback:50051" {
		t.Errorf("ClientAndNode() addr = %q, want %q", addr, "fallback:50051")
	}
}

func TestNewFromClient(t *testing.T) {
	c := client.NewGrpcClient("test:50051")
	p := NewFromClient(c, "test:50051")
	if p.Client() != c {
		t.Error("NewFromClient should return pool with provided client")
	}
	if got := p.ActiveNode(); got != "test:50051" {
		t.Errorf("ActiveNode() = %q, want %q", got, "test:50051")
	}
	if p.FallbackClient() != nil {
		t.Error("NewFromClient should not have fallback")
	}
}

func TestSetAPIKey(t *testing.T) {
	p := newTestPool("primary:50051", "")
	err := p.SetAPIKey("test-key")
	if err != nil {
		t.Errorf("SetAPIKey() returned error: %v", err)
	}
}

func TestSetAPIKey_WithFallback(t *testing.T) {
	p := newTestPool("primary:50051", "fallback:50051")
	err := p.SetAPIKey("test-key")
	if err != nil {
		t.Errorf("SetAPIKey() with fallback returned error: %v", err)
	}
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
	if addr != "primary:50051" && addr != "fallback:50051" {
		t.Errorf("ActiveNode() = %q, want primary or fallback", addr)
	}
	if c == nil {
		t.Error("Client() should not be nil")
	}
	if addr == "primary:50051" && c != p.primary.client {
		t.Error("client/node mismatch: addr is primary but client is not")
	}
	if addr == "fallback:50051" && c != p.fallback.client {
		t.Error("client/node mismatch: addr is fallback but client is not")
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
	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}
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
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if p.Client() == nil {
		t.Error("Client() should not be nil")
	}
	if got := p.ActiveNode(); got != "passthrough:///bufconn" {
		t.Errorf("ActiveNode() = %q", got)
	}
	p.Stop()
}

func TestAddFallback_Success(t *testing.T) {
	primaryLis := startMockServer(t, &mockWalletServer{healthy: true})
	fallbackLis := startMockServer(t, &mockWalletServer{healthy: true})

	p, err := New("passthrough:///primary", bufconnDialOpts(primaryLis))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer p.Stop()

	err = p.AddFallback("passthrough:///fallback", bufconnDialOpts(fallbackLis))
	if err != nil {
		t.Fatalf("AddFallback() error: %v", err)
	}
	if p.FallbackClient() == nil {
		t.Error("FallbackClient() should not be nil after AddFallback")
	}
}

func TestCheckHealth_Healthy(t *testing.T) {
	c := newBufconnClient(t, &mockWalletServer{healthy: true})
	p := NewFromClient(c, "bufconn")
	if !p.CheckHealth() {
		t.Error("CheckHealth() should return true when node is healthy")
	}
}

func TestCheckHealth_Unhealthy_NoFallback(t *testing.T) {
	c := newBufconnClient(t, &mockWalletServer{healthy: false})
	p := NewFromClient(c, "bufconn")
	if p.CheckHealth() {
		t.Error("CheckHealth() should return false when node is unhealthy")
	}
}

func TestCheckHealth_Unhealthy_WithFallback(t *testing.T) {
	primary := newBufconnClient(t, &mockWalletServer{healthy: false})
	fallback := newBufconnClient(t, &mockWalletServer{healthy: true})

	p := NewFromClient(primary, "primary")
	p.fallback = &node{client: fallback, address: "fallback"}

	if p.CheckHealth() {
		t.Error("CheckHealth() should return false when primary is unhealthy")
	}
	// Should have failed over to fallback
	if p.ActiveNode() != "fallback" {
		t.Errorf("should have failed over, active = %q", p.ActiveNode())
	}
}

func TestCheckHealth_OnFallback_PrimaryRecovers(t *testing.T) {
	primary := newBufconnClient(t, &mockWalletServer{healthy: true})
	fallback := newBufconnClient(t, &mockWalletServer{healthy: true})

	p := NewFromClient(primary, "primary")
	p.fallback = &node{client: fallback, address: "fallback"}
	p.Failover() // move to fallback

	if !p.CheckHealth() {
		t.Error("CheckHealth() should return true when fallback is healthy")
	}
	// Should have recovered to primary since primary is also healthy
	if p.ActiveNode() != "primary" {
		t.Errorf("should have recovered to primary, active = %q", p.ActiveNode())
	}
}
