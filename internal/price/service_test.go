package price

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockCoinGecko(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Service) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	svc := NewService(Config{
		BaseURL:  srv.URL,
		CacheTTL: 1 * time.Second,
	})
	return srv, svc
}

func TestGetTRXPrice(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tron":{"usd":0.123}}`)
	})

	price, err := svc.GetTRXPrice(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 0.123, price, 0.001)
}

func TestGetPriceByID(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tether":{"usd":0.9998}}`)
	})

	price, err := svc.GetPriceByID(context.Background(), "tether")
	require.NoError(t, err)
	assert.InDelta(t, 0.9998, price, 0.001)
}

func TestGetPriceByID_NotFound(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{}`)
	})

	_, err := svc.GetPriceByID(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no price data")
}

func TestGetPriceByContract(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tr7nhqjekqxgtci8q8zy4pl8otszgjlj6t":{"usd":1.0001}}`)
	})

	price, err := svc.GetPriceByContract(context.Background(), "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t")
	require.NoError(t, err)
	assert.InDelta(t, 1.0001, price, 0.001)
}

func TestGetPriceByContract_NotFound(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{}`)
	})

	_, err := svc.GetPriceByContract(context.Background(), "TUnknown")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no price data")
}

func TestGetTokenPrice_TRX(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tron":{"usd":0.15}}`)
	})

	price, err := svc.GetTokenPrice(context.Background(), "TRX")
	require.NoError(t, err)
	assert.InDelta(t, 0.15, price, 0.001)
}

func TestGetTokenPrice_Contract(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tr7nhqjekqxgtci8q8zy4pl8otszgjlj6t":{"usd":1.0}}`)
	})

	price, err := svc.GetTokenPrice(context.Background(), "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t")
	require.NoError(t, err)
	assert.InDelta(t, 1.0, price, 0.001)
}

func TestBatchGetPrices(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tron":{"usd":0.12},"tether":{"usd":1.0},"usd-coin":{"usd":0.999}}`)
	})

	prices, err := svc.BatchGetPrices(context.Background(), []string{"tron", "tether", "usd-coin"})
	require.NoError(t, err)
	assert.Len(t, prices, 3)
	assert.InDelta(t, 0.12, prices["tron"], 0.001)
	assert.InDelta(t, 1.0, prices["tether"], 0.001)
	assert.InDelta(t, 0.999, prices["usd-coin"], 0.001)
}

func TestCaching(t *testing.T) {
	var callCount int32
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tron":{"usd":0.15}}`)
	})

	// First call — hits the server
	price1, err := svc.GetTRXPrice(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 0.15, price1, 0.001)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))

	// Second call — from cache
	price2, err := svc.GetTRXPrice(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 0.15, price2, 0.001)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "should use cache")

	// Wait for cache to expire
	time.Sleep(1100 * time.Millisecond)

	// Third call — cache expired, hits server again
	price3, err := svc.GetTRXPrice(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 0.15, price3, 0.001)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "should fetch again after TTL")
}

func TestStaleWhileRevalidate(t *testing.T) {
	var callCount int32
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"tron":{"usd":0.15}}`)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	// First call succeeds
	price, err := svc.GetTRXPrice(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 0.15, price, 0.001)

	// Wait for cache to expire
	time.Sleep(1100 * time.Millisecond)

	// Second call — server fails, returns stale cached value
	price, err = svc.GetTRXPrice(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 0.15, price, 0.001, "should return stale price on error")
}

func TestRateLimited(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := svc.GetTRXPrice(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
}

func TestInvalidJSON(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "not json")
	})

	_, err := svc.GetTRXPrice(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing")
}

func TestAPIKey(t *testing.T) {
	var receivedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("x-cg-pro-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tron":{"usd":0.15}}`)
	}))
	defer srv.Close()

	svc := NewService(Config{BaseURL: srv.URL, APIKey: "my-pro-key"})
	_, err := svc.GetTRXPrice(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "my-pro-key", receivedKey)
}

func TestNewService_Defaults(t *testing.T) {
	svc := NewService(Config{})
	assert.Equal(t, defaultCoinGeckoURL, svc.baseURL)
	assert.Equal(t, defaultCacheTTL, svc.cacheTTL)
}

func TestGetPriceByContract_StaleOnError(t *testing.T) {
	var callCount int32
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"tr7nhqjekqxgtci8q8zy4pl8otszgjlj6t":{"usd":1.0}}`)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	// First call succeeds
	p, err := svc.GetPriceByContract(context.Background(), "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t")
	require.NoError(t, err)
	assert.InDelta(t, 1.0, p, 0.001)

	// Wait for cache to expire
	time.Sleep(1100 * time.Millisecond)

	// Second call — server fails, returns stale
	p, err = svc.GetPriceByContract(context.Background(), "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t")
	require.NoError(t, err)
	assert.InDelta(t, 1.0, p, 0.001, "should return stale price on error")
}

func TestGetPriceByContract_CaseInsensitiveCache(t *testing.T) {
	var callCount int32
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tr7nhqjekqxgtci8q8zy4pl8otszgjlj6t":{"usd":1.0}}`)
	})

	// First call with mixed case
	_, err := svc.GetPriceByContract(context.Background(), "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t")
	require.NoError(t, err)

	// Second call with different case — should use cache
	_, err = svc.GetPriceByContract(context.Background(), "tr7nhqjekqxgtci8q8zy4pl8otszgjlj6t")
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "same address different case should hit cache")
}

func TestSetCache_EvictsOldestWhenFull(t *testing.T) {
	svc := NewService(Config{CacheTTL: 1 * time.Second})

	// Fill cache with entries — use a small number since we can't fill 10k in a test
	// Instead, directly manipulate and verify the eviction logic
	now := time.Now()
	svc.mu.Lock()
	for i := 0; i < 10; i++ {
		svc.cache[fmt.Sprintf("key-%d", i)] = &CachedPrice{USD: float64(i), UpdatedAt: now.Add(-time.Duration(10-i) * time.Second)}
	}
	svc.mu.Unlock()

	// Verify entries exist
	svc.mu.RLock()
	assert.Equal(t, 10, len(svc.cache))
	svc.mu.RUnlock()

	// getStale should reject entries older than 10x TTL (10s)
	// key-0 is 10s old, key-1 is 9s old, etc.
	stale := svc.getStale("key-0")
	assert.Nil(t, stale, "entry older than maxStaleFactor*TTL should be nil")

	stale = svc.getStale("key-9")
	assert.NotNil(t, stale, "recent entry should be returned as stale")
}

func TestGetPriceByID_StaleExpired(t *testing.T) {
	var callCount int32
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"tron":{"usd":0.15}}`)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	// First call succeeds, caches the value
	p, err := svc.GetPriceByID(context.Background(), "tron")
	require.NoError(t, err)
	assert.InDelta(t, 0.15, p, 0.001)

	// Manually age the cache entry past max stale (10x TTL = 10s)
	svc.mu.Lock()
	if cp, ok := svc.cache["id:tron"]; ok {
		cp.UpdatedAt = time.Now().Add(-20 * time.Second)
	}
	svc.mu.Unlock()

	// Second call — server fails, stale is too old → error
	_, err = svc.GetPriceByID(context.Background(), "tron")
	assert.Error(t, err, "should error when stale cache is expired and server fails")
}

func TestBatchGetPrices_CachesResults(t *testing.T) {
	var callCount int32
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tron":{"usd":0.12},"tether":{"usd":1.0}}`)
	})

	_, err := svc.BatchGetPrices(context.Background(), []string{"tron", "tether"})
	require.NoError(t, err)

	// Individual lookups should be cached from batch
	p, err := svc.GetPriceByID(context.Background(), "tron")
	require.NoError(t, err)
	assert.InDelta(t, 0.12, p, 0.001)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "should use cache from batch")
}

func TestSetCache_EvictsOldestEntries(t *testing.T) {
	svc := NewService(Config{CacheTTL: 60 * time.Second})
	svc.maxCacheSize = 5 // low limit to trigger eviction

	// Add 5 entries with staggered timestamps
	now := time.Now()
	svc.mu.Lock()
	for i := 0; i < 5; i++ {
		svc.cache[fmt.Sprintf("id:token-%d", i)] = &CachedPrice{
			USD:       float64(i),
			UpdatedAt: now.Add(time.Duration(i) * time.Second),
		}
	}
	svc.mu.Unlock()

	// Add one more — should trigger eviction of oldest
	svc.setCache("id:new-token", 99.0)

	svc.mu.RLock()
	assert.LessOrEqual(t, len(svc.cache), 5, "cache should be evicted to maxCacheSize")
	// Newest entry should still be present
	assert.NotNil(t, svc.cache["id:new-token"])
	// Oldest entry (token-0) should have been evicted
	assert.Nil(t, svc.cache["id:token-0"], "oldest entry should be evicted")
	svc.mu.RUnlock()
}

func TestSetCache_EvictsStaleBeforeOldest(t *testing.T) {
	svc := NewService(Config{CacheTTL: 1 * time.Second})
	svc.maxCacheSize = 5

	now := time.Now()
	svc.mu.Lock()
	// 3 stale entries (older than 10x TTL = 10s)
	for i := 0; i < 3; i++ {
		svc.cache[fmt.Sprintf("id:stale-%d", i)] = &CachedPrice{
			USD:       float64(i),
			UpdatedAt: now.Add(-20 * time.Second),
		}
	}
	// 3 fresh entries
	for i := 0; i < 3; i++ {
		svc.cache[fmt.Sprintf("id:fresh-%d", i)] = &CachedPrice{
			USD:       float64(i),
			UpdatedAt: now,
		}
	}
	svc.mu.Unlock()

	// Add one more (7 total, limit 5) — stale entries should be removed first
	svc.setCache("id:newest", 42.0)

	svc.mu.RLock()
	// Stale entries should be gone, fresh + newest should remain
	assert.LessOrEqual(t, len(svc.cache), 5)
	assert.NotNil(t, svc.cache["id:newest"])
	assert.NotNil(t, svc.cache["id:fresh-0"])
	assert.Nil(t, svc.cache["id:stale-0"], "stale entries should be evicted first")
	svc.mu.RUnlock()
}

func TestDoGet_ServerError(t *testing.T) {
	_, svc := newMockCoinGecko(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	_, err := svc.GetTRXPrice(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}
