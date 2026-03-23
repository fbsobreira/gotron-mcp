package health

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

type mockWalletServer struct {
	api.UnimplementedWalletServer
	GetNowBlock2Func func(context.Context, *api.EmptyMessage) (*api.BlockExtention, error)
}

func (m *mockWalletServer) GetNowBlock2(ctx context.Context, in *api.EmptyMessage) (*api.BlockExtention, error) {
	if m.GetNowBlock2Func != nil {
		return m.GetNowBlock2Func(ctx, in)
	}
	return m.UnimplementedWalletServer.GetNowBlock2(ctx, in)
}

func newMockPool(t *testing.T, mock *mockWalletServer) *nodepool.Pool {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	api.RegisterWalletServer(srv, mock)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.GracefulStop()
		_ = lis.Close()
	})

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err, "failed to create mock client")
	t.Cleanup(func() { _ = conn.Close() })

	c := client.NewGrpcClient("bufconn")
	c.Conn = conn
	c.Client = api.NewWalletClient(conn)
	return nodepool.NewFromClient(c, "mock:50051")
}

func TestNewHandler(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{})
	h := NewHandler(pool, "mainnet")
	require.NotNil(t, h, "NewHandler returned nil")
	assert.Equal(t, "mainnet", h.network)
}

func TestServeHTTP_Success(t *testing.T) {
	mock := &mockWalletServer{
		GetNowBlock2Func: func(_ context.Context, _ *api.EmptyMessage) (*api.BlockExtention, error) {
			return &api.BlockExtention{
				BlockHeader: &core.BlockHeader{
					RawData: &core.BlockHeaderRaw{
						Number:    12345,
						Timestamp: 1700000000,
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	h := NewHandler(pool, "mainnet")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "failed to parse response")
	assert.Equal(t, "ok", body["status"])
}

func TestServeHTTP_Degraded(t *testing.T) {
	mock := &mockWalletServer{
		// GetNowBlock2 not set — will return unimplemented error
	}
	pool := newMockPool(t, mock)
	h := NewHandler(pool, "mainnet")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "failed to parse response")
	assert.Equal(t, "degraded", body["status"])
}

func TestServeHTTP_CacheHit(t *testing.T) {
	calls := 0
	mock := &mockWalletServer{
		GetNowBlock2Func: func(_ context.Context, _ *api.EmptyMessage) (*api.BlockExtention, error) {
			calls++
			return &api.BlockExtention{
				BlockHeader: &core.BlockHeader{
					RawData: &core.BlockHeaderRaw{
						Number: int64(12345 + calls),
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	h := NewHandler(pool, "mainnet")

	// First request — cache miss
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, httptest.NewRequest("GET", "/health", nil))
	require.Equal(t, http.StatusOK, rec1.Code, "first request")

	// Second request — should be cache hit (same body)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest("GET", "/health", nil))
	require.Equal(t, http.StatusOK, rec2.Code, "second request")

	assert.Equal(t, rec1.Body.String(), rec2.Body.String(), "second request should return cached response")

	// First request calls GetNowBlock twice (CheckHealth + GetNowBlockCtx).
	// Second request should serve from cache with no additional calls.
	firstCallCount := calls
	assert.GreaterOrEqual(t, calls, 1, "expected at least 1 gRPC call on first request")
	// Third request — verify no new calls (still cached)
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, httptest.NewRequest("GET", "/health", nil))
	assert.Equal(t, firstCallCount, calls, "cache miss: gRPC calls should not increase")
}
