package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_MissingFile(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/policy.yaml")
	require.NoError(t, err)
	assert.Empty(t, cfg.Wallets)
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte(""), 0600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Empty(t, cfg.Wallets)
}

func TestLoadConfig_Disabled(t *testing.T) {
	yaml := `
enabled: false
wallets:
  savings:
    per_tx_limit_trx: 1000
`
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
	assert.Len(t, cfg.Wallets, 1)
}

func TestLoadConfig_ValidConfig(t *testing.T) {
	yaml := `
enabled: true
wallets:
  savings:
    per_tx_limit_trx: 1000
    daily_limit_trx: 5000
    whitelist:
      - "TKX..."
      - "TAB..."
    approval_required_above_trx: 500
  petty-cash:
    per_tx_limit_trx: 100
    daily_limit_trx: 500
`
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Wallets, 2)

	savings := cfg.GetPolicy("savings")
	require.NotNil(t, savings)
	assert.Equal(t, float64(1000), savings.PerTxLimitTRX)
	assert.Equal(t, float64(5000), savings.DailyLimitTRX)
	assert.Equal(t, []string{"TKX...", "TAB..."}, savings.Whitelist)
	assert.Equal(t, float64(500), savings.ApprovalRequiredAboveTRX)

	petty := cfg.GetPolicy("petty-cash")
	require.NotNil(t, petty)
	assert.Equal(t, float64(100), petty.PerTxLimitTRX)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte("not: [valid: yaml"), 0600))

	_, err := LoadConfig(path)
	assert.Error(t, err)
}

func TestLoadConfig_NegativeLimit(t *testing.T) {
	yaml := `
wallets:
  bad:
    per_tx_limit_trx: -100
`
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0600))

	_, err := LoadConfig(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "per_tx_limit_trx must be >= 0")
}

func TestGetPolicy_NoConfig(t *testing.T) {
	var cfg *Config
	assert.Nil(t, cfg.GetPolicy("anything"))
}

func TestGetPolicy_UnknownWallet(t *testing.T) {
	cfg := &Config{Wallets: map[string]*WalletPolicy{
		"known": {PerTxLimitTRX: 100},
	}}
	assert.Nil(t, cfg.GetPolicy("unknown"))
	assert.NotNil(t, cfg.GetPolicy("known"))
}

func TestResolveDecimals(t *testing.T) {
	t.Run("NilConfig", func(t *testing.T) {
		var cfg *Config
		cfg.ResolveDecimals(nil) // should not panic
	})

	t.Run("TRX_auto_6", func(t *testing.T) {
		cfg := &Config{
			Wallets: map[string]*WalletPolicy{
				"w": {
					TokenLimits: map[string]*TokenLimit{
						"TRX": {DailyLimitUnits: 100},
					},
				},
			},
		}
		cfg.ResolveDecimals(nil)
		assert.Equal(t, 6, cfg.Wallets["w"].TokenLimits["TRX"].Decimals)
	})

	t.Run("MockResolver", func(t *testing.T) {
		cfg := &Config{
			Wallets: map[string]*WalletPolicy{
				"w": {
					TokenLimits: map[string]*TokenLimit{
						"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t": {DailyLimitUnits: 100},
					},
				},
			},
		}
		resolver := func(contract string) (int, error) {
			if contract == "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t" {
				return 6, nil
			}
			return 0, fmt.Errorf("unknown token")
		}
		cfg.ResolveDecimals(resolver)
		assert.Equal(t, 6, cfg.Wallets["w"].TokenLimits["TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"].Decimals)
	})

	t.Run("ResolverError", func(t *testing.T) {
		cfg := &Config{
			Wallets: map[string]*WalletPolicy{
				"w": {
					TokenLimits: map[string]*TokenLimit{
						"TBadContract": {DailyLimitUnits: 100},
					},
				},
			},
		}
		resolver := func(contract string) (int, error) {
			return 0, fmt.Errorf("network error")
		}
		cfg.ResolveDecimals(resolver)
		// Decimals stays at 0 on error
		assert.Equal(t, 0, cfg.Wallets["w"].TokenLimits["TBadContract"].Decimals)
	})

	t.Run("AlreadySet", func(t *testing.T) {
		cfg := &Config{
			Wallets: map[string]*WalletPolicy{
				"w": {
					TokenLimits: map[string]*TokenLimit{
						"TContract": {Decimals: 18, DailyLimitUnits: 100},
					},
				},
			},
		}
		called := false
		resolver := func(contract string) (int, error) {
			called = true
			return 6, nil
		}
		cfg.ResolveDecimals(resolver)
		assert.False(t, called, "resolver should not be called when decimals already set")
		assert.Equal(t, 18, cfg.Wallets["w"].TokenLimits["TContract"].Decimals)
	})
}

func TestNormalizeTokenLimitKeys(t *testing.T) {
	t.Run("HexToBase58", func(t *testing.T) {
		// 41 + 20 bytes hex = valid TRON hex address
		// Use a known address: TKzxdSv2FZKQrEqkKVgp5DcwEXBEKMg2Ax = 4162b2de53e4de5f8dd24c08dc03e9c9fa5dae7940
		hexAddr := "4162b2de53e4de5f8dd24c08dc03e9c9fa5dae7940"
		p := &WalletPolicy{
			TokenLimits: map[string]*TokenLimit{
				hexAddr: {PerTxLimitUnits: 100},
			},
		}
		normalizeTokenLimitKeys(p)
		// The hex key should be gone
		assert.Nil(t, p.TokenLimits[hexAddr])
		// Should have a base58 key instead
		assert.Len(t, p.TokenLimits, 1)
		for key, tl := range p.TokenLimits {
			assert.NotEqual(t, hexAddr, key)
			assert.Equal(t, float64(100), tl.PerTxLimitUnits)
		}
	})

	t.Run("NilEntriesSkipped", func(t *testing.T) {
		p := &WalletPolicy{
			TokenLimits: map[string]*TokenLimit{
				"TRX":       {PerTxLimitUnits: 50},
				"TNilToken": nil,
			},
		}
		normalizeTokenLimitKeys(p)
		assert.Len(t, p.TokenLimits, 1)
		assert.NotNil(t, p.TokenLimits["TRX"])
	})

	t.Run("NilTokenLimits", func(t *testing.T) {
		p := &WalletPolicy{}
		normalizeTokenLimitKeys(p) // should not panic
	})
}

func TestValidatePolicy_NegativeApprovalUSD(t *testing.T) {
	yamlData := `
wallets:
  bad:
    approval_required_above_usd: -10
`
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yamlData), 0600))

	_, err := LoadConfig(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "approval_required_above_usd must be >= 0")
}

func TestValidatePolicy_NegativeTokenApproval(t *testing.T) {
	yamlData := `
wallets:
  bad:
    token_limits:
      TRX:
        approval_required_above_units: -5
`
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yamlData), 0600))

	_, err := LoadConfig(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "limits must be >= 0")
}

func TestLoadConfig_NilWallet(t *testing.T) {
	yamlData := `
enabled: true
wallets:
  empty_wallet:
`
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yamlData), 0600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Contains(t, cfg.Wallets, "empty_wallet")
	assert.NotNil(t, cfg.Wallets["empty_wallet"])
}

func TestNormalizeAddress(t *testing.T) {
	t.Run("HexToBase58", func(t *testing.T) {
		hexAddr := "4162b2de53e4de5f8dd24c08dc03e9c9fa5dae7940"
		result := normalizeAddress(hexAddr)
		assert.NotEqual(t, hexAddr, result)
		assert.True(t, len(result) > 0)
		// Should start with T (TRON base58 prefix)
		assert.Equal(t, "T", result[:1])
	})

	t.Run("AlreadyBase58", func(t *testing.T) {
		addr := "TKzxdSv2FZKQrEqkKVgp5DcwEXBEKMg2Ax"
		result := normalizeAddress(addr)
		assert.Equal(t, addr, result)
	})

	t.Run("Invalid", func(t *testing.T) {
		addr := "not-an-address"
		result := normalizeAddress(addr)
		// Invalid addresses are returned as-is
		assert.Equal(t, addr, result)
	})
}

func TestLoadConfig_ApprovalValidation(t *testing.T) {
	t.Run("TelegramMissingSection", func(t *testing.T) {
		yaml := `
enabled: true
approval:
  method: telegram
wallets: {}
`
		path := filepath.Join(t.TempDir(), "policy.yaml")
		require.NoError(t, os.WriteFile(path, []byte(yaml), 0600))
		_, err := LoadConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "requires telegram config section")
	})

	t.Run("TelegramMissingBotTokenEnv", func(t *testing.T) {
		yaml := `
enabled: true
approval:
  method: telegram
  telegram:
    chat_id: 123
    authorized_users: [456]
wallets: {}
`
		path := filepath.Join(t.TempDir(), "policy.yaml")
		require.NoError(t, os.WriteFile(path, []byte(yaml), 0600))
		_, err := LoadConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "bot_token_env is required")
	})

	t.Run("TelegramMissingAuthorizedUsers", func(t *testing.T) {
		yaml := `
enabled: true
approval:
  method: telegram
  telegram:
    bot_token_env: MY_TOKEN
    chat_id: 123
wallets: {}
`
		path := filepath.Join(t.TempDir(), "policy.yaml")
		require.NoError(t, os.WriteFile(path, []byte(yaml), 0600))
		_, err := LoadConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "authorized_users requires at least one")
	})

	t.Run("TelegramValid", func(t *testing.T) {
		yaml := `
enabled: true
approval:
  method: telegram
  telegram:
    bot_token_env: MY_TOKEN
    chat_id: 123
    authorized_users: [456]
wallets: {}
`
		path := filepath.Join(t.TempDir(), "policy.yaml")
		require.NoError(t, os.WriteFile(path, []byte(yaml), 0600))
		cfg, err := LoadConfig(path)
		require.NoError(t, err)
		assert.Equal(t, "telegram", cfg.Approval.Method)
	})
}
