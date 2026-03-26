package price

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultCoinGeckoURL = "https://api.coingecko.com/api/v3"
	defaultCacheTTL     = 60 * time.Second
	maxResponseBody     = 1 << 20 // 1 MB
	maxCacheEntries     = 10000   // prevent unbounded cache growth
	maxStaleFactor      = 10      // stale entries expire at 10x TTL
	tronPlatformID      = "tron"
	trxCoinGeckoID      = "tron"
)

// CachedPrice holds a USD price with its fetch time.
type CachedPrice struct {
	USD       float64
	UpdatedAt time.Time
}

// Service provides token USD prices via CoinGecko with in-memory caching.
type Service struct {
	baseURL      string
	apiKey       string // optional CoinGecko Pro API key
	cacheTTL     time.Duration
	maxCacheSize int // max cache entries before eviction (default: maxCacheEntries)
	client       *http.Client

	mu    sync.RWMutex
	cache map[string]*CachedPrice // key: "TRX" or contract address → price
}

// Config holds configuration for the price service.
type Config struct {
	BaseURL  string        // override CoinGecko base URL (for testing)
	APIKey   string        // optional CoinGecko Pro API key
	CacheTTL time.Duration // cache duration (default 60s)
}

// NewService creates a price service with CoinGecko backend.
func NewService(cfg Config) *Service {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultCoinGeckoURL
	}
	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	return &Service{
		baseURL:      baseURL,
		apiKey:       cfg.APIKey,
		cacheTTL:     cacheTTL,
		maxCacheSize: maxCacheEntries,
		client:       &http.Client{Timeout: 10 * time.Second},
		cache:        make(map[string]*CachedPrice),
	}
}

// GetTRXPrice returns the current USD price of TRX.
func (s *Service) GetTRXPrice(ctx context.Context) (float64, error) {
	return s.GetPriceByID(ctx, trxCoinGeckoID)
}

// GetPriceByID returns the USD price for a CoinGecko coin ID (e.g., "tron", "tether").
func (s *Service) GetPriceByID(ctx context.Context, coinID string) (float64, error) {
	cacheKey := "id:" + coinID

	// Check cache
	if cached := s.getCached(cacheKey); cached != nil {
		return cached.USD, nil
	}

	// Fetch from CoinGecko
	path := fmt.Sprintf("/simple/price?ids=%s&vs_currencies=usd", url.QueryEscape(coinID))
	body, err := s.doGet(ctx, path)
	if err != nil {
		// Stale-while-revalidate: return stale cache on error
		if stale := s.getStale(cacheKey); stale != nil {
			log.Printf("price: using stale price for %s (fetch error: %v)", coinID, err)
			return stale.USD, nil
		}
		return 0, fmt.Errorf("fetching price for %s: %w", coinID, err)
	}

	// Parse: {"tron": {"usd": 0.123}}
	var result map[string]map[string]float64
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("parsing price response: %w", err)
	}

	coinData, ok := result[coinID]
	if !ok {
		return 0, fmt.Errorf("no price data for %s", coinID)
	}
	usdPrice, ok := coinData["usd"]
	if !ok {
		return 0, fmt.Errorf("no USD price for %s", coinID)
	}

	s.setCache(cacheKey, usdPrice)
	return usdPrice, nil
}

// GetPriceByContract returns the USD price for a TRC20 token by its contract address.
func (s *Service) GetPriceByContract(ctx context.Context, contractAddr string) (float64, error) {
	cacheKey := "contract:" + strings.ToLower(contractAddr)

	if cached := s.getCached(cacheKey); cached != nil {
		return cached.USD, nil
	}

	path := fmt.Sprintf("/simple/token_price/%s?contract_addresses=%s&vs_currencies=usd",
		tronPlatformID, url.QueryEscape(contractAddr))
	body, err := s.doGet(ctx, path)
	if err != nil {
		if stale := s.getStale(cacheKey); stale != nil {
			log.Printf("price: using stale price for contract %s (fetch error: %v)", contractAddr, err)
			return stale.USD, nil
		}
		return 0, fmt.Errorf("fetching price for contract %s: %w", contractAddr, err)
	}

	// Parse: {"TR7N...": {"usd": 1.0}}
	var result map[string]map[string]float64
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("parsing contract price response: %w", err)
	}

	// CoinGecko lowercases contract addresses in response
	lowerAddr := strings.ToLower(contractAddr)
	for key, data := range result {
		if strings.ToLower(key) == lowerAddr {
			if usd, ok := data["usd"]; ok {
				s.setCache(cacheKey, usd)
				return usd, nil
			}
		}
	}

	return 0, fmt.Errorf("no price data for contract %s", contractAddr)
}

// GetTokenPrice returns the USD price for a token.
// Accepts "TRX" or a TRC20 contract address.
func (s *Service) GetTokenPrice(ctx context.Context, tokenID string) (float64, error) {
	if strings.ToUpper(tokenID) == "TRX" {
		return s.GetTRXPrice(ctx)
	}
	return s.GetPriceByContract(ctx, tokenID)
}

// BatchGetPrices fetches prices for multiple CoinGecko IDs in one call.
// Falls back to stale cached values on fetch error, consistent with single-token methods.
func (s *Service) BatchGetPrices(ctx context.Context, coinIDs []string) (map[string]float64, error) {
	ids := strings.Join(coinIDs, ",")
	path := fmt.Sprintf("/simple/price?ids=%s&vs_currencies=usd", url.QueryEscape(ids))
	body, fetchErr := s.doGet(ctx, path)
	if fetchErr != nil {
		return s.batchStale(coinIDs, fetchErr)
	}

	var result map[string]map[string]float64
	if err := json.Unmarshal(body, &result); err != nil {
		return s.batchStale(coinIDs, fmt.Errorf("parsing batch price response: %w", err))
	}

	prices := make(map[string]float64, len(result))
	for id, data := range result {
		if usd, ok := data["usd"]; ok {
			prices[id] = usd
			s.setCache("id:"+id, usd)
		}
	}
	return prices, nil
}

// batchStale returns stale cached prices for the requested IDs on fetch error.
// Returns the original error only if no stale values are available.
func (s *Service) batchStale(coinIDs []string, fetchErr error) (map[string]float64, error) {
	prices := make(map[string]float64)
	for _, id := range coinIDs {
		if stale := s.getStale("id:" + id); stale != nil {
			prices[id] = stale.USD
		}
	}
	if len(prices) > 0 {
		log.Printf("price: using stale prices for batch fetch (%d/%d available, error: %v)", len(prices), len(coinIDs), fetchErr)
		return prices, nil
	}
	return nil, fmt.Errorf("batch price fetch: %w", fetchErr)
}

// --- Cache ---

func (s *Service) getCached(key string) *CachedPrice {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.cache[key]; ok && time.Since(p.UpdatedAt) < s.cacheTTL {
		return p
	}
	return nil
}

func (s *Service) getStale(key string) *CachedPrice {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.cache[key]; ok && time.Since(p.UpdatedAt) < s.cacheTTL*maxStaleFactor {
		return p
	}
	return nil
}

func (s *Service) setCache(key string, usd float64) {
	s.mu.Lock()
	s.cache[key] = &CachedPrice{USD: usd, UpdatedAt: time.Now()}
	if len(s.cache) > s.maxCacheSize {
		// First pass: remove entries beyond max stale age
		maxStale := s.cacheTTL * maxStaleFactor
		now := time.Now()
		for k, p := range s.cache {
			if now.Sub(p.UpdatedAt) > maxStale {
				delete(s.cache, k)
			}
		}
		// Second pass: if still over limit, evict oldest entries
		if len(s.cache) > s.maxCacheSize {
			type entry struct {
				key string
				ts  time.Time
			}
			entries := make([]entry, 0, len(s.cache))
			for k, p := range s.cache {
				entries = append(entries, entry{k, p.UpdatedAt})
			}
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].ts.Before(entries[j].ts)
			})
			toRemove := len(s.cache) - s.maxCacheSize
			for i := 0; i < toRemove; i++ {
				delete(s.cache, entries[i].key)
			}
		}
	}
	s.mu.Unlock()
}

// --- HTTP ---

func (s *Service) doGet(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if s.apiKey != "" {
		req.Header.Set("x-cg-pro-api-key", s.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("CoinGecko rate limited (429)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CoinGecko returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return body, nil
}
