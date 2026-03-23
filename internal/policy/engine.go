package policy

import (
	"fmt"
	"math"
	"time"
)

// CheckResult holds the outcome of a policy evaluation.
type CheckResult struct {
	Allowed          bool
	Reason           string
	ApprovalRequired bool
}

// Engine evaluates transaction intents against wallet policies.
type Engine struct {
	cfg   *Config
	store *Store
}

// NewEngine creates a policy engine with the given config and persistent store.
func NewEngine(cfg *Config, store *Store) *Engine {
	return &Engine{cfg: cfg, store: store}
}

// Close closes the underlying store. Safe to call on nil engine.
func (e *Engine) Close() error {
	if e == nil || e.store == nil {
		return nil
	}
	return e.store.Close()
}

// GetPolicy returns the policy for a wallet, or nil if unrestricted.
func (e *Engine) GetPolicy(wallet string) *WalletPolicy {
	if e == nil {
		return nil
	}
	return e.cfg.GetPolicy(wallet)
}

// GetRemainingBudget returns the remaining daily budget for a wallet.
func (e *Engine) GetRemainingBudget(wallet string) map[string]any {
	if e == nil || e.store == nil {
		return nil
	}
	wp := e.cfg.GetPolicy(wallet)
	if wp == nil {
		return nil
	}

	remaining := map[string]any{}
	now := time.Now().UTC()

	// Legacy TRX daily limit
	if wp.DailyLimitTRX > 0 {
		spent, _ := e.store.GetDailySpend(wallet+"/TRX", now)
		spentTRX := float64(spent) / 1_000_000
		remaining["trx_spent_today"] = spentTRX
		remaining["trx_remaining_today"] = wp.DailyLimitTRX - spentTRX
	}

	// Per-token daily limits (show in human units)
	for token, tl := range wp.TokenLimits {
		if tl.DailyLimitUnits > 0 {
			spendKey := fmt.Sprintf("%s/%s", wallet, token)
			spentRaw, _ := e.store.GetDailySpend(spendKey, now)
			mult := decimalMultiplier(tl.Decimals)
			spentHuman := float64(spentRaw) / mult
			remaining[token+"_spent_today"] = spentHuman
			remaining[token+"_remaining_today"] = tl.DailyLimitUnits - spentHuman
			remaining[token+"_daily_limit"] = tl.DailyLimitUnits
		}
	}

	return remaining
}

// Check evaluates an intent against the wallet's policy.
// Returns allowed if no policy exists for the wallet (unrestricted).
// Stateless checks (whitelist, approval threshold, per-TX) run first.
// Stateful checks (daily limits via CheckAndReserve) run last.
// Call ReleaseReserve if the transaction fails after Check returns allowed.
func (e *Engine) Check(intent *Intent) (*CheckResult, error) {
	if e == nil {
		return &CheckResult{Allowed: true}, nil
	}

	wp := e.cfg.GetPolicy(intent.WalletName)
	if wp == nil {
		return &CheckResult{Allowed: true}, nil
	}

	// === STATELESS checks first (no side effects) ===

	// 1. Whitelist (cheapest check, no state)
	if len(wp.Whitelist) > 0 && intent.ToAddr != "" {
		allowed := false
		for _, addr := range wp.Whitelist {
			if addr == intent.ToAddr {
				allowed = true
				break
			}
		}
		if !allowed {
			return &CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("destination %s is not in the whitelist for wallet %q", intent.ToAddr, intent.WalletName),
			}, nil
		}
	}

	// 2. Per-TX token unit limit (stateless, compared in raw on-chain units)
	if tl := wp.TokenLimits[intent.TokenID]; tl != nil {
		rawLimit := tl.RawPerTxLimit()
		if rawLimit > 0 && intent.TokenAmount > rawLimit {
			return &CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("transaction amount exceeds per-tx token limit of %.0f %s for wallet %q", tl.PerTxLimitUnits, intent.TokenID, intent.WalletName),
			}, nil
		}
		// Per-token approval threshold
		rawApproval := tl.RawApprovalThreshold()
		if rawApproval > 0 && intent.TokenAmount > rawApproval {
			return &CheckResult{
				Allowed:          false,
				ApprovalRequired: true,
				Reason:           fmt.Sprintf("transaction amount exceeds approval threshold of %.0f %s for wallet %q — approval required", tl.ApprovalRequiredAboveUnits, intent.TokenID, intent.WalletName),
			}, nil
		}
	}

	// 3. Legacy TRX per-TX limit (stateless, for configs without token_limits)
	if intent.TokenID == "TRX" && wp.PerTxLimitTRX > 0 {
		if _, exists := wp.TokenLimits["TRX"]; !exists {
			if intent.AmountTRX() > wp.PerTxLimitTRX {
				return &CheckResult{
					Allowed: false,
					Reason:  fmt.Sprintf("transaction amount %.6f TRX exceeds per-tx limit of %.0f TRX for wallet %q", intent.AmountTRX(), wp.PerTxLimitTRX, intent.WalletName),
				}, nil
			}
		}
	}

	// 4. Approval threshold (stateless — check before reserving spend)
	if intent.TokenID == "TRX" && wp.ApprovalRequiredAboveTRX > 0 && intent.AmountTRX() > wp.ApprovalRequiredAboveTRX {
		return &CheckResult{
			Allowed:          false,
			ApprovalRequired: true,
			Reason:           fmt.Sprintf("transaction amount %.6f TRX exceeds approval threshold of %.0f TRX for wallet %q — approval required", intent.AmountTRX(), wp.ApprovalRequiredAboveTRX, intent.WalletName),
		}, nil
	}
	// TODO: approval_required_above_usd (requires price service #85)

	// === STATEFUL checks (CheckAndReserve — has side effects) ===

	// Record check time for consistent rollback on the same UTC day
	checkTime := time.Now().UTC()
	intent.CheckTime = checkTime

	// Guard: reject token amounts that would overflow int64 in spend tracking
	if intent.TokenAmount > float64(math.MaxInt64) {
		return &CheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("token amount too large to track safely for wallet %q", intent.WalletName),
		}, nil
	}

	// 5. Per-token daily unit limit (compared in raw on-chain units)
	if tl := wp.TokenLimits[intent.TokenID]; tl != nil && tl.DailyLimitUnits > 0 && e.store != nil {
		rawLimit := tl.RawDailyLimit()
		spendKey := fmt.Sprintf("%s/%s", intent.WalletName, intent.TokenID)
		ok, _, err := e.store.CheckAndReserve(spendKey, checkTime, int64(intent.TokenAmount), int64(rawLimit))
		if err != nil {
			return nil, fmt.Errorf("checking daily token spend for %s: %w", intent.TokenID, err)
		}
		if !ok {
			return &CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("transaction would exceed daily token limit of %.0f %s for wallet %q", tl.DailyLimitUnits, intent.TokenID, intent.WalletName),
			}, nil
		}
	}

	// 6. Legacy TRX daily limit
	if intent.TokenID == "TRX" && wp.DailyLimitTRX > 0 && e.store != nil {
		if _, exists := wp.TokenLimits["TRX"]; !exists {
			limitSUN := int64(wp.DailyLimitTRX * 1_000_000)
			ok, dailySpent, err := e.store.CheckAndReserve(intent.WalletName+"/TRX", checkTime, intent.AmountSUN, limitSUN)
			if err != nil {
				return nil, fmt.Errorf("checking daily TRX spend: %w", err)
			}
			if !ok {
				return &CheckResult{
					Allowed: false,
					Reason:  fmt.Sprintf("transaction would exceed daily TRX limit: already spent %.6f TRX + this TX %.6f TRX > limit %.0f TRX for wallet %q", float64(dailySpent)/1_000_000, intent.AmountTRX(), wp.DailyLimitTRX, intent.WalletName),
				}, nil
			}
		}
	}

	// TODO: Aggregate USD limits (requires price service #85)

	return &CheckResult{Allowed: true}, nil
}

// ReleaseReserve rolls back a daily spend reservation on broadcast failure.
// Call this when Check returned Allowed but the transaction failed to broadcast.
func (e *Engine) ReleaseReserve(intent *Intent) {
	if e == nil || e.store == nil || intent == nil {
		return
	}

	wp := e.cfg.GetPolicy(intent.WalletName)
	if wp == nil {
		return
	}

	// Use the same UTC day as the original reservation to avoid midnight boundary issues
	releaseTime := intent.CheckTime
	if releaseTime.IsZero() {
		releaseTime = time.Now().UTC()
	}

	// Release per-token daily reservation
	if tl := wp.TokenLimits[intent.TokenID]; tl != nil && tl.DailyLimitUnits > 0 {
		spendKey := fmt.Sprintf("%s/%s", intent.WalletName, intent.TokenID)
		_ = e.store.AddDailySpend(spendKey, releaseTime, -int64(intent.TokenAmount))
	}

	// Release legacy TRX daily reservation
	if intent.TokenID == "TRX" && wp.DailyLimitTRX > 0 {
		if _, exists := wp.TokenLimits["TRX"]; !exists {
			_ = e.store.AddDailySpend(intent.WalletName+"/TRX", releaseTime, -intent.AmountSUN)
		}
	}
}

// RecordAudit records a transaction in the audit log.
// Daily spend is already tracked atomically in Check via CheckAndReserve.
func (e *Engine) RecordAudit(intent *Intent, txid string) error {
	if e == nil || e.store == nil {
		return nil
	}

	return e.store.RecordAudit(AuditEntry{
		Timestamp:  time.Now().UTC(),
		Action:     intent.Action,
		From:       intent.FromAddr,
		To:         intent.ToAddr,
		AmountSUN:  intent.AmountSUN,
		WalletName: intent.WalletName,
		TxID:       txid,
	})
}
