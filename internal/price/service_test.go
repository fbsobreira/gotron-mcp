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
