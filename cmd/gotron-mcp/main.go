package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/auth"
	"github.com/fbsobreira/gotron-mcp/internal/config"
	"github.com/fbsobreira/gotron-mcp/internal/health"
	"github.com/fbsobreira/gotron-mcp/internal/middleware"
	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	mcpserver "github.com/fbsobreira/gotron-mcp/internal/server"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg := config.Parse()

	var opts []grpc.DialOption
	if cfg.TLS {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	pool, err := nodepool.New(cfg.Node, opts)
	if err != nil {
		log.Fatalf("failed to connect to TRON node %s: %v", cfg.Node, err)
	}
	defer pool.Stop()

	if cfg.FallbackNode != "" {
		var fallbackOpts []grpc.DialOption
		if cfg.FallbackNodeTLS {
			fallbackOpts = append(fallbackOpts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
		} else {
			fallbackOpts = append(fallbackOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
		if err := pool.AddFallback(cfg.FallbackNode, fallbackOpts); err != nil {
			log.Printf("warning: failed to connect to fallback node %s: %v", cfg.FallbackNode, err)
		} else {
			log.Printf("Fallback node connected: %s", cfg.FallbackNode)
		}
	}

	if cfg.APIKey != "" {
		if err := pool.SetAPIKey(cfg.APIKey); err != nil {
			log.Fatalf("failed to set API key: %v", err)
		}
	}

	s, wm := mcpserver.New(cfg, pool)
	if wm != nil {
		defer wm.Close()
	}

	switch cfg.Transport {
	case "stdio":
		if err := server.ServeStdio(s); err != nil {
			log.Fatalf("stdio server error: %v", err)
		}
	case "http":
		httpTransport := server.NewStreamableHTTPServer(s,
			server.WithEndpointPath("/mcp"),
		)

		if cfg.AuthToken != "" && cfg.AuthTokenFile != "" {
			log.Fatalf("--auth-token and --auth-token-file are mutually exclusive")
		}

		var mcpHandler http.Handler = httpTransport
		switch {
		case cfg.AuthTokenFile != "":
			store, err := auth.NewTokenStore(cfg.AuthTokenFile)
			if err != nil {
				log.Fatalf("failed to load auth token file: %v", err)
			}
			defer store.Stop()
			if err := store.Watch(); err != nil {
				log.Printf("warning: token file watch failed, hot-reload disabled: %v", err)
			}
			mcpHandler = store.Middleware(httpTransport)
			log.Printf("HTTP authentication enabled (token file)")
		case cfg.AuthToken != "":
			mcpHandler = auth.BearerAuth(cfg.AuthToken, httpTransport)
			log.Printf("HTTP authentication enabled (single token)")
		}

		// Rate limiter wraps auth: applied before authentication to reject
		// excess traffic early. Trade-off: unauthenticated requests consume
		// rate limit tokens for the source IP.
		if cfg.RateLimit > 0 {
			trustMode := middleware.ParseTrustMode(cfg.TrustedProxy)
			rl := middleware.NewRateLimiter(cfg.RateLimit, trustMode)
			defer rl.Stop()
			mcpHandler = rl.Wrap(mcpHandler)
			log.Printf("Rate limiting enabled: %d req/min per IP (trusted-proxy: %s)", cfg.RateLimit, cfg.TrustedProxy)
		}

		mux := http.NewServeMux()
		mux.Handle("/health", health.NewHandler(pool, cfg.Network))
		mux.Handle("/mcp", mcpHandler)

		addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
		log.Printf("GoTRON MCP server starting on %s", addr)
		httpServer := &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       15 * time.Second,
			IdleTimeout:       120 * time.Second,
		}

		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			log.Println("Shutting down server...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := httpServer.Shutdown(ctx); err != nil {
				log.Printf("HTTP shutdown error: %v", err)
			}
		}()

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	default:
		log.Fatalf("unknown transport: %s (use stdio or http)", cfg.Transport)
	}
}
