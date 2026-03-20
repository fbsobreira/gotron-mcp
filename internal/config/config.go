package config

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all configuration for the GoTRON MCP server.
type Config struct {
	Node            string
	APIKey          string
	Network         string
	Transport       string
	Port            int
	Bind            string
	FallbackNode    string
	FallbackNodeTLS bool
	Keystore        string
	TLS             bool
	AuthToken       string
	AuthTokenFile   string
	RateLimit       int
	TrustedProxy    string
	KeystoreDir     string
	KeystorePass    string
	RequirePolicy   bool
}

var networkNodes = map[string]string{
	"mainnet": "grpc.trongrid.io:50051",
	"nile":    "grpc.nile.trongrid.io:50051",
	"shasta":  "grpc.shasta.trongrid.io:50051",
}

// Parse reads CLI flags and environment variables into a Config.
func Parse() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.Node, "node", envOrDefault("GOTRON_MCP_NODE", ""), "TRON gRPC node address")
	flag.StringVar(&cfg.APIKey, "api-key", envOrDefault("GOTRON_NODE_API_KEY", ""), "TronGrid API key")
	flag.StringVar(&cfg.Network, "network", envOrDefault("GOTRON_MCP_NETWORK", "mainnet"), "Network: mainnet, nile, shasta")
	flag.StringVar(&cfg.Transport, "transport", "stdio", "Transport: stdio, http")
	flag.IntVar(&cfg.Port, "port", 8080, "HTTP server port")
	flag.StringVar(&cfg.Bind, "bind", "127.0.0.1", "HTTP server bind address")
	flag.StringVar(&cfg.FallbackNode, "fallback-node", envOrDefault("GOTRON_MCP_FALLBACK_NODE", ""), "Fallback TRON gRPC node address")
	flag.BoolVar(&cfg.FallbackNodeTLS, "fallback-tls", envOrDefault("GOTRON_MCP_FALLBACK_TLS", "") == "true", "Use TLS for fallback node connection")
	flag.StringVar(&cfg.Keystore, "keystore", "", "Path to tronctl keystore directory")
	flag.BoolVar(&cfg.TLS, "tls", envOrDefault("GOTRON_MCP_TLS", "") == "true", "Use TLS for gRPC connection (default: plaintext)")
	flag.StringVar(&cfg.AuthToken, "auth-token", envOrDefault("GOTRON_MCP_AUTH_TOKEN", ""), "Bearer token for HTTP authentication")
	flag.StringVar(&cfg.AuthTokenFile, "auth-token-file", envOrDefault("GOTRON_MCP_AUTH_TOKEN_FILE", ""), "Path to file with bearer tokens (one per line, hot-reloaded)")
	flag.IntVar(&cfg.RateLimit, "rate-limit", envOrDefaultInt("GOTRON_MCP_RATE_LIMIT", 0), "Max requests per minute per IP (0 = unlimited)")
	flag.StringVar(&cfg.TrustedProxy, "trusted-proxy", envOrDefault("GOTRON_MCP_TRUSTED_PROXY", "none"), "Trusted proxy mode: cloudflare, all, none")
	flag.StringVar(&cfg.KeystoreDir, "keystore-dir", envOrDefault("GOTRON_MCP_KEYSTORE_DIR", ""), "Path to MCP wallet directory (default: ~/.gotron-mcp/wallets/)")
	flag.StringVar(&cfg.KeystorePass, "keystore-pass", envOrDefault("GOTRON_MCP_KEYSTORE_PASSPHRASE", ""), "Passphrase for keystore encryption")
	flag.BoolVar(&cfg.RequirePolicy, "require-policy", envOrDefault("GOTRON_MCP_REQUIRE_POLICY", "") == "true", "Refuse to sign if no policy config exists")

	flag.Parse()

	if cfg.RateLimit < 0 {
		log.Fatalf("invalid --rate-limit %d: must be >= 0", cfg.RateLimit)
	}

	switch strings.ToLower(strings.TrimSpace(cfg.TrustedProxy)) {
	case "", "none":
		cfg.TrustedProxy = "none"
	case "all":
		cfg.TrustedProxy = "all"
	case "cloudflare":
		cfg.TrustedProxy = "cloudflare"
	default:
		log.Fatalf("invalid --trusted-proxy %q: must be one of none, all, cloudflare", cfg.TrustedProxy)
	}

	if cfg.Node == "" {
		cfg.Node = resolveNode(cfg.Network)
	}

	if cfg.KeystoreDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cfg.KeystoreDir = filepath.Join(home, ".gotron-mcp", "wallets")
		}
	}

	return cfg
}

// resolveNode returns the gRPC endpoint for the given network name,
// falling back to mainnet if the network is unknown.
func resolveNode(network string) string {
	if node, ok := networkNodes[network]; ok {
		return node
	}
	return networkNodes["mainnet"]
}

// IsHostedMode returns true when running in HTTP (hosted) mode.
func (c *Config) IsHostedMode() bool {
	return c.Transport == "http"
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		log.Printf("warning: invalid value %q for %s, using default %d", val, key, fallback)
		return fallback
	}
	if n < 0 {
		log.Printf("warning: negative value %d for %s, using default %d", n, key, fallback)
		return fallback
	}
	return n
}
