package policy

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/approval"
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
	result, err := e.Check(context.Background(), &Intent{WalletName: "any", AmountSUN: 1000000000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_NoPolicy(t *testing.T) {
	e := newTestEngine(t, &Config{Wallets: map[string]*WalletPolicy{}})
	result, err := e.Check(context.Background(), &Intent{WalletName: "unknown", AmountSUN: 999999999999, TokenID: "TRX"})
	require.NoError(t, err)
	assert.True(t, result.Allowed, "no policy = unrestricted")
}

func TestCheck_PerTxLimit_Allowed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {PerTxLimitTRX: 100},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", AmountSUN: 50_000_000, TokenID: "TRX", TokenAmount: 50_000_000})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_PerTxLimit_Denied(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {PerTxLimitTRX: 100},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", AmountSUN: 200_000_000, TokenID: "TRX", TokenAmount: 200_000_000})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "per-tx limit")
}

func TestCheck_DailyLimit_Allowed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {DailyLimitTRX: 500},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", AmountSUN: 400_000_000, TokenID: "TRX", TokenAmount: 400_000_000})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_DailyLimit_Denied(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {DailyLimitTRX: 500},
	}}
	e := newTestEngine(t, cfg)

	// First TX: 400 TRX — allowed (atomically reserved)
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", AmountSUN: 400_000_000, TokenID: "TRX", TokenAmount: 400_000_000})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Second TX: 200 TRX — would exceed 500 daily limit
	result, err = e.Check(context.Background(), &Intent{WalletName: "wallet", AmountSUN: 200_000_000, TokenID: "TRX", TokenAmount: 200_000_000})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "daily TRX limit")
}

func TestCheck_Whitelist_Allowed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {Whitelist: []string{"TAllowed1", "TAllowed2"}},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", ToAddr: "TAllowed1", AmountSUN: 1_000_000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_Whitelist_Denied(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {Whitelist: []string{"TAllowed1"}},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", ToAddr: "TNotAllowed", AmountSUN: 1_000_000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "not in the whitelist")
}

func TestCheck_Whitelist_EmptyToAddr(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {Whitelist: []string{"TAllowed1"}},
	}}
	e := newTestEngine(t, cfg)
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", ToAddr: "", AmountSUN: 1_000_000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_ApprovalRequired(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {ApprovalRequiredAboveTRX: 500},
	}}
	e := newTestEngine(t, cfg)

	// Below threshold
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", AmountSUN: 400_000_000, TokenID: "TRX"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Above threshold
	result, err = e.Check(context.Background(), &Intent{WalletName: "wallet", AmountSUN: 600_000_000, TokenID: "TRX"})
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
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", TokenID: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", TokenAmount: 500, ToAddr: "TAnywhere"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// 2000 USDT — denied
	result, err = e.Check(context.Background(), &Intent{WalletName: "wallet", TokenID: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", TokenAmount: 2000, ToAddr: "TAnywhere"})
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
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", TokenID: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", TokenAmount: 800, ToAddr: "TAnywhere"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// 300 USDT — would exceed 1000 daily
	result, err = e.Check(context.Background(), &Intent{WalletName: "wallet", TokenID: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", TokenAmount: 300, ToAddr: "TAnywhere"})
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
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", TokenID: "TRX", TokenAmount: 200_000_000, AmountSUN: 200_000_000})
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
	result, err := e.Check(context.Background(), &Intent{WalletName: "wallet", ToAddr: "TAllowed", AmountSUN: 100_000_000, TokenID: "TRX", TokenAmount: 100_000_000})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Whitelist denied
	result, err = e.Check(context.Background(), &Intent{WalletName: "wallet", ToAddr: "TNotAllowed", AmountSUN: 100_000_000, TokenID: "TRX", TokenAmount: 100_000_000})
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
	result, err := e.Check(context.Background(), intent)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Another 80 TRX should be denied (80+80 > 100)
	intent2 := &Intent{WalletName: "wallet", AmountSUN: 80_000_000, TokenID: "TRX", TokenAmount: 80_000_000}
	result, err = e.Check(context.Background(), intent2)
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// Release the first reservation
	e.ReleaseReserve(intent)

	// Now 80 TRX should be allowed again
	intent3 := &Intent{WalletName: "wallet", AmountSUN: 80_000_000, TokenID: "TRX", TokenAmount: 80_000_000}
	result, err = e.Check(context.Background(), intent3)
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
	result, err := e.Check(context.Background(), intent)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Record some USDT spend
	usdtIntent := &Intent{WalletName: "wallet", TokenID: "USDT", TokenAmount: 100_000_000, ToAddr: "TAnywhere"}
	result, err = e.Check(context.Background(), usdtIntent)
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
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		TokenID:     "USDT",
		TokenAmount: 50_000_000, // 50 USDT in raw units
		ToAddr:      "TAnywhere",
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Above threshold: 200 USDT (200 * 10^6 raw units)
	result, err = e.Check(context.Background(), &Intent{
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

	result, err := e.Check(context.Background(), &Intent{
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
	result, err := e.Check(context.Background(), intent)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Another 80 USDT should be denied (80+80 > 100 * 10^6)
	intent2 := &Intent{
		WalletName:  "wallet",
		TokenID:     "USDT",
		TokenAmount: 80_000_000,
		ToAddr:      "TAnywhere",
	}
	result, err = e.Check(context.Background(), intent2)
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
	result, err = e.Check(context.Background(), intent3)
	require.NoError(t, err)
	assert.True(t, result.Allowed, "budget should be restored after ReleaseReserve for token limits")
}

// --- Approval and notification tests ---

type mockApprover struct {
	approved bool
	err      error
}

func (m *mockApprover) RequestApproval(_ context.Context, _ approval.Request) (approval.Result, error) {
	if m.err != nil {
		return approval.Result{}, m.err
	}
	return approval.Result{Approved: m.approved, ApprovedBy: "test", Timestamp: time.Now()}, nil
}

type mockNotifier struct {
	mockApprover
	notified bool
	txid     string
	success  bool
}

func (m *mockNotifier) NotifyBroadcast(_ context.Context, txid string, success bool) error {
	m.notified = true
	m.txid = txid
	m.success = success
	return nil
}

func TestRequestApproval_NilEngine(t *testing.T) {
	var e *Engine
	approved, err := e.RequestApproval(context.Background(), &Intent{WalletName: "any"})
	assert.False(t, approved)
	assert.NoError(t, err)
}

func TestRequestApproval_NoApprover(t *testing.T) {
	e := newTestEngine(t, &Config{})
	approved, err := e.RequestApproval(context.Background(), &Intent{WalletName: "any"})
	assert.False(t, approved)
	assert.NoError(t, err)
}

func TestRequestApproval_Approved(t *testing.T) {
	e := newTestEngine(t, &Config{})
	e.SetApprover(&mockApprover{approved: true})

	approved, err := e.RequestApproval(context.Background(), &Intent{WalletName: "wallet"})
	require.NoError(t, err)
	assert.True(t, approved)
}

func TestRequestApproval_Rejected(t *testing.T) {
	e := newTestEngine(t, &Config{})
	e.SetApprover(&mockApprover{approved: false})

	approved, err := e.RequestApproval(context.Background(), &Intent{WalletName: "wallet"})
	require.NoError(t, err)
	assert.False(t, approved)
}

func TestRequestApproval_UsesConfigTimeout(t *testing.T) {
	cfg := &Config{
		Approval: &ApprovalConfig{
			Telegram: &TelegramYAMLConfig{TimeoutSeconds: 60},
		},
	}
	e := newTestEngine(t, cfg)
	e.SetApprover(&mockApprover{approved: true})

	intent := &Intent{WalletName: "wallet"}
	approved, err := e.RequestApproval(context.Background(), intent)
	require.NoError(t, err)
	assert.True(t, approved)
}

func TestBuildApprovalSummary(t *testing.T) {
	t.Run("TransferContract", func(t *testing.T) {
		intent := &Intent{
			Action:    "TransferContract",
			AmountSUN: 100_000_000,
			ToAddr:    "TRecipient",
			TokenID:   "TRX",
		}
		summary := buildApprovalSummary(intent)
		assert.Contains(t, summary, "Transfer")
		assert.Contains(t, summary, "TRX")
		assert.Contains(t, summary, "TRecipient")
	})

	t.Run("TriggerSmartContract_TRC20", func(t *testing.T) {
		intent := &Intent{
			Action:  "TriggerSmartContract",
			ToAddr:  "TContract",
			TokenID: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		}
		summary := buildApprovalSummary(intent)
		assert.Contains(t, summary, "Token transfer")
		assert.Contains(t, summary, "TContract")
		assert.Contains(t, summary, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t")
	})

	t.Run("TriggerSmartContract_NativeCall", func(t *testing.T) {
		intent := &Intent{
			Action:  "TriggerSmartContract",
			ToAddr:  "TContract",
			TokenID: "TRX",
		}
		summary := buildApprovalSummary(intent)
		assert.Contains(t, summary, "Contract call")
		assert.Contains(t, summary, "TContract")
	})

	t.Run("UnknownType", func(t *testing.T) {
		intent := &Intent{
			Action:     "FreezeBalanceV2Contract",
			WalletName: "mywallet",
		}
		summary := buildApprovalSummary(intent)
		assert.Contains(t, summary, "FreezeBalanceV2Contract")
		assert.Contains(t, summary, "mywallet")
	})
}

func TestNotifyBroadcast_NilEngine(t *testing.T) {
	var e *Engine
	assert.NotPanics(t, func() {
		e.NotifyBroadcast(context.Background(), "txid", true)
	})
}

func TestNotifyBroadcast_WithMockNotifier(t *testing.T) {
	e := newTestEngine(t, &Config{})
	mn := &mockNotifier{mockApprover: mockApprover{approved: true}}
	e.SetApprover(mn)

	e.NotifyBroadcast(context.Background(), "abc123", true)
	assert.True(t, mn.notified)
	assert.Equal(t, "abc123", mn.txid)
	assert.True(t, mn.success)

	// Test failure notification
	mn.notified = false
	e.NotifyBroadcast(context.Background(), "fail456", false)
	assert.True(t, mn.notified)
	assert.Equal(t, "fail456", mn.txid)
	assert.False(t, mn.success)
}

// --- capturingApprover captures the Request passed to RequestApproval ---

type capturingApprover struct {
	lastReq approval.Request
	result  bool
}

func (c *capturingApprover) RequestApproval(_ context.Context, req approval.Request) (approval.Result, error) {
	c.lastReq = req
	return approval.Result{Approved: c.result}, nil
}

func TestBuildSpendContext_WithTokenLimits(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			DailyLimitTRX: 1000,
			TokenLimits: map[string]*TokenLimit{
				"USDT": {Decimals: 6, DailyLimitUnits: 500},
			},
		},
	}}
	e := newTestEngine(t, cfg)

	// Record some USDT spend first
	usdtIntent := &Intent{WalletName: "wallet", TokenID: "USDT", TokenAmount: 100_000_000, ToAddr: "TAnywhere"}
	result, err := e.Check(context.Background(), usdtIntent)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Now request approval for another USDT transfer — capture the Request
	cap := &capturingApprover{result: true}
	e.SetApprover(cap)

	intent := &Intent{WalletName: "wallet", TokenID: "USDT", TokenAmount: 50_000_000, ToAddr: "TAnywhere"}
	_, err = e.RequestApproval(context.Background(), intent)
	require.NoError(t, err)
	assert.NotEmpty(t, cap.lastReq.SpendContext)
	assert.Contains(t, cap.lastReq.SpendContext, "Daily USDT:")
	assert.Contains(t, cap.lastReq.SpendContext, "/ 500")
}

func TestBuildSpendContext_NoStore(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {DailyLimitTRX: 1000},
	}}
	e := NewEngine(cfg, nil) // nil store

	cap := &capturingApprover{result: true}
	e.SetApprover(cap)

	intent := &Intent{WalletName: "wallet", TokenID: "TRX", AmountSUN: 100_000_000}
	_, err := e.RequestApproval(context.Background(), intent)
	require.NoError(t, err)
	assert.Empty(t, cap.lastReq.SpendContext)
}

func TestBuildSpendContext_NoPolicy(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{}}
	e := newTestEngine(t, cfg)

	cap := &capturingApprover{result: true}
	e.SetApprover(cap)

	intent := &Intent{WalletName: "unknown", TokenID: "TRX", AmountSUN: 100_000_000}
	_, err := e.RequestApproval(context.Background(), intent)
	require.NoError(t, err)
	assert.Empty(t, cap.lastReq.SpendContext)
}

func TestBuildSpendContext_LegacyTRX(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {DailyLimitTRX: 500},
	}}
	e := newTestEngine(t, cfg)

	// Spend some TRX first
	trxIntent := &Intent{WalletName: "wallet", TokenID: "TRX", AmountSUN: 200_000_000, TokenAmount: 200_000_000}
	result, err := e.Check(context.Background(), trxIntent)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	cap := &capturingApprover{result: true}
	e.SetApprover(cap)

	intent := &Intent{WalletName: "wallet", TokenID: "TRX", AmountSUN: 50_000_000}
	_, err = e.RequestApproval(context.Background(), intent)
	require.NoError(t, err)
	assert.NotEmpty(t, cap.lastReq.SpendContext)
	assert.Contains(t, cap.lastReq.SpendContext, "Daily TRX:")
	assert.Contains(t, cap.lastReq.SpendContext, "/ 500")
}

func TestHasApprover_True(t *testing.T) {
	e := newTestEngine(t, &Config{})
	e.SetApprover(&mockApprover{approved: true})
	assert.True(t, e.HasApprover())
}

func TestHasApprover_False(t *testing.T) {
	e := newTestEngine(t, &Config{})
	assert.False(t, e.HasApprover())
}

func TestHasApprover_NilEngine(t *testing.T) {
	var e *Engine
	assert.False(t, e.HasApprover())
}

func TestNotifyBroadcast_NoNotifier(t *testing.T) {
	e := newTestEngine(t, &Config{})
	// Set a plain approver that does NOT implement Notifier
	e.SetApprover(&mockApprover{approved: true})
	assert.NotPanics(t, func() {
		e.NotifyBroadcast(context.Background(), "txid123", true)
	})
}

func TestCheck_ApprovalAfterDailyReserve(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			DailyLimitTRX:            1000,
			ApprovalRequiredAboveTRX: 100,
		},
	}}
	e := newTestEngine(t, cfg)

	// Send 200 TRX — above approval threshold, should trigger ApprovalRequired
	intent := &Intent{WalletName: "wallet", AmountSUN: 200_000_000, TokenID: "TRX", TokenAmount: 200_000_000}
	result, err := e.Check(context.Background(), intent)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.True(t, result.ApprovalRequired)

	// Verify the daily spend WAS reserved even though approval is required
	budget := e.GetRemainingBudget("wallet")
	require.NotNil(t, budget)
	assert.Equal(t, float64(200), budget["trx_spent_today"], "daily spend should be reserved before approval flag is returned")
	assert.Equal(t, float64(800), budget["trx_remaining_today"])
}

// --- USD limit tests ---

// mockPricer implements PriceProvider for testing.
type mockPricer struct {
	prices map[string]float64
	err    error
}

func (m *mockPricer) GetTokenPrice(_ context.Context, tokenID string) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	if p, ok := m.prices[tokenID]; ok {
		return p, nil
	}
	return 0, fmt.Errorf("no price for %s", tokenID)
}

func TestCheck_PerTxLimitUSD_Allowed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {PerTxLimitUSD: 10}, // $10 per-TX limit
	}}
	e := newTestEngine(t, cfg)
	e.SetPriceProvider(&mockPricer{prices: map[string]float64{"TRX": 0.10}})

	// 50 TRX * $0.10 = $5 — below $10 limit
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		AmountSUN:   50_000_000,
		TokenID:     "TRX",
		TokenAmount: 50_000_000,
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_PerTxLimitUSD_Denied(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {PerTxLimitUSD: 10},
	}}
	e := newTestEngine(t, cfg)
	e.SetPriceProvider(&mockPricer{prices: map[string]float64{"TRX": 0.10}})

	// 200 TRX * $0.10 = $20 — above $10 limit
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		AmountSUN:   200_000_000,
		TokenID:     "TRX",
		TokenAmount: 200_000_000,
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "per-TX USD limit")
}

func TestCheck_PerTxLimitUSD_FailClosed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {PerTxLimitUSD: 10},
	}}
	e := newTestEngine(t, cfg)
	e.SetPriceProvider(&mockPricer{err: fmt.Errorf("CoinGecko rate limited")})

	// Price fetch fails — should deny (fail-closed)
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		AmountSUN:   50_000_000,
		TokenID:     "TRX",
		TokenAmount: 50_000_000,
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "price unavailable")
}

func TestCheck_ApprovalRequiredAboveUSD_BelowThreshold(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {ApprovalRequiredAboveUSD: 5},
	}}
	e := newTestEngine(t, cfg)
	e.SetPriceProvider(&mockPricer{prices: map[string]float64{"TRX": 0.10}})

	// 20 TRX * $0.10 = $2 — below $5 threshold
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		AmountSUN:   20_000_000,
		TokenID:     "TRX",
		TokenAmount: 20_000_000,
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_ApprovalRequiredAboveUSD_AboveThreshold(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {ApprovalRequiredAboveUSD: 5},
	}}
	e := newTestEngine(t, cfg)
	e.SetPriceProvider(&mockPricer{prices: map[string]float64{"TRX": 0.10}})

	// 100 TRX * $0.10 = $10 — above $5 threshold
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		AmountSUN:   100_000_000,
		TokenID:     "TRX",
		TokenAmount: 100_000_000,
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.True(t, result.ApprovalRequired)
	assert.Contains(t, result.Reason, "USD approval threshold")
}

func TestCheck_ApprovalRequiredAboveUSD_FailClosed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {ApprovalRequiredAboveUSD: 5},
	}}
	e := newTestEngine(t, cfg)
	e.SetPriceProvider(&mockPricer{err: fmt.Errorf("network error")})

	// Price fetch fails — should deny (fail-closed)
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		AmountSUN:   100_000_000,
		TokenID:     "TRX",
		TokenAmount: 100_000_000,
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "price unavailable")
}

func TestCheck_USDLimits_NoPriceProvider_FailClosed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			PerTxLimitUSD: 10,
		},
	}}
	e := newTestEngine(t, cfg)
	// No price provider set — USD checks should fail-closed

	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		AmountSUN:   50_000_000,
		TokenID:     "TRX",
		TokenAmount: 50_000_000,
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed, "USD checks must fail-closed when no price provider")
	assert.Contains(t, result.Reason, "price unavailable")
}

func TestCheck_USDApproval_NoPriceProvider_FailClosed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			ApprovalRequiredAboveUSD: 5,
		},
	}}
	e := newTestEngine(t, cfg)

	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		AmountSUN:   50_000_000,
		TokenID:     "TRX",
		TokenAmount: 50_000_000,
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed, "USD approval threshold must fail-closed when no price provider")
	assert.Contains(t, result.Reason, "price unavailable")
}

func TestCheck_NoUSDLimits_NoPriceProvider_Allowed(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {PerTxLimitTRX: 1000},
	}}
	e := newTestEngine(t, cfg)
	// No USD limits configured, no price provider — should be fine

	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		AmountSUN:   50_000_000,
		TokenID:     "TRX",
		TokenAmount: 50_000_000,
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed, "no USD limits = no price provider needed")
}

func TestCheck_USDLimits_ZeroDecimalToken(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			PerTxLimitUSD: 100,
			TokenLimits: map[string]*TokenLimit{
				"NODEC": {Decimals: 0, PerTxLimitUnits: 10000},
			},
		},
	}}
	e := newTestEngine(t, cfg)
	e.SetPriceProvider(&mockPricer{prices: map[string]float64{"NODEC": 0.50}})

	// 100 raw units * $0.50 = $50 — below $100 USD limit
	// decimalMultiplier(0) = 1, so humanAmount = 100 / 1 = 100
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		TokenID:     "NODEC",
		TokenAmount: 100,
		ToAddr:      "TAnywhere",
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed, "zero-decimal token with configured entry should be allowed")
}

func TestCheck_USDLimits_UnknownDecimals(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {PerTxLimitUSD: 10},
	}}
	e := newTestEngine(t, cfg)
	e.SetPriceProvider(&mockPricer{prices: map[string]float64{"TUnknownToken": 1.0}})

	// Token with no decimals info — should deny (cannot compute USD)
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		TokenID:     "TUnknownToken",
		TokenAmount: 1_000_000,
		ToAddr:      "TAnywhere",
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "unknown decimals")
}

func TestCheck_PerTxLimitUSD_TRC20WithDecimals(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			PerTxLimitUSD: 100,
			TokenLimits: map[string]*TokenLimit{
				"USDT": {Decimals: 6, PerTxLimitUnits: 10000},
			},
		},
	}}
	e := newTestEngine(t, cfg)
	e.SetPriceProvider(&mockPricer{prices: map[string]float64{"USDT": 1.0}})

	// 50 USDT (50 * 10^6 raw) * $1.00 = $50 — below $100 USD limit
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		TokenID:     "USDT",
		TokenAmount: 50_000_000,
		ToAddr:      "TAnywhere",
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// 200 USDT (200 * 10^6 raw) * $1.00 = $200 — above $100 USD limit
	result, err = e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		TokenID:     "USDT",
		TokenAmount: 200_000_000,
		ToAddr:      "TAnywhere",
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "per-TX USD limit")
}

func TestCheck_PerTxLimitUSD_NoReservationLeak(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"wallet": {
			PerTxLimitUSD: 5,
			DailyLimitTRX: 1000,
		},
	}}
	e := newTestEngine(t, cfg)
	e.SetPriceProvider(&mockPricer{prices: map[string]float64{"TRX": 0.10}})

	// 100 TRX * $0.10 = $10 — above $5 per-TX USD limit → denied
	result, err := e.Check(context.Background(), &Intent{
		WalletName:  "wallet",
		AmountSUN:   100_000_000,
		TokenID:     "TRX",
		TokenAmount: 100_000_000,
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// Daily budget should NOT have been consumed (USD check runs before stateful)
	budget := e.GetRemainingBudget("wallet")
	require.NotNil(t, budget)
	assert.Equal(t, float64(0), budget["trx_spent_today"], "no daily spend should be reserved when USD per-TX check denies")
	assert.Equal(t, float64(1000), budget["trx_remaining_today"])
}
