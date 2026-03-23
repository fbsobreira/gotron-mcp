package policy

import (
	"encoding/hex"
	"math"
	"math/big"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/client/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTRC20Calldata constructs ABI-encoded calldata for transfer/approve:
// selector (4 bytes) + address (32 bytes padded) + amount (32 bytes).
func buildTRC20Calldata(selector string, addrHex20 string, amount *big.Int) string {
	sel, _ := hex.DecodeString(selector)
	data := make([]byte, 68)
	copy(data[:4], sel)
	// Address goes in bytes 16..36 (right-aligned in 32-byte word, skipping 0x41 prefix)
	addrBytes, _ := hex.DecodeString(addrHex20)
	copy(data[4+12:4+32], addrBytes)
	// Amount goes in bytes 36..68 (right-aligned in 32-byte word)
	amtBytes := amount.Bytes()
	copy(data[68-len(amtBytes):68], amtBytes)
	return hex.EncodeToString(data)
}

func TestIntentFromContractData_NilData(t *testing.T) {
	intent, err := IntentFromContractData("wallet1", nil)
	assert.Nil(t, intent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil contract data")
}

func TestIntentFromContractData_TransferContract(t *testing.T) {
	data := &transaction.ContractData{
		Type: "TransferContract",
		Fields: map[string]any{
			"owner_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"to_address":    "TVjsyZ7fYF3qLF6BQgPmTEZy1xrNNyVAAA",
			"amount":        float64(5_000_000), // 5 TRX in SUN
		},
	}

	intent, err := IntentFromContractData("mywallet", data)
	require.NoError(t, err)
	require.NotNil(t, intent)

	assert.Equal(t, "mywallet", intent.WalletName)
	assert.Equal(t, "TransferContract", intent.Action)
	assert.Equal(t, "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8", intent.FromAddr)
	assert.Equal(t, "TVjsyZ7fYF3qLF6BQgPmTEZy1xrNNyVAAA", intent.ToAddr)
	assert.Equal(t, int64(5_000_000), intent.AmountSUN)
	assert.Equal(t, "TRX", intent.TokenID)
	assert.InDelta(t, 5.0, intent.TokenAmount, 0.0001)
}

func TestIntentFromContractData_TransferContract_Int64Amount(t *testing.T) {
	data := &transaction.ContractData{
		Type: "TransferContract",
		Fields: map[string]any{
			"owner_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"to_address":    "TVjsyZ7fYF3qLF6BQgPmTEZy1xrNNyVAAA",
			"amount":        int64(10_000_000),
		},
	}

	intent, err := IntentFromContractData("w", data)
	require.NoError(t, err)
	assert.Equal(t, int64(10_000_000), intent.AmountSUN)
	assert.InDelta(t, 10.0, intent.TokenAmount, 0.0001)
}

func TestIntentFromContractData_TransferContract_MissingAmount(t *testing.T) {
	data := &transaction.ContractData{
		Type: "TransferContract",
		Fields: map[string]any{
			"owner_address": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"to_address":    "TVjsyZ7fYF3qLF6BQgPmTEZy1xrNNyVAAA",
		},
	}

	// No amount field — extractAmount returns nil for missing key, so AmountSUN stays 0
	intent, err := IntentFromContractData("w", data)
	require.NoError(t, err)
	assert.Equal(t, int64(0), intent.AmountSUN)
}

func TestIntentFromContractData_FrozenBalance_Fallback(t *testing.T) {
	// "amount" has unsupported type -> triggers fallback to "frozen_balance"
	data := &transaction.ContractData{
		Type: "FreezeBalanceV2Contract",
		Fields: map[string]any{
			"owner_address":  "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"amount":         []byte{1}, // bad type forces error
			"frozen_balance": float64(100_000_000),
		},
	}

	intent, err := IntentFromContractData("w", data)
	require.NoError(t, err)
	assert.Equal(t, int64(100_000_000), intent.AmountSUN)
	assert.InDelta(t, 100.0, intent.TokenAmount, 0.0001)
}

func TestIntentFromContractData_BalanceField_Fallback(t *testing.T) {
	// Both "amount" and "frozen_balance" have bad types -> falls through to "balance"
	data := &transaction.ContractData{
		Type: "DelegateResourceContract",
		Fields: map[string]any{
			"owner_address":    "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"receiver_address": "TVjsyZ7fYF3qLF6BQgPmTEZy1xrNNyVAAA",
			"amount":           []byte{1}, // bad type
			"frozen_balance":   []byte{1}, // bad type
			"balance":          float64(50_000_000),
		},
	}

	intent, err := IntentFromContractData("w", data)
	require.NoError(t, err)
	assert.Equal(t, int64(50_000_000), intent.AmountSUN)
	assert.Equal(t, "TVjsyZ7fYF3qLF6BQgPmTEZy1xrNNyVAAA", intent.ToAddr)
}

func TestIntentFromContractData_AllAmountFieldsFail(t *testing.T) {
	data := &transaction.ContractData{
		Type: "TransferContract",
		Fields: map[string]any{
			"owner_address":  "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"amount":         []byte{1}, // bad type
			"frozen_balance": []byte{1}, // bad type
			"balance":        []byte{1}, // bad type
		},
	}

	intent, err := IntentFromContractData("w", data)
	assert.Nil(t, intent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed amount")
}

func TestIntentFromContractData_TRC20Transfer(t *testing.T) {
	// 20-byte address (without 0x41 prefix)
	recipientHex20 := "a614f803b6fd780986a42c78ec9c7f77e6ded13c"
	amount := big.NewInt(1_000_000) // 1M raw token units
	calldata := buildTRC20Calldata("a9059cbb", recipientHex20, amount)

	data := &transaction.ContractData{
		Type: "TriggerSmartContract",
		Fields: map[string]any{
			"owner_address":    "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
			"data":             calldata,
		},
	}

	intent, err := IntentFromContractData("w", data)
	require.NoError(t, err)
	require.NotNil(t, intent)

	assert.Equal(t, "TriggerSmartContract", intent.Action)
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", intent.TokenID)
	assert.NotEmpty(t, intent.ToAddr)
	assert.Equal(t, int64(0), intent.AmountSUN) // TRC20, not TRX
	require.NotNil(t, intent.RawTokenAmt)
	assert.Equal(t, int64(1_000_000), intent.RawTokenAmt.Int64())
	assert.InDelta(t, 1_000_000.0, intent.TokenAmount, 0.01)
}

func TestIntentFromContractData_TRC20Approve(t *testing.T) {
	recipientHex20 := "a614f803b6fd780986a42c78ec9c7f77e6ded13c"
	amount := big.NewInt(999)
	calldata := buildTRC20Calldata("095ea7b3", recipientHex20, amount)

	data := &transaction.ContractData{
		Type: "TriggerSmartContract",
		Fields: map[string]any{
			"owner_address":    "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
			"data":             calldata,
		},
	}

	intent, err := IntentFromContractData("w", data)
	require.NoError(t, err)
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", intent.TokenID)
	require.NotNil(t, intent.RawTokenAmt)
	assert.Equal(t, int64(999), intent.RawTokenAmt.Int64())
}

func TestIntentFromContractData_TriggerSmartContract_UnknownSelector(t *testing.T) {
	// Unknown function selector — falls through to call_value extraction
	data := &transaction.ContractData{
		Type: "TriggerSmartContract",
		Fields: map[string]any{
			"owner_address":    "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
			"data":             "deadbeef0000000000000000000000000000000000000000000000000000000000000000",
			"call_value":       float64(2_000_000),
		},
	}

	intent, err := IntentFromContractData("w", data)
	require.NoError(t, err)
	assert.Equal(t, "TRX", intent.TokenID)
	assert.Equal(t, int64(2_000_000), intent.AmountSUN)
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", intent.ToAddr)
	assert.InDelta(t, 2.0, intent.TokenAmount, 0.0001)
}

func TestIntentFromContractData_TriggerSmartContract_NoData(t *testing.T) {
	// No data field at all — extractTRC20Intent fails, falls to call_value
	data := &transaction.ContractData{
		Type: "TriggerSmartContract",
		Fields: map[string]any{
			"owner_address":    "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		},
	}

	intent, err := IntentFromContractData("w", data)
	require.NoError(t, err)
	assert.Equal(t, "TRX", intent.TokenID)
	assert.Equal(t, int64(0), intent.AmountSUN)
}

func TestIntentFromContractData_TriggerSmartContract_MalformedCallValue(t *testing.T) {
	data := &transaction.ContractData{
		Type: "TriggerSmartContract",
		Fields: map[string]any{
			"owner_address":    "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
			"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
			"call_value":       []byte{1, 2, 3}, // unsupported type
		},
	}

	intent, err := IntentFromContractData("w", data)
	assert.Nil(t, intent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed call_value")
}

func TestIntentFromContractData_MissingFields(t *testing.T) {
	data := &transaction.ContractData{
		Type:   "TransferContract",
		Fields: map[string]any{},
	}

	intent, err := IntentFromContractData("w", data)
	require.NoError(t, err)
	assert.Equal(t, "", intent.FromAddr)
	assert.Equal(t, "", intent.ToAddr)
	assert.Equal(t, int64(0), intent.AmountSUN)
}

func TestIntentFromContractData_HexAddress(t *testing.T) {
	// Hex addresses (41-prefixed) should be normalized to base58
	data := &transaction.ContractData{
		Type: "TransferContract",
		Fields: map[string]any{
			"owner_address": "41a614f803b6fd780986a42c78ec9c7f77e6ded13c",
			"to_address":    "41b2a1f0f0c0f0e0d0f0a0b0c0d0e0f0102030405",
			"amount":        float64(1_000_000),
		},
	}

	intent, err := IntentFromContractData("w", data)
	require.NoError(t, err)
	// Normalized addresses should start with T (base58)
	assert.True(t, len(intent.FromAddr) > 0)
	assert.NotEqual(t, "41a614f803b6fd780986a42c78ec9c7f77e6ded13c", intent.FromAddr)
}

func TestDecodeAddressAndAmount_ValidData(t *testing.T) {
	// Build 68-byte calldata: 4-byte selector + 32-byte addr + 32-byte amount
	raw := make([]byte, 68)
	// selector
	raw[0], raw[1], raw[2], raw[3] = 0xa9, 0x05, 0x9c, 0xbb
	// address at bytes 16..36 (20 bytes)
	addrBytes, _ := hex.DecodeString("a614f803b6fd780986a42c78ec9c7f77e6ded13c")
	copy(raw[16:36], addrBytes)
	// amount = 42 at bytes 36..68
	raw[67] = 42

	addr, amount, err := decodeAddressAndAmount(raw)
	require.NoError(t, err)
	assert.NotEmpty(t, addr)
	// Should be a valid base58 TRON address
	assert.True(t, len(addr) > 20)
	assert.Equal(t, int64(42), amount.Int64())
}

func TestDecodeAddressAndAmount_ShortData(t *testing.T) {
	raw := make([]byte, 40) // less than 68
	addr, amount, err := decodeAddressAndAmount(raw)
	assert.Empty(t, addr)
	assert.Nil(t, amount)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "calldata too short")
}

func TestBigIntToSafeFloat_Nil(t *testing.T) {
	assert.Equal(t, 0.0, bigIntToSafeFloat(nil))
}

func TestBigIntToSafeFloat_Zero(t *testing.T) {
	assert.Equal(t, 0.0, bigIntToSafeFloat(big.NewInt(0)))
}

func TestBigIntToSafeFloat_Normal(t *testing.T) {
	assert.InDelta(t, 12345.0, bigIntToSafeFloat(big.NewInt(12345)), 0.001)
}

func TestBigIntToSafeFloat_Negative(t *testing.T) {
	result := bigIntToSafeFloat(big.NewInt(-1))
	assert.Equal(t, math.MaxFloat64, result)
}

func TestBigIntToSafeFloat_Huge(t *testing.T) {
	// A number so large it would overflow float64
	huge := new(big.Int).Exp(big.NewInt(2), big.NewInt(2000), nil)
	result := bigIntToSafeFloat(huge)
	assert.Equal(t, math.MaxFloat64, result)
}

func TestExtractAmount_Float64(t *testing.T) {
	intent := &Intent{}
	fields := map[string]any{"amount": float64(3_000_000)}
	err := extractAmount(intent, fields, "amount")
	require.NoError(t, err)
	assert.Equal(t, int64(3_000_000), intent.AmountSUN)
}

func TestExtractAmount_Int64(t *testing.T) {
	intent := &Intent{}
	fields := map[string]any{"amount": int64(7_500_000)}
	err := extractAmount(intent, fields, "amount")
	require.NoError(t, err)
	assert.Equal(t, int64(7_500_000), intent.AmountSUN)
}

func TestExtractAmount_String(t *testing.T) {
	intent := &Intent{}
	fields := map[string]any{"amount": "2.5"}
	err := extractAmount(intent, fields, "amount")
	require.NoError(t, err)
	assert.Equal(t, int64(2_500_000), intent.AmountSUN)
}

func TestExtractAmount_StringInvalid(t *testing.T) {
	intent := &Intent{}
	fields := map[string]any{"amount": "notanumber"}
	err := extractAmount(intent, fields, "amount")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot parse")
}

func TestExtractAmount_UnknownType(t *testing.T) {
	intent := &Intent{}
	fields := map[string]any{"amount": []int{1, 2, 3}}
	err := extractAmount(intent, fields, "amount")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected type")
}

func TestExtractAmount_MissingKey(t *testing.T) {
	intent := &Intent{}
	fields := map[string]any{"other": float64(100)}
	err := extractAmount(intent, fields, "amount")
	require.NoError(t, err)
	assert.Equal(t, int64(0), intent.AmountSUN)
}

func TestAmountTRX(t *testing.T) {
	tests := []struct {
		name      string
		amountSUN int64
		expected  float64
	}{
		{"zero", 0, 0.0},
		{"one_trx", 1_000_000, 1.0},
		{"fractional", 1_500_000, 1.5},
		{"large", 100_000_000, 100.0},
		{"small_sun", 1, 0.000001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := &Intent{AmountSUN: tt.amountSUN}
			assert.InDelta(t, tt.expected, intent.AmountTRX(), 0.0000001)
		})
	}
}
