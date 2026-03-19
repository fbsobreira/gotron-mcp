package trongrid

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var trongridURLs = map[string]string{
	"mainnet": "https://api.trongrid.io",
	"nile":    "https://nile.trongrid.io",
	"shasta":  "https://api.shasta.trongrid.io",
}

// Client is a TronGrid REST API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a TronGrid client for the given network.
// If the network is unknown, it defaults to mainnet.
func NewClient(network, apiKey string) *Client {
	baseURL, ok := trongridURLs[network]
	if !ok {
		baseURL = trongridURLs["mainnet"]
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest performs a GET request against the TronGrid API and returns the raw body.
func (c *Client) doRequest(ctx context.Context, path string, params url.Values) ([]byte, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	const maxResponseSize = 10 << 20 // 10 MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited by TronGrid (HTTP 429): %s", body)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("TronGrid server error (HTTP %d): %s", resp.StatusCode, body)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("TronGrid request failed (HTTP %d): %s", resp.StatusCode, body)
	}

	return body, nil
}

// buildParams converts QueryOpts to url.Values.
func buildParams(opts QueryOpts) url.Values {
	params := url.Values{}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Fingerprint != "" {
		params.Set("fingerprint", opts.Fingerprint)
	}
	if opts.OnlyConfirmed {
		params.Set("only_confirmed", "true")
	}
	if opts.OnlyTo {
		params.Set("only_to", "true")
	}
	if opts.OnlyFrom {
		params.Set("only_from", "true")
	}
	if opts.MinTimestamp > 0 {
		params.Set("min_timestamp", strconv.FormatInt(opts.MinTimestamp, 10))
	}
	if opts.MaxTimestamp > 0 {
		params.Set("max_timestamp", strconv.FormatInt(opts.MaxTimestamp, 10))
	}
	if opts.OrderBy != "" {
		params.Set("order_by", opts.OrderBy)
	}
	return params
}
