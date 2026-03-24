package approval

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockApprover implements Approver for testing.
type MockApprover struct {
	Approved bool
	Err      error
}

func (m *MockApprover) RequestApproval(_ context.Context, _ Request) (Result, error) {
	if m.Err != nil {
		return Result{}, m.Err
	}
	return Result{
		Approved:   m.Approved,
		ApprovedBy: "test-user",
		Timestamp:  time.Now().UTC(),
	}, nil
}

func TestRequest_Fields(t *testing.T) {
	req := Request{
		ID:           "test-id",
		WalletName:   "savings",
		ContractType: "TransferContract",
		ContractData: map[string]any{"amount": "100"},
		HumanSummary: "Transfer 100 TRX",
		ExpiresAt:    time.Now().Add(5 * time.Minute),
	}
	assert.Equal(t, "test-id", req.ID)
	assert.Equal(t, "savings", req.WalletName)
	assert.Equal(t, "TransferContract", req.ContractType)
}

func TestResult_Fields(t *testing.T) {
	result := Result{
		Approved:   true,
		ApprovedBy: "@user",
		Timestamp:  time.Now().UTC(),
		Reason:     "",
	}
	assert.True(t, result.Approved)
	assert.Equal(t, "@user", result.ApprovedBy)
}

func TestMockApprover_Approved(t *testing.T) {
	m := &MockApprover{Approved: true}
	result, err := m.RequestApproval(context.Background(), Request{})
	require.NoError(t, err)
	assert.True(t, result.Approved)
}

func TestMockApprover_Rejected(t *testing.T) {
	m := &MockApprover{Approved: false}
	result, err := m.RequestApproval(context.Background(), Request{})
	require.NoError(t, err)
	assert.False(t, result.Approved)
}

func TestMockApprover_Error(t *testing.T) {
	m := &MockApprover{Err: assert.AnError}
	_, err := m.RequestApproval(context.Background(), Request{})
	assert.Error(t, err)
}

func TestTelegramConfig_Validation(t *testing.T) {
	// Missing bot token
	_, err := NewTelegramApprover(TelegramConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bot token")

	// Missing chat ID
	_, err = NewTelegramApprover(TelegramConfig{BotToken: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chat ID")

	// Missing authorized users
	_, err = NewTelegramApprover(TelegramConfig{BotToken: "test", ChatID: 123})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authorized_users")
}

func TestFormatApprovalMessage(t *testing.T) {
	req := Request{
		ContractType: "TransferContract",
		WalletName:   "savings",
		ContractData: map[string]any{
			"owner_address": "TFrom...",
			"to_address":    "TTo...",
			"amount":        "100 TRX",
		},
		HumanSummary: "Payment for invoice #1234",
		ExpiresAt:    time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC),
	}
	msg := formatApprovalMessage(req)
	assert.Contains(t, msg, "TransferContract")
	assert.Contains(t, msg, "savings")
	assert.Contains(t, msg, "TFrom...")
	assert.Contains(t, msg, "TTo...")
	assert.Contains(t, msg, "100 TRX")
	assert.Contains(t, msg, "Payment for invoice #1234")
	assert.Contains(t, msg, "12:00:00 UTC")
	assert.Contains(t, msg, "approval._")
}

func TestIsAuthorized(t *testing.T) {
	ta := &TelegramApprover{
		cfg: TelegramConfig{
			AuthorizedUsers: []int64{111, 222},
		},
	}
	assert.True(t, ta.isAuthorized(111))
	assert.True(t, ta.isAuthorized(222))
	assert.False(t, ta.isAuthorized(333))

	// Empty list = no one authorized
	ta2 := &TelegramApprover{cfg: TelegramConfig{}}
	assert.False(t, ta2.isAuthorized(999))
}
