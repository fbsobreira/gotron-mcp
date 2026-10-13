package wallet

import (
	"fmt"
	"strings"

	"github.com/fbsobreira/gotron-sdk/pkg/pqc"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

// KeyType names the signature algorithm of a wallet key, as MCP tools accept
// and report it.
type KeyType string

const (
	// KeyTypeECDSA is a secp256k1 key, the TRON default.
	KeyTypeECDSA KeyType = "ecdsa"
	// KeyTypeMLDSA44 is a TIP-899 ML-DSA-44 (Dilithium-2) post-quantum key.
	KeyTypeMLDSA44 KeyType = "ml_dsa_44"
	// KeyTypeFNDSA512 is a TIP-899 FN-DSA-512 (Falcon-512) post-quantum key.
	KeyTypeFNDSA512 KeyType = "fn_dsa_512"
)

// KeyTypes lists every supported key type, ECDSA first.
var KeyTypes = []KeyType{KeyTypeECDSA, KeyTypeMLDSA44, KeyTypeFNDSA512}

// ParseKeyType parses a user-supplied key type. Matching ignores case and
// treats "-" as "_", so "ML-DSA-44" and "ml_dsa_44" are equal. An empty
// string means ECDSA.
func ParseKeyType(s string) (KeyType, error) {
	if norm := strings.ToLower(strings.TrimSpace(s)); norm == "" || norm == string(KeyTypeECDSA) {
		return KeyTypeECDSA, nil
	}
	scheme, err := pqc.ParseScheme(s)
	if err != nil {
		return "", fmt.Errorf("unsupported key type %q: use one of ecdsa, ml_dsa_44, fn_dsa_512", s)
	}
	return KeyTypeFromPQScheme(scheme)
}

// IsPQ reports whether k is a post-quantum key type.
func (k KeyType) IsPQ() bool {
	_, ok := k.PQScheme()
	return ok
}

// PQScheme returns the TIP-899 scheme for a post-quantum key type. It
// returns false for ECDSA and unknown key types.
func (k KeyType) PQScheme() (core.PQScheme, bool) {
	switch k {
	case KeyTypeMLDSA44:
		return core.PQScheme_ML_DSA_44, true
	case KeyTypeFNDSA512:
		return core.PQScheme_FN_DSA_512, true
	}
	return core.PQScheme_UNKNOWN_PQ_SCHEME, false
}

// KeyTypeFromPQScheme maps a TIP-899 scheme to its key type.
func KeyTypeFromPQScheme(scheme core.PQScheme) (KeyType, error) {
	switch scheme {
	case core.PQScheme_ML_DSA_44:
		return KeyTypeMLDSA44, nil
	case core.PQScheme_FN_DSA_512:
		return KeyTypeFNDSA512, nil
	}
	return "", fmt.Errorf("unsupported post-quantum scheme %s", scheme)
}
