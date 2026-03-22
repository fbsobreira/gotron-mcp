package tools

import (
	"context"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

func TestParseResourceCode(t *testing.T) {
	tests := []struct {
		input   string
		want    core.ResourceCode
		wantErr bool
	}{
		{"ENERGY", core.ResourceCode_ENERGY, false},
		{"BANDWIDTH", core.ResourceCode_BANDWIDTH, false},
		{"invalid", 0, true},
		{"", 0, true},
		{"energy", 0, true}, // case-sensitive
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseResourceCode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseResourceCode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseResourceCode(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFreezeBalance_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleFreezeBalance(pool), map[string]any{
		"from":     "invalid",
		"amount":   "100",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for invalid address")
	}
}

func TestFreezeBalance_InvalidAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleFreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "abc",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for invalid amount")
	}
}

func TestFreezeBalance_ZeroAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleFreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "0",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for zero amount")
	}
}

func TestFreezeBalance_InvalidResource(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleFreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "100",
		"resource": "INVALID",
	})
	if !result.IsError {
		t.Error("expected error for invalid resource type")
	}
}

func TestFreezeBalance_Success(t *testing.T) {
	mock := &mockWalletServer{
		FreezeBalanceV2Func: func(_ context.Context, _ *core.FreezeBalanceV2Contract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Txid: []byte{0x01, 0x02, 0x03},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleFreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "100",
		"resource": "ENERGY",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["type"] != "FreezeBalanceV2Contract" {
		t.Errorf("type = %v, want FreezeBalanceV2Contract", data["type"])
	}
	if data["resource"] != "ENERGY" {
		t.Errorf("resource = %v, want ENERGY", data["resource"])
	}
	if data["transaction_hex"] == nil || data["transaction_hex"] == "" {
		t.Error("transaction_hex should not be empty")
	}
	if data["txid"] == nil || data["txid"] == "" {
		t.Error("txid should not be empty")
	}
}

func TestUnfreezeBalance_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleUnfreezeBalance(pool), map[string]any{
		"from":     "",
		"amount":   "100",
		"resource": "BANDWIDTH",
	})
	if !result.IsError {
		t.Error("expected error for empty address")
	}
}

func TestUnfreezeBalance_ZeroAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleUnfreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "0",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for zero amount")
	}
}

func TestUnfreezeBalance_InvalidAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleUnfreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "not-a-number",
		"resource": "BANDWIDTH",
	})
	if !result.IsError {
		t.Error("expected error for invalid amount")
	}
}

func TestUnfreezeBalance_InvalidResource(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleUnfreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "100",
		"resource": "INVALID",
	})
	if !result.IsError {
		t.Error("expected error for invalid resource type")
	}
}

func TestUnfreezeBalance_Success(t *testing.T) {
	mock := &mockWalletServer{
		UnfreezeBalanceV2Func: func(_ context.Context, _ *core.UnfreezeBalanceV2Contract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Txid: []byte{0x04, 0x05, 0x06},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleUnfreezeBalance(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "50.5",
		"resource": "BANDWIDTH",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["type"] != "UnfreezeBalanceV2Contract" {
		t.Errorf("type = %v, want UnfreezeBalanceV2Contract", data["type"])
	}
	if data["resource"] != "BANDWIDTH" {
		t.Errorf("resource = %v, want BANDWIDTH", data["resource"])
	}
	if data["transaction_hex"] == nil || data["transaction_hex"] == "" {
		t.Error("transaction_hex should not be empty")
	}
	if data["txid"] == nil || data["txid"] == "" {
		t.Error("txid should not be empty")
	}
}

func TestDelegateResource_InvalidFrom(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleDelegateResource(pool), map[string]any{
		"from":     "invalid",
		"to":       "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "100",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestDelegateResource_InvalidTo(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleDelegateResource(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":       "invalid",
		"amount":   "100",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for invalid to address")
	}
}

func TestDelegateResource_ZeroAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleDelegateResource(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":       "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":   "0",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for zero amount")
	}
}

func TestDelegateResource_Success(t *testing.T) {
	mock := &mockWalletServer{
		DelegateResourceFunc: func(_ context.Context, _ *core.DelegateResourceContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Txid: []byte{0x0a, 0x0b, 0x0c},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleDelegateResource(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":       "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":   "100",
		"resource": "ENERGY",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["type"] != "DelegateResourceContract" {
		t.Errorf("type = %v, want DelegateResourceContract", data["type"])
	}
	if data["transaction_hex"] == nil || data["transaction_hex"] == "" {
		t.Error("transaction_hex should not be empty")
	}
}

func TestDelegateResource_WithLock(t *testing.T) {
	mock := &mockWalletServer{
		DelegateResourceFunc: func(_ context.Context, in *core.DelegateResourceContract) (*api.TransactionExtention, error) {
			if !in.Lock {
				t.Error("expected lock=true")
			}
			if in.LockPeriod != 86400 {
				t.Errorf("lock_period = %d, want 86400", in.LockPeriod)
			}
			return &api.TransactionExtention{
				Txid: []byte{0x0d, 0x0e, 0x0f},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleDelegateResource(pool), map[string]any{
		"from":        "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":      "100",
		"resource":    "BANDWIDTH",
		"lock_period": float64(86400),
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["lock_period"] != float64(86400) {
		t.Errorf("lock_period = %v, want 86400", data["lock_period"])
	}
}

func TestDelegateResource_NegativeLockPeriod(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleDelegateResource(pool), map[string]any{
		"from":        "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":          "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":      "100",
		"resource":    "ENERGY",
		"lock_period": float64(-1),
	})
	if !result.IsError {
		t.Error("expected error for negative lock_period")
	}
}

func TestUndelegateResource_InvalidFrom(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleUndelegateResource(pool), map[string]any{
		"from":     "invalid",
		"to":       "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount":   "100",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestUndelegateResource_InvalidTo(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleUndelegateResource(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":       "invalid",
		"amount":   "100",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for invalid to address")
	}
}

func TestUndelegateResource_ZeroAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleUndelegateResource(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":       "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":   "0",
		"resource": "ENERGY",
	})
	if !result.IsError {
		t.Error("expected error for zero amount")
	}
}

func TestUndelegateResource_Success(t *testing.T) {
	mock := &mockWalletServer{
		UnDelegateResourceFunc: func(_ context.Context, _ *core.UnDelegateResourceContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Txid: []byte{0x10, 0x11, 0x12},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleUndelegateResource(pool), map[string]any{
		"from":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":       "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":   "50",
		"resource": "BANDWIDTH",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["type"] != "UnDelegateResourceContract" {
		t.Errorf("type = %v, want UnDelegateResourceContract", data["type"])
	}
}

func TestWithdrawExpireUnfreeze_InvalidFrom(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleWithdrawExpireUnfreeze(pool), map[string]any{
		"from": "invalid",
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestWithdrawExpireUnfreeze_Success(t *testing.T) {
	mock := &mockWalletServer{
		WithdrawExpireUnfreezeFunc: func(_ context.Context, _ *core.WithdrawExpireUnfreezeContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Txid: []byte{0x13, 0x14, 0x15},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleWithdrawExpireUnfreeze(pool), map[string]any{
		"from": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["type"] != "WithdrawExpireUnfreezeContract" {
		t.Errorf("type = %v, want WithdrawExpireUnfreezeContract", data["type"])
	}
	if data["transaction_hex"] == nil || data["transaction_hex"] == "" {
		t.Error("transaction_hex should not be empty")
	}
}
