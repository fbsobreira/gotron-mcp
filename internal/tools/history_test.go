package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/trongrid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHistoryClient struct {
	txResp    *trongrid.Response[trongrid.Transaction]
	trc20Resp *trongrid.Response[trongrid.TRC20Transfer]
	eventResp *trongrid.Response[trongrid.ContractEvent]
	err       error
}

func (m *mockHistoryClient) GetTransactionsByAddress(_ context.Context, _ string, _ trongrid.QueryOpts) (*trongrid.Response[trongrid.Transaction], error) {
	return m.txResp, m.err
}

func (m *mockHistoryClient) GetTRC20Transfers(_ context.Context, _ string, _ trongrid.QueryOpts) (*trongrid.Response[trongrid.TRC20Transfer], error) {
	return m.trc20Resp, m.err
}

func (m *mockHistoryClient) GetContractEvents(_ context.Context, _ string, _ trongrid.QueryOpts) (*trongrid.Response[trongrid.ContractEvent], error) {
	return m.eventResp, m.err
}

func (m *mockHistoryClient) GetContractEventsByName(_ context.Context, _, _ string, _ trongrid.QueryOpts) (*trongrid.Response[trongrid.ContractEvent], error) {
	return m.eventResp, m.err
}

func TestHandleGetTransactionHistory(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]any
		client  *mockHistoryClient
		wantErr bool
	}{
		{
			name:    "missing address",
			params:  map[string]any{},
			client:  &mockHistoryClient{},
			wantErr: true,
		},
		{
			name:   "valid request",
			params: map[string]any{"address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"},
			client: &mockHistoryClient{
				txResp: &trongrid.Response[trongrid.Transaction]{
					Data:    []trongrid.Transaction{{TransactionID: "tx1"}},
					Success: true,
					Meta:    trongrid.Meta{PageSize: 1, Fingerprint: "fp1"},
				},
			},
		},
		{
			name:   "api error",
			params: map[string]any{"address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"},
			client: &mockHistoryClient{
				err: fmt.Errorf("connection refused"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handleGetTransactionHistory(tt.client)
			req := mcp.CallToolRequest{}
			req.Params.Arguments = tt.params

			result, err := handler(context.Background(), req)
			require.NoError(t, err, "unexpected Go error")
			if tt.wantErr {
				assert.True(t, result.IsError, "expected tool error")
				return
			}
			assert.False(t, result.IsError, "unexpected tool error: %v", result.Content)
		})
	}
}

func TestHandleGetTransactionHistoryShapedOutput(t *testing.T) {
	client := &mockHistoryClient{
		txResp: &trongrid.Response[trongrid.Transaction]{
			Data:    []trongrid.Transaction{{TransactionID: "tx1", BlockNumber: 100}},
			Success: true,
			Meta:    trongrid.Meta{PageSize: 1, Fingerprint: "fp1"},
		},
	}

	handler := handleGetTransactionHistory(client)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"}

	result, err := handler(context.Background(), req)
	require.NoError(t, err, "unexpected error")
	require.False(t, result.IsError, "unexpected tool error: %v", result.Content)
	require.NotEmpty(t, result.Content, "expected non-empty result content")
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	var data map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &data), "failed to unmarshal result")
	assert.Equal(t, float64(1), data["count"])
	txs, ok := data["transactions"].([]any)
	require.True(t, ok && len(txs) > 0, "expected non-empty transactions array")
	tx, ok := txs[0].(map[string]any)
	require.True(t, ok, "expected map for transaction, got %T", txs[0])
	assert.Equal(t, "tx1", tx["txid"])
	// Should not have raw_data (full blob) — only shaped fields
	_, hasRawData := tx["raw_data"]
	assert.False(t, hasRawData, "shaped output should not contain raw_data")
	meta, ok := data["meta"].(map[string]any)
	require.True(t, ok, "expected meta map in response")
	assert.Equal(t, "fp1", meta["fingerprint"])
}

func TestHandleGetTRC20Transfers(t *testing.T) {
	client := &mockHistoryClient{
		trc20Resp: &trongrid.Response[trongrid.TRC20Transfer]{
			Data: []trongrid.TRC20Transfer{{
				TransactionID: "tx1",
				From:          "TFrom",
				To:            "TTo",
				Value:         "1000000",
				TokenInfo:     trongrid.TokenInfo{Symbol: "USDT", Decimals: 6},
			}},
			Success: true,
			Meta:    trongrid.Meta{PageSize: 1},
		},
	}

	handler := handleGetTRC20Transfers(client)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"}

	result, err := handler(context.Background(), req)
	require.NoError(t, err, "unexpected error")
	require.False(t, result.IsError, "unexpected tool error: %v", result.Content)

	require.NotEmpty(t, result.Content, "expected non-empty result content")
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	var data map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &data), "failed to unmarshal result")
	transfers, ok := data["transfers"].([]any)
	require.True(t, ok && len(transfers) == 1, "expected 1 transfer, got %v", data["transfers"])
	tr, ok := transfers[0].(map[string]any)
	require.True(t, ok, "expected map for transfer, got %T", transfers[0])
	assert.Equal(t, "USDT", tr["token_symbol"])
	assert.Equal(t, "TFrom", tr["from"])
}

func TestHandleGetContractEvents(t *testing.T) {
	client := &mockHistoryClient{
		eventResp: &trongrid.Response[trongrid.ContractEvent]{
			Data: []trongrid.ContractEvent{{
				TransactionID: "tx1",
				EventName:     "Transfer",
			}},
			Success: true,
			Meta:    trongrid.Meta{PageSize: 1},
		},
	}

	handler := handleGetContractEvents(client)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"contract_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"}

	result, err := handler(context.Background(), req)
	require.NoError(t, err, "unexpected error")
	require.False(t, result.IsError, "unexpected tool error: %v", result.Content)

	require.NotEmpty(t, result.Content, "expected non-empty result content")
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	var data map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &data), "failed to unmarshal result")
	assert.Equal(t, float64(1), data["count"])
	events, ok := data["events"].([]any)
	require.True(t, ok && len(events) > 0, "expected non-empty events array")
	ev, ok := events[0].(map[string]any)
	require.True(t, ok, "expected map for event, got %T", events[0])
	assert.Equal(t, "tx1", ev["txid"])
	assert.Equal(t, "Transfer", ev["event_name"])
}

func TestHandleGetContractEventsWithName(t *testing.T) {
	called := false
	client := &mockHistoryClient{
		eventResp: &trongrid.Response[trongrid.ContractEvent]{
			Data:    []trongrid.ContractEvent{},
			Success: true,
		},
	}

	// Override to track that GetContractEventsByName is called
	handler := handleGetContractEvents(&trackingClient{
		mockHistoryClient: client,
		onEventsByName:    func() { called = true },
	})
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"contract_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
		"event_name":       "Transfer",
	}

	result, err := handler(context.Background(), req)
	require.NoError(t, err, "unexpected error")
	require.False(t, result.IsError, "unexpected tool error: %v", result.Content)
	assert.True(t, called, "expected GetContractEventsByName to be called")
}

type trackingClient struct {
	*mockHistoryClient
	onEventsByName func()
}

func (tc *trackingClient) GetContractEventsByName(ctx context.Context, addr, name string, opts trongrid.QueryOpts) (*trongrid.Response[trongrid.ContractEvent], error) {
	if tc.onEventsByName != nil {
		tc.onEventsByName()
	}
	return tc.mockHistoryClient.GetContractEventsByName(ctx, addr, name, opts)
}

func TestParseQueryOpts(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"limit":          float64(50),
		"fingerprint":    "abc",
		"only_confirmed": true,
		"only_to":        true,
		"min_timestamp":  float64(1000),
		"max_timestamp":  float64(2000),
	}
	// Note: JSON unmarshals numbers as float64, which is what mcp-go expects

	opts, err := parseQueryOpts(req)
	require.NoError(t, err, "unexpected error")
	assert.Equal(t, 50, opts.Limit)
	assert.Equal(t, "abc", opts.Fingerprint)
	assert.True(t, opts.OnlyConfirmed, "OnlyConfirmed should be true")
	assert.True(t, opts.OnlyTo, "OnlyTo should be true")
	assert.Equal(t, int64(1000), opts.MinTimestamp)
	assert.Equal(t, int64(2000), opts.MaxTimestamp)
}

func TestParseQueryOptsLimitCap(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"limit": float64(500)}
	// Note: JSON unmarshals numbers as float64, which is what mcp-go expects

	opts, err := parseQueryOpts(req)
	require.NoError(t, err, "unexpected error")
	assert.Equal(t, 200, opts.Limit, "Limit should be capped at 200")
}

func TestParseQueryOptsDefaultLimit(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	opts, err := parseQueryOpts(req)
	require.NoError(t, err, "unexpected error")
	assert.Equal(t, defaultHistoryLimit, opts.Limit, "Limit should be default")
}

func TestParseQueryOptsOnlyFrom(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"only_from": true}

	opts, err := parseQueryOpts(req)
	require.NoError(t, err, "unexpected error")
	assert.True(t, opts.OnlyFrom, "OnlyFrom should be true")
	assert.False(t, opts.OnlyTo, "OnlyTo should be false")
}

func TestParseQueryOptsMutuallyExclusive(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"only_to":   true,
		"only_from": true,
	}

	_, err := parseQueryOpts(req)
	require.Error(t, err, "expected error for only_to + only_from")
}

func TestParseQueryOptsTimestampOrder(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"min_timestamp": float64(2000),
		"max_timestamp": float64(1000),
	}

	_, err := parseQueryOpts(req)
	require.Error(t, err, "expected error for min_timestamp > max_timestamp")
}

func TestHandleGetTransactionHistoryNilResponse(t *testing.T) {
	client := &mockHistoryClient{txResp: nil, err: nil}
	handler := handleGetTransactionHistory(client)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"}

	result, err := handler(context.Background(), req)
	require.NoError(t, err, "unexpected Go error")
	assert.True(t, result.IsError, "expected tool error for nil response")
}

func TestHandleGetTRC20TransfersNilResponse(t *testing.T) {
	client := &mockHistoryClient{trc20Resp: nil, err: nil}
	handler := handleGetTRC20Transfers(client)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"}

	result, err := handler(context.Background(), req)
	require.NoError(t, err, "unexpected Go error")
	assert.True(t, result.IsError, "expected tool error for nil response")
}

func TestHandleGetContractEventsNilResponse(t *testing.T) {
	client := &mockHistoryClient{eventResp: nil, err: nil}
	handler := handleGetContractEvents(client)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"contract_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"}

	result, err := handler(context.Background(), req)
	require.NoError(t, err, "unexpected Go error")
	assert.True(t, result.IsError, "expected tool error for nil response")
}

func TestHandleGetTRC20TransfersErrors(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		client *mockHistoryClient
	}{
		{
			name:   "missing address",
			params: map[string]any{},
			client: &mockHistoryClient{},
		},
		{
			name:   "api error",
			params: map[string]any{"address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"},
			client: &mockHistoryClient{err: fmt.Errorf("timeout")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handleGetTRC20Transfers(tt.client)
			req := mcp.CallToolRequest{}
			req.Params.Arguments = tt.params

			result, err := handler(context.Background(), req)
			require.NoError(t, err, "unexpected Go error")
			assert.True(t, result.IsError, "expected tool error")
		})
	}
}

func TestHandleGetContractEventsErrors(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		client *mockHistoryClient
	}{
		{
			name:   "missing address",
			params: map[string]any{},
			client: &mockHistoryClient{},
		},
		{
			name:   "api error",
			params: map[string]any{"contract_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"},
			client: &mockHistoryClient{err: fmt.Errorf("timeout")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handleGetContractEvents(tt.client)
			req := mcp.CallToolRequest{}
			req.Params.Arguments = tt.params

			result, err := handler(context.Background(), req)
			require.NoError(t, err, "unexpected Go error")
			assert.True(t, result.IsError, "expected tool error")
		})
	}
}
