package util

import (
	"fmt"
	"math"
	"math/big"
	"strings"
)

const sunPerTRX = 1_000_000

// TRXToSun converts a TRX amount string (e.g., "1.5") to SUN (int64).
func TRXToSun(trx string) (int64, error) {
	if trx == "" {
		return 0, fmt.Errorf("empty amount")
	}

	parts := strings.Split(trx, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid amount: %s", trx)
	}

	whole, ok := new(big.Int).SetString(parts[0], 10)
	if !ok {
		return 0, fmt.Errorf("invalid amount: %s", trx)
	}

	if whole.Sign() < 0 {
		return 0, fmt.Errorf("negative amount not allowed: %s", trx)
	}

	sun := new(big.Int).Mul(whole, big.NewInt(sunPerTRX))

	if len(parts) == 2 {
		decimal := parts[1]
		if len(decimal) > 6 {
			return 0, fmt.Errorf("too many decimal places (max 6): %s", trx)
		}
		decimal = decimal + strings.Repeat("0", 6-len(decimal))
		frac, ok := new(big.Int).SetString(decimal, 10)
		if !ok {
			return 0, fmt.Errorf("invalid decimal: %s", trx)
		}
		sun.Add(sun, frac)
	}

	if !sun.IsInt64() {
		return 0, fmt.Errorf("amount overflow: %s", trx)
	}

	return sun.Int64(), nil
}

// SunToTRX converts SUN (int64) to a TRX string with 6 decimal places.
func SunToTRX(sun int64) string {
	if sun == math.MinInt64 {
		// -9223372036854.775808 — can't negate without overflow
		return "-9223372036854.775808"
	}
	sign := ""
	if sun < 0 {
		sign = "-"
		sun = -sun
	}
	return fmt.Sprintf("%s%d.%06d", sign, sun/sunPerTRX, sun%sunPerTRX)
}

// FormatTRC20Amount formats a raw TRC20 token amount with the given decimals.
// Only non-negative raw values are supported.
func FormatTRC20Amount(raw *big.Int, decimals int) string {
	if raw == nil {
		raw = big.NewInt(0)
	}
	if decimals <= 0 {
		return raw.String()
	}

	sign := ""
	val := new(big.Int).Set(raw)
	if val.Sign() < 0 {
		sign = "-"
		val.Abs(val)
	}

	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int).Div(val, divisor)
	frac := new(big.Int).Mod(val, divisor)

	fracStr := frac.String()
	if len(fracStr) < decimals {
		fracStr = strings.Repeat("0", decimals-len(fracStr)) + fracStr
	}

	return fmt.Sprintf("%s%s.%s", sign, whole.String(), fracStr)
}

// ParseTRC20Amount parses a human-readable TRC20 amount string into a raw big.Int value.
func ParseTRC20Amount(amount string, decimals int) (*big.Int, error) {
	if amount == "" {
		return nil, fmt.Errorf("empty amount")
	}

	parts := strings.Split(amount, ".")
	if len(parts) > 2 {
		return nil, fmt.Errorf("invalid amount: %s", amount)
	}

	whole, ok := new(big.Int).SetString(parts[0], 10)
	if !ok {
		return nil, fmt.Errorf("invalid amount: %s", amount)
	}

	if whole.Sign() < 0 {
		return nil, fmt.Errorf("negative amount not allowed: %s", amount)
	}

	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	result := new(big.Int).Mul(whole, multiplier)

	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) > decimals {
			return nil, fmt.Errorf("too many decimal places (max %d): %s", decimals, amount)
		}
		frac = frac + strings.Repeat("0", decimals-len(frac))
		fracInt, ok := new(big.Int).SetString(frac, 10)
		if !ok {
			return nil, fmt.Errorf("invalid decimal: %s", amount)
		}
		result.Add(result, fracInt)
	}

	return result, nil
}
