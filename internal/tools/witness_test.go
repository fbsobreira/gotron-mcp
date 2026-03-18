package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestListWitnesses_Success(t *testing.T) {
	mock := &mockWalletServer{
		ListWitnessesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.WitnessList, error) {
			return &api.WitnessList{
				Witnesses: []*core.Witness{
					{
						Address:       []byte{0x41, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14},
						VoteCount:     1000,
						TotalProduced: 500,
						TotalMissed:   10,
						IsJobs:        true,
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleListWitnesses(pool), map[string]any{})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}

// makeWitnesses creates n Witness protos with distinct addresses.
func makeWitnesses(n int) []*core.Witness {
	ws := make([]*core.Witness, n)
	for i := range n {
		// 21-byte address: 0x41 prefix + 20 bytes
		addr := make([]byte, 21)
		addr[0] = 0x41
		addr[20] = byte(i + 1)
		ws[i] = &core.Witness{
			Address:        addr,
			VoteCount:      int64(1000 - i),
			Url:            fmt.Sprintf("https://sr%d.example.com", i),
			TotalProduced:  int64(100 + i),
			TotalMissed:    int64(i),
			LatestBlockNum: int64(50000 + i),
			IsJobs:         i%2 == 0,
		}
	}
	return ws
}

func mockWitnessServer(witnesses []*core.Witness) *mockWalletServer {
	return &mockWalletServer{
		ListWitnessesFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.WitnessList, error) {
			return &api.WitnessList{Witnesses: witnesses}, nil
		},
	}
}

func TestListWitnesses(t *testing.T) {
	witnesses := makeWitnesses(25)

	tests := []struct {
		name         string
		args         map[string]any
		wantReturned int
		wantTotal    int
		wantHasMore  bool
		wantNextOff  int
	}{
		{
			name:         "default pagination",
			args:         map[string]any{},
			wantReturned: 10,
			wantTotal:    25,
			wantHasMore:  true,
			wantNextOff:  10,
		},
		{
			name:         "custom limit",
			args:         map[string]any{"limit": float64(5)},
			wantReturned: 5,
			wantTotal:    25,
			wantHasMore:  true,
			wantNextOff:  5,
		},
		{
			name:         "offset and limit",
			args:         map[string]any{"limit": float64(10), "offset": float64(20)},
			wantReturned: 5,
			wantTotal:    25,
			wantHasMore:  false,
		},
		{
			name:         "offset beyond total",
			args:         map[string]any{"offset": float64(100)},
			wantReturned: 0,
			wantTotal:    25,
			wantHasMore:  false,
		},
		{
			name:         "exact boundary",
			args:         map[string]any{"limit": float64(25)},
			wantReturned: 25,
			wantTotal:    25,
			wantHasMore:  false,
		},
		{
			name:         "second page",
			args:         map[string]any{"limit": float64(10), "offset": float64(10)},
			wantReturned: 10,
			wantTotal:    25,
			wantHasMore:  true,
			wantNextOff:  20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := newMockPool(t, mockWitnessServer(witnesses))
			result := callTool(t, handleListWitnesses(pool), tt.args)
			if result.IsError {
				t.Fatalf("expected success, got error: %v", result.Content)
			}

			data := parseJSONResult(t, result)

			if got := int(data["total"].(float64)); got != tt.wantTotal {
				t.Errorf("total = %d, want %d", got, tt.wantTotal)
			}
			if got := int(data["returned"].(float64)); got != tt.wantReturned {
				t.Errorf("returned = %d, want %d", got, tt.wantReturned)
			}

			ws := data["witnesses"].([]any)
			if len(ws) != tt.wantReturned {
				t.Errorf("witnesses length = %d, want %d", len(ws), tt.wantReturned)
			}

			hasMore, ok := data["has_more"]
			if tt.wantHasMore {
				if !ok || hasMore != true {
					t.Errorf("has_more = %v, want true", hasMore)
				}
				if got := int(data["next_offset"].(float64)); got != tt.wantNextOff {
					t.Errorf("next_offset = %d, want %d", got, tt.wantNextOff)
				}
			} else if ok {
				t.Errorf("has_more should not be present, got %v", hasMore)
			}
		})
	}
}

func TestListWitnesses_NegativeParams(t *testing.T) {
	witnesses := makeWitnesses(5)
	pool := newMockPool(t, mockWitnessServer(witnesses))
	result := callTool(t, handleListWitnesses(pool), map[string]any{"limit": float64(-1), "offset": float64(-5)})
	if result.IsError {
		t.Fatalf("expected success with clamped values, got error: %v", result.Content)
	}
	data := parseJSONResult(t, result)
	if got := int(data["total"].(float64)); got != 5 {
		t.Errorf("total = %d, want 5", got)
	}
	if got := int(data["returned"].(float64)); got != 5 {
		t.Errorf("returned = %d, want 5", got)
	}
}

func TestListWitnesses_Empty(t *testing.T) {
	pool := newMockPool(t, mockWitnessServer(nil))
	result := callTool(t, handleListWitnesses(pool), map[string]any{})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	if got := int(data["total"].(float64)); got != 0 {
		t.Errorf("total = %d, want 0", got)
	}
	if got := int(data["returned"].(float64)); got != 0 {
		t.Errorf("returned = %d, want 0", got)
	}
}

// parseJSONResult extracts the JSON map from a tool result.
func parseJSONResult(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("empty result content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(text.Text), &data); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	return data
}
