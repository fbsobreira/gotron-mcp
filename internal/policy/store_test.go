package policy

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test-state.db")
	s, err := NewStore(path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_DailySpend(t *testing.T) {
	s := newTestStore(t)
	today := time.Now().UTC()

	// Initially zero
	spent, err := s.GetDailySpend("wallet1", today)
	require.NoError(t, err)
	assert.Equal(t, int64(0), spent)

	// Add some spend
	require.NoError(t, s.AddDailySpend("wallet1", today, 1000000))
	spent, err = s.GetDailySpend("wallet1", today)
	require.NoError(t, err)
	assert.Equal(t, int64(1000000), spent)

	// Add more — cumulative
	require.NoError(t, s.AddDailySpend("wallet1", today, 2000000))
	spent, err = s.GetDailySpend("wallet1", today)
	require.NoError(t, err)
	assert.Equal(t, int64(3000000), spent)
}

func TestStore_DailySpend_DateRollover(t *testing.T) {
	s := newTestStore(t)
	today := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)

	require.NoError(t, s.AddDailySpend("wallet1", yesterday, 5000000))
	require.NoError(t, s.AddDailySpend("wallet1", today, 1000000))

	// Yesterday's spend
	spent, err := s.GetDailySpend("wallet1", yesterday)
	require.NoError(t, err)
	assert.Equal(t, int64(5000000), spent)

	// Today's spend — independent
	spent, err = s.GetDailySpend("wallet1", today)
	require.NoError(t, err)
	assert.Equal(t, int64(1000000), spent)
}

func TestStore_DailySpend_SeparateWallets(t *testing.T) {
	s := newTestStore(t)
	today := time.Now().UTC()

	require.NoError(t, s.AddDailySpend("wallet1", today, 1000000))
	require.NoError(t, s.AddDailySpend("wallet2", today, 9000000))

	spent1, err := s.GetDailySpend("wallet1", today)
	require.NoError(t, err)
	assert.Equal(t, int64(1000000), spent1)

	spent2, err := s.GetDailySpend("wallet2", today)
	require.NoError(t, err)
	assert.Equal(t, int64(9000000), spent2)
}

func TestStore_Audit(t *testing.T) {
	s := newTestStore(t)

	entry := AuditEntry{
		Timestamp:  time.Now().UTC(),
		Action:     "TransferContract",
		From:       "TFrom...",
		To:         "TTo...",
		AmountSUN:  1000000,
		WalletName: "savings",
		TxID:       "abc123",
	}
	require.NoError(t, s.RecordAudit(entry))
}

func TestStore_Close_Nil(t *testing.T) {
	var s *Store
	assert.NoError(t, s.Close())
}
