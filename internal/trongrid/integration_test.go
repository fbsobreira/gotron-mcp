package trongrid

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests against nile testnet. Skipped unless TRONGRID_INTEGRATION=1.
// Run: TRONGRID_INTEGRATION=1 go test -v -run TestIntegration ./internal/trongrid/

const (
	nileTestAddress  = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	nileTestContract = "TXLAQ63Xg1NAzckPwKHvzw7CSEmLMEqcdj" // USDT on nile
)

func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("TRONGRID_INTEGRATION") != "1" {
		t.Skip("skipping integration test; set TRONGRID_INTEGRATION=1 to run")
	}
}

func TestIntegrationGetTransactionsByAddress(t *testing.T) {
	skipUnlessIntegration(t)

	c := NewClient("nile", os.Getenv("GOTRON_NODE_API_KEY"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.GetTransactionsByAddress(ctx, nileTestAddress, QueryOpts{Limit: 3})
	require.NoError(t, err, "GetTransactionsByAddress")
	require.True(t, resp.Success, "expected success=true")
	require.NotEmpty(t, resp.Data, "expected at least 1 transaction")

	tx := resp.Data[0]
	assert.NotEmpty(t, tx.TransactionID, "expected non-empty transaction ID")
	assert.NotZero(t, tx.BlockNumber, "expected non-zero block number")
	t.Logf("got %d transactions, first txid=%s block=%d", len(resp.Data), tx.TransactionID, tx.BlockNumber)

	// Test pagination metadata
	t.Logf("meta: page_size=%d fingerprint=%q", resp.Meta.PageSize, resp.Meta.Fingerprint)
}

func TestIntegrationGetTRC20Transfers(t *testing.T) {
	skipUnlessIntegration(t)

	c := NewClient("nile", os.Getenv("GOTRON_NODE_API_KEY"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.GetTRC20Transfers(ctx, nileTestAddress, QueryOpts{Limit: 3})
	require.NoError(t, err, "GetTRC20Transfers")
	require.True(t, resp.Success, "expected success=true")
	require.NotEmpty(t, resp.Data, "expected at least 1 TRC20 transfer")

	tr := resp.Data[0]
	assert.NotEmpty(t, tr.TransactionID, "expected non-empty transaction ID")
	assert.NotEmpty(t, tr.TokenInfo.Symbol, "expected non-empty token symbol")
	assert.NotEmpty(t, tr.From, "expected non-empty from address")
	assert.NotEmpty(t, tr.To, "expected non-empty to address")
	t.Logf("got %d transfers, first: %s %s from=%s to=%s", len(resp.Data), tr.TokenInfo.Symbol, tr.Value, tr.From, tr.To)
}

func TestIntegrationGetContractEvents(t *testing.T) {
	skipUnlessIntegration(t)

	c := NewClient("nile", os.Getenv("GOTRON_NODE_API_KEY"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.GetContractEvents(ctx, nileTestContract, QueryOpts{Limit: 3})
	require.NoError(t, err, "GetContractEvents")
	require.True(t, resp.Success, "expected success=true")
	require.NotEmpty(t, resp.Data, "expected at least 1 event")

	ev := resp.Data[0]
	assert.NotEmpty(t, ev.EventName, "expected non-empty event name")
	assert.NotEmpty(t, ev.TransactionID, "expected non-empty transaction ID")
	t.Logf("got %d events, first: %s txid=%s", len(resp.Data), ev.EventName, ev.TransactionID)
}

func TestIntegrationGetContractEventsByName(t *testing.T) {
	skipUnlessIntegration(t)

	c := NewClient("nile", os.Getenv("GOTRON_NODE_API_KEY"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.GetContractEventsByName(ctx, nileTestContract, "Transfer", QueryOpts{Limit: 2})
	require.NoError(t, err, "GetContractEventsByName")
	require.True(t, resp.Success, "expected success=true")
	require.NotEmpty(t, resp.Data, "expected at least 1 Transfer event")
	for _, ev := range resp.Data {
		assert.Equal(t, "Transfer", ev.EventName)
	}
	t.Logf("got %d Transfer events", len(resp.Data))
}

func TestIntegrationPagination(t *testing.T) {
	skipUnlessIntegration(t)

	c := NewClient("nile", os.Getenv("GOTRON_NODE_API_KEY"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Page 1
	resp1, err := c.GetTransactionsByAddress(ctx, nileTestAddress, QueryOpts{Limit: 2})
	require.NoError(t, err, "page 1")
	require.True(t, resp1.Success, "expected success=true on page 1")
	if resp1.Meta.Fingerprint == "" {
		t.Skip("address has <= 2 transactions, cannot test pagination")
	}
	require.NotEmpty(t, resp1.Data, "expected data on page 1")

	// Page 2
	resp2, err := c.GetTransactionsByAddress(ctx, nileTestAddress, QueryOpts{
		Limit:       2,
		Fingerprint: resp1.Meta.Fingerprint,
	})
	require.NoError(t, err, "page 2")
	require.True(t, resp2.Success, "expected success=true on page 2")
	require.NotEmpty(t, resp2.Data, "expected data on page 2")

	// Verify different transactions
	assert.NotEqual(t, resp1.Data[0].TransactionID, resp2.Data[0].TransactionID, "page 1 and page 2 returned the same first transaction")
	t.Logf("page 1: %d txs (fp=%s), page 2: %d txs", len(resp1.Data), resp1.Meta.Fingerprint, len(resp2.Data))
}
