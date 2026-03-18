package middleware

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// visitor tracks a rate limiter and the last time it was seen.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter provides per-IP rate limiting using a token bucket algorithm.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	maxSize  int
	rpm      int
	trust    TrustMode
	done     chan struct{}
	stopOnce sync.Once
}

// NewRateLimiter creates a rate limiter allowing rpm requests per minute per IP.
// It starts a background goroutine to clean up stale entries every minute.
const defaultMaxVisitors = 10000

func NewRateLimiter(rpm int, trust TrustMode) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		maxSize:  defaultMaxVisitors,
		rpm:      rpm,
		trust:    trust,
		done:     make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Stop halts the background cleanup goroutine. It is safe to call multiple times.
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() { close(rl.done) })
}

// Wrap returns HTTP middleware that rate-limits requests by client IP.
// It should be applied only to the endpoint that needs limiting.
func (rl *RateLimiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r, rl.trust)
		lim := rl.getLimiter(ip)

		if !lim.Allow() {
			retryAfter := math.Ceil(float64(time.Minute) / float64(time.Second) / float64(rl.rpm))
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter)))
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "rate limit exceeded",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, ok := rl.visitors[ip]
	if !ok {
		// Evict oldest entry if at capacity to bound memory growth.
		if len(rl.visitors) >= rl.maxSize {
			rl.evictOldest()
		}
		// Token bucket: refill rate = rpm/60 per second, burst = rpm.
		lim := rate.NewLimiter(rate.Limit(float64(rl.rpm)/60.0), rl.rpm)
		rl.visitors[ip] = &visitor{limiter: lim, lastSeen: time.Now()}
		return lim
	}
	v.lastSeen = time.Now()
	return v.limiter
}

// evictOldest removes the least recently seen visitor. Must be called with mu held.
func (rl *RateLimiter) evictOldest() {
	var oldestIP string
	var oldestTime time.Time
	first := true
	for ip, v := range rl.visitors {
		if first || v.lastSeen.Before(oldestTime) {
			oldestIP = ip
			oldestTime = v.lastSeen
			first = false
		}
	}
	if oldestIP != "" {
		delete(rl.visitors, oldestIP)
	}
}

// cleanup removes visitors that haven't been seen for 3 minutes.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			for ip, v := range rl.visitors {
				if time.Since(v.lastSeen) > 3*time.Minute {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.done:
			return
		}
	}
}
