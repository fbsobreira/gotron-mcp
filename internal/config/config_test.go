package config

import (
	"os"
	"path/filepath"
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

func TestConfig_DefaultPolicyConfig(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err, "UserHomeDir must succeed")

	tests := []struct {
		name   string
		envVal string
		want   string
	}{
		{
			"empty defaults to home-based path",
			"",
			filepath.Join(home, ".gotron-mcp", "policy.yaml"),
		},
		{
			"env override is preserved",
			"/custom/policy.yaml",
			"/custom/policy.yaml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{PolicyConfig: tt.envVal}
			if cfg.PolicyConfig == "" && home != "" {
				cfg.PolicyConfig = filepath.Join(home, ".gotron-mcp", "policy.yaml")
			}
			assert.Equal(t, tt.want, cfg.PolicyConfig)
		})
	}
}

func TestConfig_DefaultStateDir(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err, "UserHomeDir must succeed")

	tests := []struct {
		name   string
		envVal string
		want   string
	}{
		{
			"empty defaults to home-based path",
			"",
			filepath.Join(home, ".gotron-mcp"),
		},
		{
			"env override is preserved",
			"/custom/state",
			"/custom/state",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{StateDir: tt.envVal}
			if cfg.StateDir == "" && home != "" {
				cfg.StateDir = filepath.Join(home, ".gotron-mcp")
			}
			assert.Equal(t, tt.want, cfg.StateDir)
		})
	}
}

func TestConfig_DefaultKeystoreDir(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err, "UserHomeDir must succeed")

	tests := []struct {
		name   string
		envVal string
		want   string
	}{
		{
			"empty defaults to home-based path",
			"",
			filepath.Join(home, ".gotron-mcp", "wallets"),
		},
		{
			"env override is preserved",
			"/custom/wallets",
			"/custom/wallets",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{KeystoreDir: tt.envVal}
			if cfg.KeystoreDir == "" && home != "" {
				cfg.KeystoreDir = filepath.Join(home, ".gotron-mcp", "wallets")
			}
			assert.Equal(t, tt.want, cfg.KeystoreDir)
		})
	}
}

func TestEnvOrDefaultInt(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envVal   string
		setEnv   bool
		fallback int
		want     int
	}{
		{"env set valid", "TEST_INT_VAR", "42", true, 0, 42},
		{"env unset uses fallback", "TEST_INT_UNSET_XYZ", "", false, 10, 10},
		{"env empty uses fallback", "TEST_INT_EMPTY", "", true, 5, 5},
		{"env invalid uses fallback", "TEST_INT_INVALID", "abc", true, 7, 7},
		{"env negative uses fallback", "TEST_INT_NEG", "-1", true, 3, 3},
		{"env zero", "TEST_INT_ZERO", "0", true, 99, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envVal)
			} else {
				_ = os.Unsetenv(tt.key)
			}
			got := envOrDefaultInt(tt.key, tt.fallback)
			assert.Equal(t, tt.want, got, "envOrDefaultInt(%q, %d)", tt.key, tt.fallback)
		})
	}
}
