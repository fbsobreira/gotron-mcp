package tools

import (
	"context"
	"math/big"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

func TestTransferTRX_InvalidFromAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "invalid",
		"to":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount": "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestTransferTRX_InvalidToAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "invalid",
		"amount": "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid to address")
	}
}

func TestTransferTRX_InvalidAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount": "abc",
	})
	if !result.IsError {
		t.Error("expected error for invalid amount")
	}
}

func TestTransferTRX_ZeroAmount(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"amount": "0",
	})
	if !result.IsError {
		t.Error("expected error for zero amount")
	}
}

func TestTransferTRX_Success(t *testing.T) {
	mock := &mockWalletServer{
		CreateTransaction2Func: func(_ context.Context, _ *core.TransferContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Txid: []byte{0x0a, 0x0b, 0x0c},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTransferTRX(pool), map[string]any{
		"from":   "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":     "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount": "100.5",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["type"] != "TransferContract" {
		t.Errorf("type = %v, want TransferContract", data["type"])
	}
	if data["from"] != "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF" {
		t.Errorf("from = %v", data["from"])
	}
	if data["transaction_hex"] == nil || data["transaction_hex"] == "" {
		t.Error("transaction_hex should not be empty")
	}
}

func TestTransferTRC20_InvalidAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRC20(pool), map[string]any{
		"from":             "invalid",
		"to":               "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid from address")
	}
}

func TestTransferTRC20_InvalidContractAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRC20(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"contract_address": "invalid",
		"amount":           "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid contract address")
	}
}

func TestTransferTRC20_InvalidToAddress(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	result := callTool(t, handleTransferTRC20(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "bad",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	if !result.IsError {
		t.Error("expected error for invalid to address")
	}
}

func TestTransferTRC20_InvalidFeeLimit(t *testing.T) {
	// Need a mock that returns decimals to get past that check
	mock := &mockWalletServer{
		TriggerConstantContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(6))},
				Result:         &api.Return{Result: true},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTransferTRC20(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
		"fee_limit":        float64(20000),
	})
	if !result.IsError {
		t.Error("expected error for fee_limit > 15000")
	}
}

func TestTransferTRC20_Success(t *testing.T) {
	mock := &mockWalletServer{
		// TRC20GetDecimalsCtx calls TriggerConstantContract
		TriggerConstantContractFunc: func(_ context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				ConstantResult: [][]byte{abiEncodeUint256(big.NewInt(6))},
				Result:         &api.Return{Result: true},
			}, nil
		},
		// TRC20SendCtx calls TriggerContract
		TriggerContractFunc: func(_ context.Context, _ *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{
				Result: &api.Return{Result: true, Code: 0},
				Txid:   []byte{0x0d, 0x0e, 0x0f},
				Transaction: &core.Transaction{
					RawData: &core.TransactionRaw{},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleTransferTRC20(pool), map[string]any{
		"from":             "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		"to":               "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"amount":           "100",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if data["type"] != "TriggerSmartContract" {
		t.Errorf("type = %v, want TriggerSmartContract", data["type"])
	}
	if data["transaction_hex"] == nil || data["transaction_hex"] == "" {
		t.Error("transaction_hex should not be empty")
	}
}
