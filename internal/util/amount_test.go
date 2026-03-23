package util

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTRXToSun(t *testing.T) {
	tests := []struct {
		name    string
		trx     string
		want    int64
		wantErr bool
	}{
		{"whole number", "1", 1_000_000, false},
		{"decimal", "1.5", 1_500_000, false},
		{"zero", "0", 0, false},
		{"small", "0.000001", 1, false},
		{"large", "1000000", 1_000_000_000_000, false},
		{"six decimals", "123.456789", 123_456_789, false},
		{"negative", "-1", 0, true},
		{"invalid", "abc", 0, true},
		{"empty", "", 0, true},
		{"too many decimals", "1.1234567", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TRXToSun(tt.trx)
			if tt.wantErr {
				assert.Error(t, err, "TRXToSun(%q)", tt.trx)
				return
			}
			require.NoError(t, err, "TRXToSun(%q)", tt.trx)
			assert.Equal(t, tt.want, got, "TRXToSun(%q)", tt.trx)
		})
	}
}

func TestSunToTRX(t *testing.T) {
	tests := []struct {
		name string
		sun  int64
		want string
	}{
		{"one trx", 1_000_000, "1.000000"},
		{"zero", 0, "0.000000"},
		{"fractional", 1, "0.000001"},
		{"large", 1_234_567_890, "1234.567890"},
		{"negative whole", -1_500_000, "-1.500000"},
		{"negative fractional", -500_000, "-0.500000"},
		{"negative one sun", -1, "-0.000001"},
		{"min int64 overflow", math.MinInt64, "-9223372036854.775808"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SunToTRX(tt.sun)
			assert.Equal(t, tt.want, got, "SunToTRX(%d)", tt.sun)
		})
	}
}

func TestFormatTRC20Amount(t *testing.T) {
	tests := []struct {
		name     string
		raw      *big.Int
		decimals int
		want     string
	}{
		{"usdt 6 decimals", big.NewInt(1_000_000), 6, "1.000000"},
		{"18 decimals", new(big.Int).Mul(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)), 18, "1.000000000000000000"},
		{"zero", big.NewInt(0), 6, "0.000000"},
		{"nil", nil, 6, "0.000000"},
		{"negative raw", big.NewInt(-1_500_000), 6, "-1.500000"},
		{"zero decimals", big.NewInt(42), 0, "42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTRC20Amount(tt.raw, tt.decimals)
			assert.Equal(t, tt.want, got, "FormatTRC20Amount()")
		})
	}
}

func TestParseTRC20Amount(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		decimals int
		want     string
		wantErr  bool
	}{
		{"whole 6 decimals", "1", 6, "1000000", false},
		{"decimal 6", "1.5", 6, "1500000", false},
		{"whole 18 decimals", "1", 18, "1000000000000000000", false},
		{"zero", "0", 6, "0", false},
		{"negative", "-1", 6, "", true},
		{"too many decimals", "1.1234567", 6, "", true},
		{"empty", "", 6, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTRC20Amount(tt.amount, tt.decimals)
			if tt.wantErr {
				assert.Error(t, err, "ParseTRC20Amount(%q, %d)", tt.amount, tt.decimals)
				return
			}
			assert.NoError(t, err, "ParseTRC20Amount(%q, %d)", tt.amount, tt.decimals)
			assert.Equal(t, tt.want, got.String(), "ParseTRC20Amount(%q, %d)", tt.amount, tt.decimals)
		})
	}
}
