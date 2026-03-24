package policy

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/approval"
)

// CheckResult holds the outcome of a policy evaluation.
type CheckResult struct {
	Allowed          bool
	Reason           string
	ApprovalRequired bool
}

// Engine evaluates transaction intents against wallet policies.
type Engine struct {
	cfg      *Config
	store    *Store
	approver approval.Approver // optional — handles approval_required responses
}

// NewEngine creates a policy engine with the given config and persistent store.
// A nil config is treated as an empty config (no restrictions).
func NewEngine(cfg *Config, store *Store) *Engine {
	if cfg == nil {
		cfg = &Config{}
	}
	return &Engine{cfg: cfg, store: store}
}

// SetApprover configures the approval backend for approval_required responses.
func (e *Engine) SetApprover(a approval.Approver) {
	if e != nil {
		e.approver = a
	}
}

// HasApprover returns true if an approval backend is configured.
func (e *Engine) HasApprover() bool {
	return e != nil && e.approver != nil
}

// RequestApproval calls the configured approver to get human approval.
// Returns (approved, error). If no approver is configured, returns (false, nil).
// Callers should check HasApprover() first and handle the no-approver case.
func (e *Engine) RequestApproval(ctx context.Context, intent *Intent) (bool, error) {
	if e == nil || e.approver == nil {
		return false, nil
	}

	// Use configured timeout (TX will be rebuilt fresh after approval)
	timeout := 5 * time.Minute
	if e.cfg.Approval != nil && e.cfg.Approval.Telegram != nil && e.cfg.Approval.Telegram.TimeoutSeconds > 0 {
		timeout = time.Duration(e.cfg.Approval.Telegram.TimeoutSeconds) * time.Second
	}
	expiresAt := time.Now().UTC().Add(timeout)

	// Build human-readable summary
	summary := buildApprovalSummary(intent)

	// Build spend context for the approval message
	spendContext := e.buildSpendContext(intent)

	reason := intent.Reason
	if reason == "" {
		reason = "No reason provided by agent"
	}

	req := approval.Request{
		WalletName:   intent.WalletName,
		ContractType: intent.Action,
		ContractData: intent.ContractData,
		HumanSummary: summary,
		Reason:       reason,
		ExpiresAt:    expiresAt,
		IsOverride:   intent.IsOverride,
		SpendContext: spendContext,
	}

	result, err := e.approver.RequestApproval(ctx, req)
	if err != nil {
		return false, fmt.Errorf("approval request failed: %w", err)
	}

	return result.Approved, nil
}

// buildSpendContext creates a summary of current spend for the approval message.
func (e *Engine) buildSpendContext(intent *Intent) string {
	if e.store == nil {
		return ""
	}
	wp := e.cfg.GetPolicy(intent.WalletName)
	if wp == nil {
		return ""
	}

	now := time.Now().UTC()
	var parts []string

	// Token-specific daily spend
	if tl := wp.TokenLimits[intent.TokenID]; tl != nil && tl.DailyLimitUnits > 0 {
		spendKey := fmt.Sprintf("%s/%s", intent.WalletName, intent.TokenID)
		spentRaw, err := e.store.GetDailySpend(spendKey, now)
		if err != nil {
			log.Printf("warning: failed to read daily spend for %s: %v", spendKey, err)
		} else {
			mult := decimalMultiplier(tl.Decimals)
			spentHuman := float64(spentRaw) / mult
			parts = append(parts, fmt.Sprintf("Daily %s: %.0f / %.0f", intent.TokenID, spentHuman, tl.DailyLimitUnits))
		}
	}

	// Legacy TRX daily spend
	if intent.TokenID == "TRX" && wp.DailyLimitTRX > 0 {
		if _, exists := wp.TokenLimits["TRX"]; !exists {
			spent, err := e.store.GetDailySpend(intent.WalletName+"/TRX", now)
			if err != nil {
				log.Printf("warning: failed to read daily TRX spend: %v", err)
			} else {
				spentTRX := float64(spent) / 1_000_000
				parts = append(parts, fmt.Sprintf("Daily TRX: %.2f / %.0f", spentTRX, wp.DailyLimitTRX))
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " | ")
}

// buildApprovalSummary creates a one-line summary of the transaction for approval.
func buildApprovalSummary(intent *Intent) string {
	switch intent.Action {
	case "TransferContract":
		return fmt.Sprintf("Transfer %.6f TRX to %s", intent.AmountTRX(), intent.ToAddr)
	case "TriggerSmartContract":
		if intent.TokenID != "TRX" && intent.TokenID != "" {
			return fmt.Sprintf("Token transfer of %.0f units to %s (contract: %s)", intent.TokenAmount, intent.ToAddr, intent.TokenID)
		}
		return fmt.Sprintf("Contract call to %s", intent.ToAddr)
	default:
		return fmt.Sprintf("%s from wallet %q", intent.Action, intent.WalletName)
	}
}

// Close closes the underlying store and approver. Safe to call on nil engine.
func (e *Engine) Close() error {
	if e == nil {
		return nil
	}
	// Close the approver if it implements io.Closer
	if closer, ok := e.approver.(interface{ Close() }); ok {
		closer.Close()
	}
	if e.store == nil {
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
	var approvalNeeded bool
	var approvalReason string
	if tl := wp.TokenLimits[intent.TokenID]; tl != nil {
		rawLimit := tl.RawPerTxLimit()
		if rawLimit > 0 && intent.TokenAmount > rawLimit {
			return &CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("transaction amount exceeds per-tx token limit of %.0f %s for wallet %q", tl.PerTxLimitUnits, intent.TokenID, intent.WalletName),
			}, nil
		}
		// Per-token approval threshold — flag but don't return yet (need stateful checks first)
		rawApproval := tl.RawApprovalThreshold()
		if rawApproval > 0 && intent.TokenAmount > rawApproval {
			approvalNeeded = true
			approvalReason = fmt.Sprintf("transaction amount exceeds approval threshold of %.0f %s for wallet %q — approval required", tl.ApprovalRequiredAboveUnits, intent.TokenID, intent.WalletName)
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

	// 4. Legacy TRX approval threshold — only if token_limits["TRX"] doesn't exist
	if !approvalNeeded && intent.TokenID == "TRX" && wp.ApprovalRequiredAboveTRX > 0 && intent.AmountTRX() > wp.ApprovalRequiredAboveTRX {
		if _, exists := wp.TokenLimits["TRX"]; !exists {
			approvalNeeded = true
			approvalReason = fmt.Sprintf("transaction amount %.6f TRX exceeds approval threshold of %.0f TRX for wallet %q — approval required", intent.AmountTRX(), wp.ApprovalRequiredAboveTRX, intent.WalletName)
		}
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
		if rawLimit > float64(math.MaxInt64) {
			return &CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("daily token limit for %s exceeds trackable range for wallet %q", intent.TokenID, intent.WalletName),
			}, nil
		}
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
			if wp.DailyLimitTRX > float64(math.MaxInt64)/1_000_000 {
				return &CheckResult{
					Allowed: false,
					Reason:  fmt.Sprintf("daily TRX limit %.0f exceeds trackable range for wallet %q", wp.DailyLimitTRX, intent.WalletName),
				}, nil
			}
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

	// 7. Approval threshold — return after stateful checks so daily spend is already reserved
	if approvalNeeded {
		return &CheckResult{
			Allowed:          false,
			ApprovalRequired: true,
			Reason:           approvalReason,
		}, nil
	}

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

// NotifyBroadcast sends a post-broadcast notification if the approver supports it.
func (e *Engine) NotifyBroadcast(ctx context.Context, txid string, success bool) {
	if e == nil || e.approver == nil {
		return
	}
	if notifier, ok := e.approver.(approval.Notifier); ok {
		if err := notifier.NotifyBroadcast(ctx, txid, success); err != nil {
			log.Printf("warning: failed to send broadcast notification: %v", err)
		}
	}
}

// RecordOverrideSpend tracks the spend from an override transaction without enforcing limits.
// This ensures daily budget reporting stays accurate after overrides.
func (e *Engine) RecordOverrideSpend(intent *Intent) {
	if e == nil || e.store == nil || intent == nil {
		return
	}
	// Guard against int64 overflow (same as Check)
	if intent.TokenAmount > float64(math.MaxInt64) {
		log.Printf("warning: override token amount too large to track for wallet %q", intent.WalletName)
		return
	}
	now := time.Now().UTC()

	// Record per-token spend
	wp := e.cfg.GetPolicy(intent.WalletName)
	if wp == nil {
		return
	}
	if tl := wp.TokenLimits[intent.TokenID]; tl != nil && tl.DailyLimitUnits > 0 {
		spendKey := fmt.Sprintf("%s/%s", intent.WalletName, intent.TokenID)
		if err := e.store.AddDailySpend(spendKey, now, int64(intent.TokenAmount)); err != nil {
			log.Printf("warning: failed to record override spend for %s: %v", spendKey, err)
		}
	}

	// Record legacy TRX spend
	if intent.TokenID == "TRX" && wp.DailyLimitTRX > 0 {
		if _, exists := wp.TokenLimits["TRX"]; !exists {
			if err := e.store.AddDailySpend(intent.WalletName+"/TRX", now, intent.AmountSUN); err != nil {
				log.Printf("warning: failed to record override TRX spend: %v", err)
			}
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
		Reason:     intent.Reason,
		Override:   intent.IsOverride,
	})
}
