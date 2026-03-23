package policy

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEngine(t *testing.T, cfg *Config) *Engine {
	t.Helper()
	store := newTestStore(t)
	return NewEngine(cfg, store)
}

func TestCheck_NilEngine(t *testing.T) {
	var e *Engine
	result, err := e.Check(&Intent{WalletName: "any", AmountSUN: 1000000000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_NoPolicy(t *testing.T) {
	e := newTestEngine(t, &Config{Wallets: map[string]*WalletPolicy{}})
	result, err := e.Check(&Intent{WalletName: "unknown", AmountSUN: 999999999999, TokenID: "TRX"})
	require.NoError(t, err)
	assert.True(t, result.Allowed, "no policy = unrestricted")
}

func TestCheck_PerTxLimit_Allowed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {PerTxLimitTRX: 100},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(&Intent{WalletName: "wallet", AmountSUN: 50_000_000, TokenID: "TRX", TokenAmount: 50_000_000})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_PerTxLimit_Denied(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {PerTxLimitTRX: 100},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(&Intent{WalletName: "wallet", AmountSUN: 200_000_000, TokenID: "TRX", TokenAmount: 200_000_000})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "per-tx limit")
}

func TestCheck_DailyLimit_Allowed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {DailyLimitTRX: 500},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(&Intent{WalletName: "wallet", AmountSUN: 400_000_000, TokenID: "TRX", TokenAmount: 400_000_000})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_DailyLimit_Denied(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {DailyLimitTRX: 500},
	}}
	e := newTestEngine(t, cfg)

	// First TX: 400 TRX — allowed (atomically reserved)
	result, err := e.Check(&Intent{WalletName: "wallet", AmountSUN: 400_000_000, TokenID: "TRX", TokenAmount: 400_000_000})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Second TX: 200 TRX — would exceed 500 daily limit
	result, err = e.Check(&Intent{WalletName: "wallet", AmountSUN: 200_000_000, TokenID: "TRX", TokenAmount: 200_000_000})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "daily TRX limit")
}

func TestCheck_Whitelist_Allowed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {Whitelist: []string{"TAllowed1", "TAllowed2"}},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(&Intent{WalletName: "wallet", ToAddr: "TAllowed1", AmountSUN: 1_000_000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_Whitelist_Denied(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {Whitelist: []string{"TAllowed1"}},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(&Intent{WalletName: "wallet", ToAddr: "TNotAllowed", AmountSUN: 1_000_000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "not in the whitelist")
}

func TestCheck_Whitelist_EmptyToAddr(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {Whitelist: []string{"TAllowed1"}},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(&Intent{WalletName: "wallet", ToAddr: "", AmountSUN: 1_000_000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_ApprovalRequired(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {ApprovalRequiredAboveTRX: 500},
	}}
	e := newTestEngine(t, cfg)

	// Below threshold
	result, err := e.Check(&Intent{WalletName: "wallet", AmountSUN: 400_000_000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Above threshold
	result, err = e.Check(&Intent{WalletName: "wallet", AmountSUN: 600_000_000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.True(t, result.ApprovalRequired)
}

func TestCheck_TokenLimits_PerTx(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			TokenLimits: map[string]*TokenLimit{
				"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t": { // USDT
					PerTxLimitUnits: 1000,
				},
			},
		},
	}}
	e := newTestEngine(t, cfg)

	// 500 USDT — allowed
	result, err := e.Check(&Intent{WalletName: "wallet", TokenID: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", TokenAmount: 500, ToAddr: "TAnywhere"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// 2000 USDT — denied
	result, err = e.Check(&Intent{WalletName: "wallet", TokenID: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", TokenAmount: 2000, ToAddr: "TAnywhere"})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "per-tx token limit")
}

func TestCheck_TokenLimits_Daily(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			TokenLimits: map[string]*TokenLimit{
				"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t": {
					DailyLimitUnits: 1000,
				},
			},
		},
	}}
	e := newTestEngine(t, cfg)

	// 800 USDT — allowed
	result, err := e.Check(&Intent{WalletName: "wallet", TokenID: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", TokenAmount: 800, ToAddr: "TAnywhere"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// 300 USDT — would exceed 1000 daily
	result, err = e.Check(&Intent{WalletName: "wallet", TokenID: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", TokenAmount: 300, ToAddr: "TAnywhere"})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "daily token limit")
}

func TestCheck_LegacyTRXPromotedToTokenLimits(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			PerTxLimitTRX: 100,
			DailyLimitTRX: 500,
		},
	}}
	// promoteLegacyTRXLimits should have created token_limits.TRX
	promoteLegacyTRXLimits(cfg.Wallets["wallet"])
	e := newTestEngine(t, cfg)

	// Per-TX check via token limits
	result, err := e.Check(&Intent{WalletName: "wallet", TokenID: "TRX", TokenAmount: 200_000_000, AmountSUN: 200_000_000})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "per-tx token limit")
}

func TestCheck_CombinedPolicies(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			PerTxLimitTRX:            1000,
			DailyLimitTRX:            5000,
			Whitelist:                []string{"TAllowed"},
			ApprovalRequiredAboveTRX: 500,
		},
	}}
	e := newTestEngine(t, cfg)

	// All pass, below approval
	result, err := e.Check(&Intent{WalletName: "wallet", ToAddr: "TAllowed", AmountSUN: 100_000_000, TokenID: "TRX", TokenAmount: 100_000_000})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Whitelist denied
	result, err = e.Check(&Intent{WalletName: "wallet", ToAddr: "TNotAllowed", AmountSUN: 100_000_000, TokenID: "TRX", TokenAmount: 100_000_000})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "not in the whitelist")
}

func TestRecordAudit_NilEngine(t *testing.T) {
	var e *Engine
	assert.NoError(t, e.RecordAudit(&Intent{WalletName: "any"}, "txid"))
}

func TestRecordAudit_Success(t *testing.T) {
	e := newTestEngine(t, &Config{Wallets: map[string]*WalletPolicy{}})
	require.NoError(t, e.RecordAudit(&Intent{
		WalletName: "wallet",
		Action:     "TransferContract",
		FromAddr:   "TFrom",
		ToAddr:     "TTo",
		AmountSUN:  1_000_000,
		TokenID:    "TRX",
	}, "txid123"))
}

func TestReleaseReserve_RestoresBudget(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {DailyLimitTRX: 100},
	}}
	e := newTestEngine(t, cfg)

	intent := &Intent{WalletName: "wallet", AmountSUN: 80_000_000, TokenID: "TRX", TokenAmount: 80_000_000}

	// Check reserves 80 TRX
	result, err := e.Check(intent)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Another 80 TRX should be denied (80+80 > 100)
	intent2 := &Intent{WalletName: "wallet", AmountSUN: 80_000_000, TokenID: "TRX", TokenAmount: 80_000_000}
	result, err = e.Check(intent2)
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// Release the first reservation
	e.ReleaseReserve(intent)

	// Now 80 TRX should be allowed again
	intent3 := &Intent{WalletName: "wallet", AmountSUN: 80_000_000, TokenID: "TRX", TokenAmount: 80_000_000}
	result, err = e.Check(intent3)
	require.NoError(t, err)
	assert.True(t, result.Allowed, "budget should be restored after ReleaseReserve")
}

func TestReleaseReserve_NilEngine(t *testing.T) {
	var e *Engine
	e.ReleaseReserve(&Intent{WalletName: "any"}) // should not panic
}

func TestEngine_Close_Nil(t *testing.T) {
	var e *Engine
	assert.NoError(t, e.Close())
}

func TestGetPolicy(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"savings": {PerTxLimitTRX: 100},
	}}
	e := newTestEngine(t, cfg)

	t.Run("KnownWallet", func(t *testing.T) {
		wp := e.GetPolicy("savings")
		require.NotNil(t, wp)
		assert.Equal(t, float64(100), wp.PerTxLimitTRX)
	})

	t.Run("UnknownWallet", func(t *testing.T) {
		assert.Nil(t, e.GetPolicy("unknown"))
	})

	t.Run("NilEngine", func(t *testing.T) {
		var nilEngine *Engine
		assert.Nil(t, nilEngine.GetPolicy("anything"))
	})
}

func TestGetRemainingBudget(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			DailyLimitTRX: 1000,
			TokenLimits: map[string]*TokenLimit{
				"USDT": {Decimals: 6, DailyLimitUnits: 500},
			},
		},
	}}
	e := newTestEngine(t, cfg)

	// Record some TRX spend
	intent := &Intent{WalletName: "wallet", AmountSUN: 200_000_000, TokenID: "TRX", TokenAmount: 200_000_000}
	result, err := e.Check(intent)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Record some USDT spend
	usdtIntent := &Intent{WalletName: "wallet", TokenID: "USDT", TokenAmount: 100_000_000, ToAddr: "TAnywhere"}
	result, err = e.Check(usdtIntent)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	budget := e.GetRemainingBudget("wallet")
	require.NotNil(t, budget)

	// TRX: spent 200 TRX of 1000 daily limit
	assert.Equal(t, float64(200), budget["trx_spent_today"])
	assert.Equal(t, float64(800), budget["trx_remaining_today"])

	// USDT: spent 100 USDT of 500 daily limit
	assert.Equal(t, float64(100), budget["USDT_spent_today"])
	assert.Equal(t, float64(400), budget["USDT_remaining_today"])
	assert.Equal(t, float64(500), budget["USDT_daily_limit"])
}

func TestGetRemainingBudget_NilEngine(t *testing.T) {
	var e *Engine
	assert.Nil(t, e.GetRemainingBudget("any"))
}

func TestGetRemainingBudget_UnknownWallet(t *testing.T) {
	e := newTestEngine(t, &Config{Wallets: map[string]*WalletPolicy{}})
	assert.Nil(t, e.GetRemainingBudget("unknown"))
}

func TestCheck_TokenApprovalThreshold(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			TokenLimits: map[string]*TokenLimit{
				"USDT": {
					Decimals:                   6,
					ApprovalRequiredAboveUnits: 100,
				},
			},
		},
	}}
	e := newTestEngine(t, cfg)

	// Below threshold: 50 USDT (50 * 10^6 raw units)
	result, err := e.Check(&Intent{
		WalletName:  "wallet",
		TokenID:     "USDT",
		TokenAmount: 50_000_000, // 50 USDT in raw units
		ToAddr:      "TAnywhere",
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Above threshold: 200 USDT (200 * 10^6 raw units)
	result, err = e.Check(&Intent{
		WalletName:  "wallet",
		TokenID:     "USDT",
		TokenAmount: 200_000_000, // 200 USDT in raw units
		ToAddr:      "TAnywhere",
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.True(t, result.ApprovalRequired)
	assert.Contains(t, result.Reason, "approval threshold")
}

func TestCheck_OverflowAmount(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			TokenLimits: map[string]*TokenLimit{
				"USDT": {Decimals: 6},
			},
		},
	}}
	e := newTestEngine(t, cfg)

	result, err := e.Check(&Intent{
		WalletName:  "wallet",
		TokenID:     "USDT",
		TokenAmount: float64(math.MaxInt64) * 2,
		ToAddr:      "TAnywhere",
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "too large to track safely")
}

func TestReleaseReserve_TokenLimits(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			TokenLimits: map[string]*TokenLimit{
				"USDT": {Decimals: 6, DailyLimitUnits: 100},
			},
		},
	}}
	e := newTestEngine(t, cfg)

	// Reserve 80 USDT (in raw units: 80 * 10^6)
	intent := &Intent{
		WalletName:  "wallet",
		TokenID:     "USDT",
		TokenAmount: 80_000_000,
		ToAddr:      "TAnywhere",
	}
	result, err := e.Check(intent)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Another 80 USDT should be denied (80+80 > 100 * 10^6)
	intent2 := &Intent{
		WalletName:  "wallet",
		TokenID:     "USDT",
		TokenAmount: 80_000_000,
		ToAddr:      "TAnywhere",
	}
	result, err = e.Check(intent2)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "daily token limit")

	// Release the first reservation
	e.ReleaseReserve(intent)

	// Now 80 USDT should be allowed again
	intent3 := &Intent{
		WalletName:  "wallet",
		TokenID:     "USDT",
		TokenAmount: 80_000_000,
		ToAddr:      "TAnywhere",
	}
	result, err = e.Check(intent3)
	require.NoError(t, err)
	assert.True(t, result.Allowed, "budget should be restored after ReleaseReserve for token limits")
}
