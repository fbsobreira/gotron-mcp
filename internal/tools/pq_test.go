package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/pqc"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
)

const (
	hintNoPQ     = "This node has no TIP-899 post-quantum support (mainnet today). Use a Nile node."
	hintNoFalcon = "This node allows FN-DSA-512, but this build cannot sign it; use the gotron-mcp-pq build, or build locally with liboqs installed: CGO_ENABLED=1 go build -tags falcon ./cmd/gotron-mcp."
)

func pqChainParams(params ...*core.ChainParameters_ChainParameter) func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
	return func(_ context.Context, _ *api.EmptyMessage) (*core.ChainParameters, error) {
		return &core.ChainParameters{ChainParameter: params}, nil
	}
}

func TestGetPQCapabilities(t *testing.T) {
	fnBuild := pqc.SchemeAvailable(core.PQScheme_FN_DSA_512) == nil
	mlBuild := pqc.SchemeAvailable(core.PQScheme_ML_DSA_44) == nil

	other := &core.ChainParameters_ChainParameter{Key: "getTransactionFee", Value: 1000}
	tests := []struct {
		name     string
		params   []*core.ChainParameters_ChainParameter
		nodeFN   bool
		nodeML   bool
		wantHint func() string
	}{
		{
			name:   "nile-like: FN-DSA on",
			params: []*core.ChainParameters_ChainParameter{other, {Key: "getAllowFnDsa512", Value: 1}},
			nodeFN: true,
			wantHint: func() string {
				if !fnBuild {
					return hintNoFalcon
				}
				return ""
			},
		},
		{
			name:     "mainnet: no PQ keys",
			params:   []*core.ChainParameters_ChainParameter{other},
			wantHint: func() string { return hintNoPQ },
		},
		{
			name: "explicit zero values",
			params: []*core.ChainParameters_ChainParameter{
				{Key: "getAllowFnDsa512", Value: 0},
				{Key: "getAllowMlDsa44", Value: 0},
			},
			wantHint: func() string { return hintNoPQ },
		},
		{
			name: "both on",
			params: []*core.ChainParameters_ChainParameter{
				{Key: "getAllowFnDsa512", Value: 1},
				{Key: "getAllowMlDsa44", Value: 1},
			},
			nodeFN: true,
			nodeML: true,
			wantHint: func() string {
				if !fnBuild {
					return hintNoFalcon
				}
				return ""
			},
		},
		{
			name:     "only ML-DSA on",
			params:   []*core.ChainParameters_ChainParameter{{Key: "getAllowMlDsa44", Value: 5}},
			nodeML:   true,
			wantHint: func() string { return "" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := newMockPool(t, &mockWalletServer{GetChainParametersFunc: pqChainParams(tt.params...)})
			result := callTool(t, handleGetPQCapabilities(pool), map[string]any{})
			require.False(t, result.IsError, "expected success, got error: %v", result.Content)

			data := parseJSONResult(t, result)
			assert.Equal(t, map[string]any{"fn_dsa_512": tt.nodeFN, "ml_dsa_44": tt.nodeML}, data["node"])
			assert.Equal(t, map[string]any{"fn_dsa_512": fnBuild, "ml_dsa_44": mlBuild}, data["build"])
			assert.Equal(t, map[string]any{
				"fn_dsa_512": tt.nodeFN && fnBuild,
				"ml_dsa_44":  tt.nodeML && mlBuild,
			}, data["can_sign"])

			if want := tt.wantHint(); want != "" {
				assert.Equal(t, want, data["hint"])
			} else {
				assert.NotContains(t, data, "hint")
			}
		})
	}
}

func TestGetPQCapabilities_Error(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{
		GetChainParametersFunc: func(_ context.Context, _ *api.EmptyMessage) (*core.ChainParameters, error) {
			return nil, fmt.Errorf("node unavailable")
		},
	})
	result := callTool(t, handleGetPQCapabilities(pool), map[string]any{})
	require.True(t, result.IsError, "expected error when chain parameters fail")
	text := toolErrText(t, result)
	assert.Contains(t, text, "get_pq_capabilities: ")
	assert.Contains(t, text, "node unavailable")
}
