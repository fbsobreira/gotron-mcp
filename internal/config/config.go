package config

import (
	"flag"
	"os"
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

	flag.Parse()

	if cfg.Node == "" {
		cfg.Node = resolveNode(cfg.Network)
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
