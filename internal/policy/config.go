package policy

import (
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"gopkg.in/yaml.v3"
)

// TokenLimit defines spend limits for a specific token.
// Limits are in human-readable units (e.g., 50 USDT, not 50000000).
// Set decimals so the engine can convert to raw on-chain units.
type TokenLimit struct {
	Decimals                   int     `yaml:"decimals"`
	DailyLimitUnits            float64 `yaml:"daily_limit_units"`
	DailyLimitUSD              float64 `yaml:"daily_limit_usd"`
	PerTxLimitUnits            float64 `yaml:"per_tx_limit_units"`
	PerTxLimitUSD              float64 `yaml:"per_tx_limit_usd"`
	ApprovalRequiredAboveUnits float64 `yaml:"approval_required_above_units"`
}

// RawPerTxLimit returns the per-TX limit scaled to raw on-chain units.
func (tl *TokenLimit) RawPerTxLimit() float64 {
	return tl.PerTxLimitUnits * decimalMultiplier(tl.Decimals)
}

// RawDailyLimit returns the daily limit scaled to raw on-chain units.
func (tl *TokenLimit) RawDailyLimit() float64 {
	return tl.DailyLimitUnits * decimalMultiplier(tl.Decimals)
}

// DecimalResolver fetches token decimals from the network.
type DecimalResolver func(contractAddress string) (int, error)

// ResolveDecimals fills in missing decimals for token_limits entries.
// TRX always gets 6. TRC20 tokens use the resolver to fetch from chain.
// Tokens that fail to resolve are logged and left at 0 (limits will be
// in raw on-chain units).
func (c *Config) ResolveDecimals(resolver DecimalResolver) {
	if c == nil {
		return
	}
	for _, wp := range c.Wallets {
		if wp == nil {
			continue
		}
		for token, tl := range wp.TokenLimits {
			if tl == nil || tl.Decimals > 0 {
				continue // already set
			}
			if token == "TRX" {
				tl.Decimals = 6
				continue
			}
			if resolver == nil {
				continue
			}
			d, err := resolver(token)
			if err != nil {
				log.Printf("warning: failed to resolve decimals for token %s: %v (using raw units)", token, err)
				continue
			}
			tl.Decimals = d
			log.Printf("Resolved decimals for token %s: %d", token, d)
		}
	}
}

// RawApprovalThreshold returns the approval threshold scaled to raw on-chain units.
func (tl *TokenLimit) RawApprovalThreshold() float64 {
	return tl.ApprovalRequiredAboveUnits * decimalMultiplier(tl.Decimals)
}

func decimalMultiplier(decimals int) float64 {
	return math.Pow(10, float64(decimals))
}

// WalletPolicy defines spend limits and restrictions for a single wallet.
type WalletPolicy struct {
	// Aggregate USD limits across all tokens
	DailyLimitUSD            float64 `yaml:"daily_limit_usd"`
	PerTxLimitUSD            float64 `yaml:"per_tx_limit_usd"`
	ApprovalRequiredAboveUSD float64 `yaml:"approval_required_above_usd"`

	// Legacy TRX-only limits (aliases for token_limits.TRX)
	PerTxLimitTRX            float64 `yaml:"per_tx_limit_trx"`
	DailyLimitTRX            float64 `yaml:"daily_limit_trx"`
	ApprovalRequiredAboveTRX float64 `yaml:"approval_required_above_trx"`

	// Per-token limits (key: "TRX" or TRC20 contract address)
	TokenLimits map[string]*TokenLimit `yaml:"token_limits"`

	Whitelist []string `yaml:"whitelist"`
}

// ApprovalConfig holds configuration for the transaction approval backend.
type ApprovalConfig struct {
	Method   string              `yaml:"method"` // "telegram", "webhook" (future)
	Telegram *TelegramYAMLConfig `yaml:"telegram"`
}

// TelegramYAMLConfig holds Telegram-specific approval configuration.
type TelegramYAMLConfig struct {
	BotTokenEnv     string  `yaml:"bot_token_env"`    // env var name for bot token
	AuthorizedUsers []int64 `yaml:"authorized_users"` // Telegram user IDs
	ChatID          int64   `yaml:"chat_id"`          // Chat to send approvals to
	TimeoutSeconds  int     `yaml:"timeout_seconds"`  // default 300
}

// Config holds the per-wallet policy configuration.
type Config struct {
	Enabled  bool                     `yaml:"enabled"`
	Wallets  map[string]*WalletPolicy `yaml:"wallets"`
	Approval *ApprovalConfig          `yaml:"approval"`
}

// LoadConfig reads and parses a policy YAML file.
// Returns an empty config (no restrictions) if the file doesn't exist.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading policy config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing policy config: %w", err)
	}

	if cfg.Wallets == nil {
		cfg.Wallets = make(map[string]*WalletPolicy)
	}

	// Validate, normalize, and promote legacy fields
	for name, p := range cfg.Wallets {
		if p == nil {
			cfg.Wallets[name] = &WalletPolicy{}
			continue
		}
		if err := validatePolicy(name, p); err != nil {
			return nil, err
		}
		normalizeWhitelist(p)
		normalizeTokenLimitKeys(p)
		promoteLegacyTRXLimits(p)
	}

	return &cfg, nil
}

func validatePolicy(name string, p *WalletPolicy) error {
	if p.PerTxLimitTRX < 0 {
		return fmt.Errorf("wallet %q: per_tx_limit_trx must be >= 0", name)
	}
	if p.DailyLimitTRX < 0 {
		return fmt.Errorf("wallet %q: daily_limit_trx must be >= 0", name)
	}
	if p.ApprovalRequiredAboveTRX < 0 {
		return fmt.Errorf("wallet %q: approval_required_above_trx must be >= 0", name)
	}
	if p.ApprovalRequiredAboveUSD < 0 {
		return fmt.Errorf("wallet %q: approval_required_above_usd must be >= 0", name)
	}
	if p.DailyLimitUSD < 0 {
		return fmt.Errorf("wallet %q: daily_limit_usd must be >= 0", name)
	}
	if p.PerTxLimitUSD < 0 {
		return fmt.Errorf("wallet %q: per_tx_limit_usd must be >= 0", name)
	}
	for token, tl := range p.TokenLimits {
		if tl == nil {
			continue
		}
		if tl.DailyLimitUnits < 0 || tl.DailyLimitUSD < 0 || tl.PerTxLimitUnits < 0 || tl.PerTxLimitUSD < 0 || tl.ApprovalRequiredAboveUnits < 0 {
			return fmt.Errorf("wallet %q, token %q: limits must be >= 0", name, token)
		}
	}
	return nil
}

// normalizeTokenLimitKeys converts token_limits keys from hex to base58.
func normalizeTokenLimitKeys(p *WalletPolicy) {
	if p.TokenLimits == nil {
		return
	}
	normalized := make(map[string]*TokenLimit, len(p.TokenLimits))
	for key, tl := range p.TokenLimits {
		if tl == nil {
			continue
		}
		normalized[normalizeAddress(key)] = tl
	}
	p.TokenLimits = normalized
}

func normalizeWhitelist(p *WalletPolicy) {
	for i, addr := range p.Whitelist {
		p.Whitelist[i] = normalizeAddress(addr)
	}
}

// promoteLegacyTRXLimits copies per_tx_limit_trx / daily_limit_trx into
// token_limits["TRX"] if not already set, for backwards compatibility.
func promoteLegacyTRXLimits(p *WalletPolicy) {
	if p.PerTxLimitTRX == 0 && p.DailyLimitTRX == 0 {
		return
	}
	if p.TokenLimits == nil {
		p.TokenLimits = make(map[string]*TokenLimit)
	}
	if _, exists := p.TokenLimits["TRX"]; !exists {
		p.TokenLimits["TRX"] = &TokenLimit{Decimals: 6}
	}
	trx := p.TokenLimits["TRX"]
	if trx.Decimals == 0 {
		trx.Decimals = 6 // 1 TRX = 1,000,000 SUN
	}
	if trx.PerTxLimitUnits == 0 && p.PerTxLimitTRX > 0 {
		trx.PerTxLimitUnits = p.PerTxLimitTRX
	}
	if trx.DailyLimitUnits == 0 && p.DailyLimitTRX > 0 {
		trx.DailyLimitUnits = p.DailyLimitTRX
	}
}

// GetPolicy returns the policy for a wallet, or nil if no policy is configured.
// A nil return means the wallet is unrestricted.
func (c *Config) GetPolicy(wallet string) *WalletPolicy {
	if c == nil || c.Wallets == nil {
		return nil
	}
	return c.Wallets[wallet]
}

// normalizeAddress converts a TRON address to canonical base58 format.
func normalizeAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if strings.HasPrefix(addr, "41") && len(addr) == 42 {
		a, err := address.HexToAddress(addr)
		if err == nil {
			return a.String()
		}
	}
	return addr
}
