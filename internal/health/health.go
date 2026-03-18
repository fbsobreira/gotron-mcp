package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/version"
)

const cacheTTL = 10 * time.Second

// Handler serves the /health endpoint with a cached node status check.
type Handler struct {
	pool    *nodepool.Pool
	network string

	mu     sync.RWMutex
	cache  []byte
	status int
	expiry time.Time
}

// NewHandler creates a new health check handler.
func NewHandler(pool *nodepool.Pool, network string) *Handler {
	return &Handler{
		pool:    pool,
		network: network,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Fast path: serve from cache under read lock
	h.mu.RLock()
	if time.Now().Before(h.expiry) {
		status := h.status
		cache := h.cache
		h.mu.RUnlock()
		w.WriteHeader(status)
		_, _ = w.Write(cache)
		return
	}
	h.mu.RUnlock()

	// Slow path: refresh under write lock
	h.mu.Lock()
	// Double-check after acquiring write lock
	if time.Now().Before(h.expiry) {
		status := h.status
		cache := h.cache
		h.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write(cache)
		return
	}

	body, statusCode := h.check()
	h.cache = body
	h.status = statusCode
	h.expiry = time.Now().Add(cacheTTL)
	h.mu.Unlock()

	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

func (h *Handler) check() ([]byte, int) {
	nodeStatus := "ok"
	var latestBlock int64

	h.pool.CheckHealth()
	block, err := h.pool.Client().GetNowBlockCtx(context.Background())
	if err != nil {
		nodeStatus = fmt.Sprintf("error: %v", err)
	} else if block.BlockHeader != nil && block.BlockHeader.RawData != nil {
		latestBlock = block.BlockHeader.RawData.Number
	}

	status := "ok"
	statusCode := http.StatusOK
	if nodeStatus != "ok" {
		status = "degraded"
		statusCode = http.StatusServiceUnavailable
	}

	body, err := json.Marshal(map[string]any{
		"status":  status,
		"version": version.Full(),
		"network": h.network,
		"node": map[string]any{
			"status":       nodeStatus,
			"latest_block": latestBlock,
		},
		"project": map[string]any{
			"name":       "GoTRON MCP",
			"site":       "https://gotron.sh",
			"repository": "https://github.com/fbsobreira/gotron-mcp",
			"community":  "CryptoChain",
			"sr_address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF",
		},
	})
	if err != nil {
		log.Printf("health: failed to marshal response: %v", err)
		return []byte(`{"status":"error"}`), http.StatusInternalServerError
	}

	return body, statusCode
}
