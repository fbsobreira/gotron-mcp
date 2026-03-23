package trongrid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTransactionsByAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/accounts/TAddr123/transactions", r.URL.Path)
		assert.Equal(t, "5", r.URL.Query().Get("limit"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [{"txID": "abc123", "blockNumber": 100, "block_timestamp": 1700000000}],
			"success": true,
			"meta": {"at": 1700000000, "page_size": 1, "fingerprint": "next123"}
		}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	resp, err := c.GetTransactionsByAddress(context.Background(), "TAddr123", QueryOpts{Limit: 5})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "abc123", resp.Data[0].TransactionID)
	assert.Equal(t, "next123", resp.Meta.Fingerprint)
}

func TestGetTRC20Transfers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/accounts/TAddr123/transactions/trc20", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [{
				"transaction_id": "def456",
				"from": "TFrom",
				"to": "TTo",
				"value": "1000000",
				"type": "Transfer",
				"block_timestamp": 1700000000,
				"token_info": {"symbol": "USDT", "address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", "decimals": 6, "name": "Tether USD"}
			}],
			"success": true,
			"meta": {"at": 1700000000, "page_size": 1, "fingerprint": ""}
		}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	resp, err := c.GetTRC20Transfers(context.Background(), "TAddr123", QueryOpts{})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "USDT", resp.Data[0].TokenInfo.Symbol)
}

func TestGetContractEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/contracts/TContract/events", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [{
				"transaction_id": "ghi789",
				"block_number": 200,
				"block_timestamp": 1700000000,
				"event_name": "Transfer",
				"contract_address": "TContract",
				"result": {"from": "TFrom", "to": "TTo", "value": "1000"},
				"result_type": {"from": "address", "to": "address", "value": "uint256"}
			}],
			"success": true,
			"meta": {"at": 1700000000, "page_size": 1, "fingerprint": ""}
		}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	resp, err := c.GetContractEvents(context.Background(), "TContract", QueryOpts{})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "Transfer", resp.Data[0].EventName)
}

func TestGetContractEventsByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Approval", r.URL.Query().Get("event_name"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": [], "success": true, "meta": {"page_size": 0}}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	resp, err := c.GetContractEventsByName(context.Background(), "TContract", "Approval", QueryOpts{})
	require.NoError(t, err)
	assert.Empty(t, resp.Data)
}

func TestAPIKeyHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("TRON-PRO-API-KEY"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": [], "success": true, "meta": {}}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, apiKey: "test-key", httpClient: srv.Client()}
	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	require.NoError(t, err)
}

func TestHTTPError429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	require.Error(t, err, "expected error for 429")
}

func TestHTTPError500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	require.Error(t, err, "expected error for 500")
}

func TestInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	require.Error(t, err, "expected error for invalid JSON")
}

func TestPaginationPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "cursor123", r.URL.Query().Get("fingerprint"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": [], "success": true, "meta": {"fingerprint": "cursor456"}}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	resp, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{Fingerprint: "cursor123"})
	require.NoError(t, err)
	assert.Equal(t, "cursor456", resp.Meta.Fingerprint)
}

func TestBuildParams(t *testing.T) {
	opts := QueryOpts{
		Limit:         50,
		Fingerprint:   "abc",
		OnlyConfirmed: true,
		OnlyTo:        true,
		OnlyFrom:      false,
		MinTimestamp:  1000,
		MaxTimestamp:  2000,
		OrderBy:       "block_timestamp,asc",
	}
	params := buildParams(opts)

	tests := []struct {
		key, want string
	}{
		{"limit", "50"},
		{"fingerprint", "abc"},
		{"only_confirmed", "true"},
		{"only_to", "true"},
		{"min_timestamp", "1000"},
		{"max_timestamp", "2000"},
		{"order_by", "block_timestamp,asc"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, params.Get(tt.key), "params[%s]", tt.key)
	}
	assert.Empty(t, params.Get("only_from"), "expected only_from to be empty")
}

func TestSuccessFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": [], "success": false, "meta": {}}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}

	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	require.Error(t, err, "expected error for success=false")

	_, err = c.GetTRC20Transfers(context.Background(), "TAddr", QueryOpts{})
	require.Error(t, err, "expected error for success=false")

	_, err = c.GetContractEvents(context.Background(), "TAddr", QueryOpts{})
	require.Error(t, err, "expected error for success=false")
}

func TestTruncateBody(t *testing.T) {
	short := []byte("short error")
	assert.Equal(t, "short error", truncateBody(short))

	long := make([]byte, 1024)
	for i := range long {
		long[i] = 'x'
	}
	got := truncateBody(long)
	assert.Len(t, got, 512+len("...(truncated)"))
	assert.True(t, strings.HasSuffix(got, "...(truncated)"), "truncateBody should end with ...(truncated)")
}

func TestBuildParamsOnlyFrom(t *testing.T) {
	opts := QueryOpts{OnlyFrom: true}
	params := buildParams(opts)
	assert.Equal(t, "true", params.Get("only_from"))
}

func TestHTTPError4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	require.Error(t, err, "expected error for 400")
}

func TestNewClientNetworks(t *testing.T) {
	tests := []struct {
		network string
		wantURL string
	}{
		{"mainnet", "https://api.trongrid.io"},
		{"nile", "https://nile.trongrid.io"},
		{"shasta", "https://api.shasta.trongrid.io"},
		{"unknown", "https://api.trongrid.io"},
	}
	for _, tt := range tests {
		c := NewClient(tt.network, "")
		assert.Equal(t, tt.wantURL, c.baseURL, "NewClient(%q).baseURL", tt.network)
	}
}
