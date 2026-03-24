package approval

import (
	"context"
	"time"
)

// Approver requests human approval for a transaction before signing.
// Implementations: TelegramApprover, ElicitationApprover (future), CryptoApprover (future).
type Approver interface {
	// RequestApproval sends an approval request and blocks until approved, rejected, or timeout.
	RequestApproval(ctx context.Context, req Request) (Result, error)
}

// Request describes a transaction needing human approval.
type Request struct {
	ID           string         // unique approval ID (UUID)
	WalletName   string         // wallet triggering the approval
	ContractType string         // e.g., "TransferContract", "TriggerSmartContract"
	ContractData map[string]any // decoded TX fields (from, to, amount)
	HumanSummary string         // one-line description for the approval message
	Reason       string         // agent-provided reason for the transaction
	ExpiresAt    time.Time      // deadline — after this, the request is auto-rejected
	IsOverride   bool           // true when this is a one-shot limit override
	SpendContext string         // e.g., "Daily spent: 450 / 500 TRX"
}

// Result is the outcome of an approval request.
type Result struct {
	Approved   bool
	ApprovedBy string // identifier (Telegram user ID, wallet address, etc.)
	Timestamp  time.Time
	Reason     string // optional rejection reason
}

// Notifier can send post-broadcast notifications (e.g., txid to Telegram chat).
// Optional — not all approvers implement this.
type Notifier interface {
	NotifyBroadcast(ctx context.Context, txid string, success bool) error
}
