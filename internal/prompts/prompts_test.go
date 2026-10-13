package prompts

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterPrompts(t *testing.T) {
	s := server.NewMCPServer("test", "1.0", server.WithPromptCapabilities(false))
	RegisterPrompts(s)
	// Should not panic — prompts registered successfully
}

func newPromptReq(args map[string]string) mcp.GetPromptRequest {
	return mcp.GetPromptRequest{
		Params: struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments,omitempty"`
		}{
			Arguments: args,
		},
	}
}

func TestAccountOverview(t *testing.T) {
	req := newPromptReq(map[string]string{"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF"})
	result, err := handleAccountOverview(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, result.Messages, 1)
	assert.Equal(t, mcp.RoleUser, result.Messages[0].Role)

	text := result.Messages[0].Content.(mcp.TextContent).Text
	assert.Contains(t, text, "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF")
	assert.Contains(t, text, "balance")
	assert.Contains(t, text, "energy")
	assert.Contains(t, text, "permissions")
	assert.Contains(t, text, "delegated")
}

func TestAccountOverview_MissingAddress(t *testing.T) {
	req := newPromptReq(map[string]string{})
	_, err := handleAccountOverview(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "address is required")
}

func TestTransferChecklist(t *testing.T) {
	req := newPromptReq(map[string]string{
		"from":   "TAddr1",
		"to":     "TAddr2",
		"amount": "100",
	})
	result, err := handleTransferChecklist(context.Background(), req)
	require.NoError(t, err)

	text := result.Messages[0].Content.(mcp.TextContent).Text
	assert.Contains(t, text, "100 TRX")
	assert.Contains(t, text, "TAddr1")
	assert.Contains(t, text, "TAddr2")
	assert.Contains(t, text, "validate")
	assert.Contains(t, text, "balance")
	assert.Contains(t, text, "energy")
}

func TestTransferChecklist_WithToken(t *testing.T) {
	req := newPromptReq(map[string]string{
		"from":   "TAddr1",
		"to":     "TAddr2",
		"amount": "50",
		"token":  "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
	})
	result, err := handleTransferChecklist(context.Background(), req)
	require.NoError(t, err)

	text := result.Messages[0].Content.(mcp.TextContent).Text
	assert.Contains(t, text, "50 TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t")
}

func TestTransferChecklist_DefaultToken(t *testing.T) {
	req := newPromptReq(map[string]string{
		"from":   "TAddr1",
		"to":     "TAddr2",
		"amount": "10",
	})
	result, err := handleTransferChecklist(context.Background(), req)
	require.NoError(t, err)

	text := result.Messages[0].Content.(mcp.TextContent).Text
	assert.Contains(t, text, "10 TRX")
}

func TestTransferChecklist_MissingArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]string
	}{
		{"missing from", map[string]string{"to": "T", "amount": "1"}},
		{"missing to", map[string]string{"from": "T", "amount": "1"}},
		{"missing amount", map[string]string{"from": "T", "to": "T"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handleTransferChecklist(context.Background(), newPromptReq(tt.args))
			assert.Error(t, err)
		})
	}
}

func TestTransactionExplain(t *testing.T) {
	txid := "abc123def456"
	req := newPromptReq(map[string]string{"txid": txid})
	result, err := handleTransactionExplain(context.Background(), req)
	require.NoError(t, err)

	text := result.Messages[0].Content.(mcp.TextContent).Text
	assert.Contains(t, text, txid)
	assert.Contains(t, text, "contract type")
	assert.Contains(t, text, "fee breakdown")
}

func TestTransactionExplain_MissingTxid(t *testing.T) {
	req := newPromptReq(map[string]string{})
	_, err := handleTransactionExplain(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "txid is required")
}

func TestStakingStatus(t *testing.T) {
	req := newPromptReq(map[string]string{"address": "TStaker123"})
	result, err := handleStakingStatus(context.Background(), req)
	require.NoError(t, err)

	text := result.Messages[0].Content.(mcp.TextContent).Text
	assert.Contains(t, text, "TStaker123")
	assert.Contains(t, text, "staking")
	assert.Contains(t, text, "delegat")
	assert.Contains(t, text, "unfreeze")
	assert.Contains(t, text, "vote")
}

func TestStakingStatus_MissingAddress(t *testing.T) {
	req := newPromptReq(map[string]string{})
	_, err := handleStakingStatus(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "address is required")
}
