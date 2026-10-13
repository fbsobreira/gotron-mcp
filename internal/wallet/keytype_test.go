package wallet

import (
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKeyType(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    KeyType
		wantErr bool
	}{
		{"empty defaults to ecdsa", "", KeyTypeECDSA, false},
		{"ecdsa", "ecdsa", KeyTypeECDSA, false},
		{"ecdsa upper", "ECDSA", KeyTypeECDSA, false},
		{"ml-dsa canonical", "ml_dsa_44", KeyTypeMLDSA44, false},
		{"ml-dsa hyphen upper", "ML-DSA-44", KeyTypeMLDSA44, false},
		{"ml-dsa proto name", "ML_DSA_44", KeyTypeMLDSA44, false},
		{"fn-dsa canonical", "fn_dsa_512", KeyTypeFNDSA512, false},
		{"fn-dsa hyphen", "fn-dsa-512", KeyTypeFNDSA512, false},
		{"surrounding spaces", "  ml_dsa_44 ", KeyTypeMLDSA44, false},
		{"unknown", "rsa", "", true},
		{"partial", "ml_dsa", "", true},
		{"unknown scheme enum", "UNKNOWN_PQ_SCHEME", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseKeyType(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestKeyTypePQScheme(t *testing.T) {
	tests := []struct {
		kt     KeyType
		scheme core.PQScheme
		isPQ   bool
	}{
		{KeyTypeECDSA, core.PQScheme_UNKNOWN_PQ_SCHEME, false},
		{KeyTypeMLDSA44, core.PQScheme_ML_DSA_44, true},
		{KeyTypeFNDSA512, core.PQScheme_FN_DSA_512, true},
		{KeyType("bogus"), core.PQScheme_UNKNOWN_PQ_SCHEME, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.kt), func(t *testing.T) {
			scheme, ok := tt.kt.PQScheme()
			assert.Equal(t, tt.isPQ, ok)
			assert.Equal(t, tt.scheme, scheme)
			assert.Equal(t, tt.isPQ, tt.kt.IsPQ())
		})
	}
}

func TestKeyTypeFromPQScheme(t *testing.T) {
	tests := []struct {
		scheme  core.PQScheme
		want    KeyType
		wantErr bool
	}{
		{core.PQScheme_ML_DSA_44, KeyTypeMLDSA44, false},
		{core.PQScheme_FN_DSA_512, KeyTypeFNDSA512, false},
		{core.PQScheme_UNKNOWN_PQ_SCHEME, "", true},
		{core.PQScheme(99), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.scheme.String(), func(t *testing.T) {
			got, err := KeyTypeFromPQScheme(tt.scheme)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
