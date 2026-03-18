package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiterAllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter(60, TrustNone)
	defer rl.Stop()

	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 60; i++ {
		req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got status %d, want 200", i, rec.Code)
		}
	}
}

func TestRateLimiterBlocks(t *testing.T) {
	rl := NewRateLimiter(5, TrustNone)
	defer rl.Stop()

	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust the burst (5 requests).
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got status %d, want 200", i, rec.Code)
		}
	}

	// Next request should be rate limited.
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got status %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header")
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", rec.Header().Get("Content-Type"))
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "rate limit exceeded" {
		t.Errorf("error = %q, want %q", body["error"], "rate limit exceeded")
	}
}

func TestRateLimiterPerIP(t *testing.T) {
	rl := NewRateLimiter(2, TrustNone)
	defer rl.Stop()

	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust limit for IP A.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		req.RemoteAddr = "1.1.1.1:1111"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// IP A should be blocked.
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.RemoteAddr = "1.1.1.1:1111"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("IP A: got status %d, want 429", rec.Code)
	}

	// IP B should still be allowed.
	req = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.RemoteAddr = "2.2.2.2:2222"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("IP B: got status %d, want 200", rec.Code)
	}
}

func TestRateLimiterStopIdempotent(t *testing.T) {
	rl := NewRateLimiter(10, TrustNone)
	rl.Stop()
	rl.Stop() // should not panic
}

func TestRateLimiterVisitorCap(t *testing.T) {
	rl := NewRateLimiter(100, TrustNone)
	defer rl.Stop()
	rl.maxSize = 3

	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Fill up to the cap with 4 unique IPs.
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		req.RemoteAddr = fmt.Sprintf("10.0.0.%d:1234", i+1)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	rl.mu.Lock()
	count := len(rl.visitors)
	rl.mu.Unlock()

	if count > 3 {
		t.Errorf("visitor count = %d, want <= 3", count)
	}
}

func TestRateLimiterUsesClientIP(t *testing.T) {
	rl := NewRateLimiter(1, TrustAll)
	defer rl.Stop()

	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request with X-Real-IP should pass.
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Real-IP", "5.5.5.5")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: got status %d, want 200", rec.Code)
	}

	// Second request with same X-Real-IP should be blocked.
	req = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Real-IP", "5.5.5.5")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got status %d, want 429", rec.Code)
	}
}
