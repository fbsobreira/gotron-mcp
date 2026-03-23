package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsHostedMode(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		want      bool
	}{
		{"http is hosted", "http", true},
		{"stdio is not hosted", "stdio", false},
		{"empty is not hosted", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Transport: tt.transport}
			assert.Equal(t, tt.want, cfg.IsHostedMode(), "IsHostedMode()")
		})
	}
}

func TestEnvOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envVal   string
		setEnv   bool
		fallback string
		want     string
	}{
		{"env set", "TEST_CONFIG_VAR", "from-env", true, "default", "from-env"},
		{"env empty uses fallback", "TEST_CONFIG_EMPTY", "", true, "default", "default"},
		{"env unset uses fallback", "TEST_CONFIG_UNSET_XYZ", "", false, "fallback-val", "fallback-val"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envVal)
			} else {
				_ = os.Unsetenv(tt.key)
			}
			got := envOrDefault(tt.key, tt.fallback)
			assert.Equal(t, tt.want, got, "envOrDefault(%q, %q)", tt.key, tt.fallback)
		})
	}
}

func TestNetworkNodes(t *testing.T) {
	tests := []struct {
		name    string
		network string
		want    string
	}{
		{"mainnet", "mainnet", "grpc.trongrid.io:50051"},
		{"nile", "nile", "grpc.nile.trongrid.io:50051"},
		{"shasta", "shasta", "grpc.shasta.trongrid.io:50051"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := networkNodes[tt.network]
			require.True(t, ok, "network %q not found in networkNodes", tt.network)
			assert.Equal(t, tt.want, got, "networkNodes[%q]", tt.network)
		})
	}
}

func TestResolveNode(t *testing.T) {
	tests := []struct {
		name    string
		network string
		want    string
	}{
		{"mainnet", "mainnet", "grpc.trongrid.io:50051"},
		{"nile", "nile", "grpc.nile.trongrid.io:50051"},
		{"shasta", "shasta", "grpc.shasta.trongrid.io:50051"},
		{"unknown falls back to mainnet", "unknown", "grpc.trongrid.io:50051"},
		{"empty falls back to mainnet", "", "grpc.trongrid.io:50051"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveNode(tt.network)
			assert.Equal(t, tt.want, got, "resolveNode(%q)", tt.network)
		})
	}
}
