package policy

import (
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/client/transaction"
)

// Intent represents what a transaction intends to do, extracted for policy evaluation.
type Intent struct {
	WalletName  string
	Action      string // contract type: "TransferContract", "TriggerSmartContract", etc.
	FromAddr    string
	ToAddr      string
	AmountSUN   int64     // TRX amount in SUN (for TRX transfers)
	TokenID     string    // "TRX" for native transfers, contract address for TRC20
	TokenAmount float64   // raw on-chain amount (SUN for TRX, raw uint256 for TRC20 before decimals)
	RawTokenAmt *big.Int  // raw uint256 token amount (before decimals, for overflow-safe checks)
	CheckTime   time.Time // set by Engine.Check — used by ReleaseReserve for consistent day bucket
}

// IntentFromContractData builds an Intent from decoded transaction contract data.
// Returns an error if the intent cannot be fully extracted — callers should deny
// the transaction when policy is active and this returns an error.
func IntentFromContractData(walletName string, data *transaction.ContractData) (*Intent, error) {
	if data == nil {
		return nil, fmt.Errorf("nil contract data")
	}

	intent := &Intent{
		WalletName: walletName,
		Action:     data.Type,
	}

	// Extract from/to addresses
	if v, ok := data.Fields["owner_address"].(string); ok {
		intent.FromAddr = normalizeAddress(v)
	}
	if v, ok := data.Fields["to_address"].(string); ok {
		intent.ToAddr = normalizeAddress(v)
	}

	// For TriggerSmartContract, decode ABI data to extract real recipient
	if data.Type == "TriggerSmartContract" {
		contractAddr, _ := data.Fields["contract_address"].(string)
		contractAddr = normalizeAddress(contractAddr)
		if err := extractTRC20Intent(intent, data); err == nil {
			intent.TokenID = contractAddr
			return intent, nil
		}
		// Not a recognized TRC20 call — use contract_address as destination
		if contractAddr != "" && intent.ToAddr == "" {
			intent.ToAddr = contractAddr
		}
		intent.TokenID = "TRX"
		if err := extractAmount(intent, data.Fields, "call_value"); err != nil {
			return nil, fmt.Errorf("malformed call_value: %w", err)
		}
		intent.TokenAmount = intent.AmountTRX()
		return intent, nil
	}

	// Standard transfers (TRX)
	intent.TokenID = "TRX"
	if v, ok := data.Fields["contract_address"].(string); ok && intent.ToAddr == "" {
		intent.ToAddr = normalizeAddress(v)
	}
	if v, ok := data.Fields["receiver_address"].(string); ok && intent.ToAddr == "" {
		intent.ToAddr = normalizeAddress(v)
	}
	if err := extractAmount(intent, data.Fields, "amount"); err != nil {
		// Try alternative field names for freeze/delegate contracts
		if err2 := extractAmount(intent, data.Fields, "frozen_balance"); err2 != nil {
			if err3 := extractAmount(intent, data.Fields, "balance"); err3 != nil {
				return nil, fmt.Errorf("malformed amount: %w", err)
			}
		}
	}
	intent.TokenAmount = intent.AmountTRX()

	return intent, nil
}

// extractTRC20Intent parses ABI-encoded data for ERC20 transfer/approve methods.
func extractTRC20Intent(intent *Intent, data *transaction.ContractData) error {
	dataHex, ok := data.Fields["data"].(string)
	if !ok || len(dataHex) < 8 {
		return fmt.Errorf("no data field")
	}

	raw, err := hex.DecodeString(dataHex)
	if err != nil || len(raw) < 4 {
		return fmt.Errorf("invalid hex data")
	}

	selector := hex.EncodeToString(raw[:4])

	switch selector {
	case "a9059cbb", "095ea7b3": // transfer(address,uint256) or approve(address,uint256)
		addr, amount, err := decodeAddressAndAmount(raw)
		if err != nil {
			return err
		}
		intent.ToAddr = addr
		intent.RawTokenAmt = amount
		intent.TokenAmount = bigIntToSafeFloat(amount)
		return nil
	}

	return fmt.Errorf("unknown selector: %s", selector)
}

// decodeAddressAndAmount extracts (address, uint256) from ABI-encoded calldata.
// Used for both transfer(address,uint256) and approve(address,uint256).
func decodeAddressAndAmount(raw []byte) (string, *big.Int, error) {
	if len(raw) < 68 {
		return "", nil, fmt.Errorf("calldata too short: need 68 bytes, got %d", len(raw))
	}
	addrBytes := make([]byte, 21)
	addrBytes[0] = 0x41
	copy(addrBytes[1:], raw[16:36])
	addr := address.BytesToAddress(addrBytes).String()
	amount := new(big.Int).SetBytes(raw[36:68])
	return addr, amount, nil
}

// bigIntToSafeFloat converts a big.Int to float64 safely.
// Returns math.MaxFloat64 for values that exceed float64 range, preventing
// underflow/wrap that could bypass limits.
func bigIntToSafeFloat(n *big.Int) float64 {
	if n == nil {
		return 0
	}
	// If negative (shouldn't happen for uint256 but guard anyway), treat as max
	if n.Sign() < 0 {
		return math.MaxFloat64
	}
	f := new(big.Float).SetInt(n)
	result, _ := f.Float64()
	if math.IsInf(result, 0) {
		return math.MaxFloat64
	}
	return result
}

// extractAmount reads an amount field from the decoded fields map.
// Returns an error if the value is present but malformed.
func extractAmount(intent *Intent, fields map[string]any, key string) error {
	v, exists := fields[key]
	if !exists {
		return nil
	}
	switch a := v.(type) {
	case float64:
		if math.IsNaN(a) || math.IsInf(a, 0) || a > float64(math.MaxInt64) || a < float64(math.MinInt64) {
			return fmt.Errorf("amount %v for %s is out of int64 range", a, key)
		}
		intent.AmountSUN = int64(a)
	case int64:
		intent.AmountSUN = a
	case string:
		sun, err := util.TRXToSun(a)
		if err != nil {
			return fmt.Errorf("cannot parse %q as TRX amount: %w", a, err)
		}
		intent.AmountSUN = sun
	default:
		return fmt.Errorf("unexpected type %T for %s", v, key)
	}
	return nil
}

// AmountTRX returns the amount in TRX as a float64.
func (i *Intent) AmountTRX() float64 {
	return float64(i.AmountSUN) / 1_000_000
}
