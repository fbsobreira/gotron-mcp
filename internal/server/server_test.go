package server

import (
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/config"
	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-sdk/pkg/client"
)

func newTestPool() *nodepool.Pool {
	c := client.NewGrpcClient("mock:50051")
	return nodepool.NewFromClient(c, "mock:50051")
}

func TestNew_HostedMode(t *testing.T) {
	cfg := &config.Config{
		Transport: "http",
		Network:   "mainnet",
		Node:      "mock:50051",
	}
	pool := newTestPool()

	s := New(cfg, pool)
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_LocalMode(t *testing.T) {
	cfg := &config.Config{
		Transport: "stdio",
		Network:   "mainnet",
		Node:      "mock:50051",
	}
	pool := newTestPool()

	s := New(cfg, pool)
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_LocalModeWithKeystore(t *testing.T) {
	cfg := &config.Config{
		Transport: "stdio",
		Network:   "mainnet",
		Node:      "mock:50051",
		Keystore:  "/tmp/test-keystore",
	}
	pool := newTestPool()

	s := New(cfg, pool)
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_HostedModeNoSignTools(t *testing.T) {
	cfg := &config.Config{
		Transport: "http",
		Network:   "mainnet",
		Node:      "mock:50051",
		Keystore:  "/tmp/test-keystore", // keystore set but hosted mode
	}
	pool := newTestPool()

	s := New(cfg, pool)
	if s == nil {
		t.Fatal("New() returned nil")
	}
	// In hosted mode, sign tools should NOT be registered even with keystore
}
