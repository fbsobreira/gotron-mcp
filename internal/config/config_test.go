package config

import (
	"testing"
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
			if got := cfg.IsHostedMode(); got != tt.want {
				t.Errorf("IsHostedMode() = %v, want %v", got, tt.want)
			}
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
			}
			got := envOrDefault(tt.key, tt.fallback)
			if got != tt.want {
				t.Errorf("envOrDefault(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.want)
			}
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
			if !ok {
				t.Fatalf("network %q not found in networkNodes", tt.network)
			}
			if got != tt.want {
				t.Errorf("networkNodes[%q] = %q, want %q", tt.network, got, tt.want)
			}
		})
	}
}

func TestNetworkNodes_UnknownFallsToMainnet(t *testing.T) {
	_, ok := networkNodes["unknown"]
	if ok {
		t.Error("unknown network should not be in networkNodes")
	}
	// Parse logic falls back to mainnet for unknown networks
	mainnet := networkNodes["mainnet"]
	if mainnet != "grpc.trongrid.io:50051" {
		t.Errorf("mainnet fallback = %q, want grpc.trongrid.io:50051", mainnet)
	}
}
