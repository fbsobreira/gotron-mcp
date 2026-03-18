package nodepool

import (
	"log"
	"sync"
	"sync/atomic"

	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"google.golang.org/grpc"
)

// Pool manages primary and fallback TRON gRPC clients with automatic failover.
type Pool struct {
	primary  *node
	fallback *node
	active   atomic.Pointer[node]
	mu       sync.Mutex
}

type node struct {
	client  *client.GrpcClient
	address string
}

// NewFromClient creates a pool from an existing client. Used for testing.
func NewFromClient(c *client.GrpcClient, addr string) *Pool {
	p := &Pool{}
	p.primary = &node{client: c, address: addr}
	p.active.Store(p.primary)
	return p
}

// New creates a pool with a primary node. Use WithFallback to add a fallback.
func New(primaryAddr string, opts []grpc.DialOption) (*Pool, error) {
	p := &Pool{}

	primaryClient := client.NewGrpcClient(primaryAddr)
	if err := primaryClient.Start(opts...); err != nil {
		return nil, err
	}
	p.primary = &node{client: primaryClient, address: primaryAddr}
	p.active.Store(p.primary)

	return p, nil
}

// AddFallback connects a fallback node. Must be called during initialization
// before the pool is used concurrently (before serving requests).
func (p *Pool) AddFallback(addr string, opts []grpc.DialOption) error {
	fallbackClient := client.NewGrpcClient(addr)
	if err := fallbackClient.Start(opts...); err != nil {
		return err
	}
	p.fallback = &node{client: fallbackClient, address: addr}
	return nil
}

// Client returns the currently active gRPC client.
func (p *Pool) Client() *client.GrpcClient {
	return p.active.Load().client
}

// FallbackClient returns the fallback gRPC client, or nil if no fallback is configured.
func (p *Pool) FallbackClient() *client.GrpcClient {
	if p.fallback == nil {
		return nil
	}
	return p.fallback.client
}

// ActiveNode returns the address of the currently active node.
func (p *Pool) ActiveNode() string {
	return p.active.Load().address
}

// ClientAndNode returns the active client and node address from a single
// atomic load, ensuring both values refer to the same node.
func (p *Pool) ClientAndNode() (*client.GrpcClient, string) {
	n := p.active.Load()
	return n.client, n.address
}

// SetAPIKey sets the API key on all connected clients.
func (p *Pool) SetAPIKey(key string) error {
	if err := p.primary.client.SetAPIKey(key); err != nil {
		return err
	}
	if p.fallback != nil {
		if err := p.fallback.client.SetAPIKey(key); err != nil {
			return err
		}
	}
	return nil
}

// Failover switches to the fallback node if available.
// Returns true if switched, false if no fallback or already on fallback.
func (p *Pool) Failover() bool {
	if p.fallback == nil {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	current := p.active.Load()
	if current == p.primary {
		p.active.Store(p.fallback)
		log.Printf("Failover: switched to fallback node %s", p.fallback.address)
		return true
	}
	return false
}

// Recover switches back to the primary node.
// Returns true if switched back, false if already on primary.
func (p *Pool) Recover() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	current := p.active.Load()
	if current != p.primary {
		p.active.Store(p.primary)
		log.Printf("Recovered: switched back to primary node %s", p.primary.address)
		return true
	}
	return false
}

// CheckHealth tests if the active node is healthy by calling GetNowBlock.
// If unhealthy and fallback exists, triggers failover.
// If on fallback and primary recovers, switches back.
func (p *Pool) CheckHealth() bool {
	active := p.active.Load()
	_, err := active.client.GetNowBlock()
	if err == nil {
		// Active is healthy — if we're on fallback, try to recover to primary
		if active == p.fallback {
			_, primaryErr := p.primary.client.GetNowBlock()
			if primaryErr == nil {
				p.Recover()
			}
		}
		return true
	}

	// Active is unhealthy — try failover
	p.Failover()
	return false
}

// Stop closes all client connections.
func (p *Pool) Stop() {
	p.primary.client.Stop()
	if p.fallback != nil {
		p.fallback.client.Stop()
	}
}
