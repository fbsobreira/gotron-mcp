package nodepool

import (
	"sync"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/client"
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
