package trongrid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetTransactionsByAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/TAddr123/transactions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("expected limit=5, got %s", r.URL.Query().Get("limit"))
		}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(resp.Data))
	}
	if resp.Data[0].TransactionID != "abc123" {
		t.Errorf("expected txID abc123, got %s", resp.Data[0].TransactionID)
	}
	if resp.Meta.Fingerprint != "next123" {
		t.Errorf("expected fingerprint next123, got %s", resp.Meta.Fingerprint)
	}
}

func TestGetTRC20Transfers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/TAddr123/transactions/trc20" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(resp.Data))
	}
	if resp.Data[0].TokenInfo.Symbol != "USDT" {
		t.Errorf("expected symbol USDT, got %s", resp.Data[0].TokenInfo.Symbol)
	}
}

func TestGetContractEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/contracts/TContract/events" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp.Data))
	}
	if resp.Data[0].EventName != "Transfer" {
		t.Errorf("expected event Transfer, got %s", resp.Data[0].EventName)
	}
}

func TestGetContractEventsByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("event_name") != "Approval" {
			t.Errorf("expected event_name=Approval, got %s", r.URL.Query().Get("event_name"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": [], "success": true, "meta": {"page_size": 0}}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	resp, err := c.GetContractEventsByName(context.Background(), "TContract", "Approval", QueryOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Fatalf("expected 0 events, got %d", len(resp.Data))
	}
}

func TestAPIKeyHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("TRON-PRO-API-KEY")
		if key != "test-key" {
			t.Errorf("expected API key test-key, got %q", key)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": [], "success": true, "meta": {}}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, apiKey: "test-key", httpClient: srv.Client()}
	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPError429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	if err == nil {
		t.Fatal("expected error for 429")
	}
}

func TestHTTPError500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestPaginationPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fp := r.URL.Query().Get("fingerprint")
		if fp != "cursor123" {
			t.Errorf("expected fingerprint=cursor123, got %s", fp)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": [], "success": true, "meta": {"fingerprint": "cursor456"}}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	resp, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{Fingerprint: "cursor123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Meta.Fingerprint != "cursor456" {
		t.Errorf("expected fingerprint cursor456, got %s", resp.Meta.Fingerprint)
	}
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
		if got := params.Get(tt.key); got != tt.want {
			t.Errorf("params[%s] = %q, want %q", tt.key, got, tt.want)
		}
	}
	if params.Get("only_from") != "" {
		t.Errorf("expected only_from to be empty, got %s", params.Get("only_from"))
	}
}

func TestSuccessFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": [], "success": false, "meta": {}}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}

	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	if err == nil {
		t.Fatal("expected error for success=false")
	}

	_, err = c.GetTRC20Transfers(context.Background(), "TAddr", QueryOpts{})
	if err == nil {
		t.Fatal("expected error for success=false")
	}

	_, err = c.GetContractEvents(context.Background(), "TAddr", QueryOpts{})
	if err == nil {
		t.Fatal("expected error for success=false")
	}
}

func TestTruncateBody(t *testing.T) {
	short := []byte("short error")
	if got := truncateBody(short); got != "short error" {
		t.Errorf("truncateBody(short) = %q, want %q", got, "short error")
	}

	long := make([]byte, 1024)
	for i := range long {
		long[i] = 'x'
	}
	got := truncateBody(long)
	if len(got) != 512+len("...(truncated)") {
		t.Errorf("truncateBody(1024 bytes) length = %d, want %d", len(got), 512+len("...(truncated)"))
	}
	if got[len(got)-len("...(truncated)"):] != "...(truncated)" {
		t.Error("truncateBody should end with ...(truncated)")
	}
}

func TestBuildParamsOnlyFrom(t *testing.T) {
	opts := QueryOpts{OnlyFrom: true}
	params := buildParams(opts)
	if params.Get("only_from") != "true" {
		t.Errorf("only_from = %q, want true", params.Get("only_from"))
	}
}

func TestHTTPError4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.GetTransactionsByAddress(context.Background(), "TAddr", QueryOpts{})
	if err == nil {
		t.Fatal("expected error for 400")
	}
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
		if c.baseURL != tt.wantURL {
			t.Errorf("NewClient(%q).baseURL = %q, want %q", tt.network, c.baseURL, tt.wantURL)
		}
	}
}
