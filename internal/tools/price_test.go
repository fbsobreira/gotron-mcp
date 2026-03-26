package tools

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/price"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockPriceService(t *testing.T, handler http.HandlerFunc) *price.Service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return price.NewService(price.Config{
		BaseURL:  srv.URL,
		CacheTTL: 1 * time.Second,
	})
}

func priceResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var parts []string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func TestHandleGetTokenPrice_TRX(t *testing.T) {
	svc := newMockPriceService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tron":{"usd":0.123}}`)
	})

	handler := handleGetTokenPrice(svc)
	result := callTool(t, handler, map[string]any{"token": "TRX"})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	text := priceResultText(t, result)
	assert.Contains(t, text, "0.123")
	assert.Contains(t, text, "USD")
}

func TestHandleGetTokenPrice_Contract(t *testing.T) {
	svc := newMockPriceService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tr7nhqjekqxgtci8q8zy4pl8otszgjlj6t":{"usd":1.0001}}`)
	})

	handler := handleGetTokenPrice(svc)
	result := callTool(t, handler, map[string]any{"token": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"})
	require.False(t, result.IsError, "expected success, got error: %v", result.Content)

	text := priceResultText(t, result)
	assert.Contains(t, text, "1.0001")
}

func TestHandleGetTokenPrice_EmptyToken(t *testing.T) {
	svc := newMockPriceService(t, func(w http.ResponseWriter, r *http.Request) {})

	handler := handleGetTokenPrice(svc)
	result := callTool(t, handler, map[string]any{"token": ""})
	assert.True(t, result.IsError)

	text := priceResultText(t, result)
	assert.Contains(t, text, "token is required")
}

func TestHandleGetTokenPrice_MissingToken(t *testing.T) {
	svc := newMockPriceService(t, func(w http.ResponseWriter, r *http.Request) {})

	handler := handleGetTokenPrice(svc)
	result := callTool(t, handler, map[string]any{})
	assert.True(t, result.IsError)

	text := priceResultText(t, result)
	assert.Contains(t, text, "token is required")
}

func TestHandleGetTokenPrice_APIError(t *testing.T) {
	svc := newMockPriceService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	handler := handleGetTokenPrice(svc)
	result := callTool(t, handler, map[string]any{"token": "TRX"})
	assert.True(t, result.IsError)

	text := priceResultText(t, result)
	assert.Contains(t, text, "get_token_price")
}

func TestHandleGetTokenPrice_NotFound(t *testing.T) {
	svc := newMockPriceService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{}`)
	})

	handler := handleGetTokenPrice(svc)
	result := callTool(t, handler, map[string]any{"token": "TRX"})
	assert.True(t, result.IsError)

	text := priceResultText(t, result)
	assert.Contains(t, text, "no price data")
}

func TestRegisterPriceTools_NilService(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.0")
	RegisterPriceTools(s, nil) // should not panic
}

func TestRegisterPriceTools_WithService(t *testing.T) {
	svc := newMockPriceService(t, func(w http.ResponseWriter, r *http.Request) {})
	s := server.NewMCPServer("test", "0.0.0")
	RegisterPriceTools(s, svc) // should register without error
}
