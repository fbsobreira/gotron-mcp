package server

import (
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/config"
	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/stretchr/testify/require"
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

	s, wm, _ := New(cfg, pool)
	require.NotNil(t, s, "New() returned nil")
	require.Nil(t, wm, "expected nil wallet manager in hosted mode")
}

func TestNew_LocalMode(t *testing.T) {
	cfg := &config.Config{
		Transport: "stdio",
		Network:   "mainnet",
		Node:      "mock:50051",
	}
	pool := newTestPool()

	s, wm, _ := New(cfg, pool)
	require.NotNil(t, s, "New() returned nil")
	require.Nil(t, wm, "expected nil wallet manager without keystore config")
}

func TestNew_LocalModeWithKeystore(t *testing.T) {
	cfg := &config.Config{
		Transport:    "stdio",
		Network:      "mainnet",
		Node:         "mock:50051",
		KeystoreDir:  t.TempDir(),
		KeystorePass: "test-pass",
	}
	pool := newTestPool()

	s, wm, _ := New(cfg, pool)
	require.NotNil(t, s, "New() returned nil")
	require.NotNil(t, wm, "expected non-nil wallet manager with keystore config")
	wm.Close()
}

func TestNew_HostedModeNoSignTools(t *testing.T) {
	cfg := &config.Config{
		Transport:    "http",
		Network:      "mainnet",
		Node:         "mock:50051",
		KeystoreDir:  t.TempDir(),
		KeystorePass: "test-pass",
	}
	pool := newTestPool()

	s, wm, _ := New(cfg, pool)
	require.NotNil(t, s, "New() returned nil")
	if wm != nil {
		wm.Close()
		require.Fail(t, "expected nil wallet manager in hosted mode even with keystore config")
	}
	// In hosted mode, sign tools should NOT be registered even with keystore
}
