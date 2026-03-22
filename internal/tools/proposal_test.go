package tools

import (
	"context"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

func TestListProposals_Success(t *testing.T) {
	mock := &mockWalletServer{
		ListProposalsFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.ProposalList, error) {
			return &api.ProposalList{
				Proposals: []*core.Proposal{
					{
						ProposalId: 1,
						Approvals:  [][]byte{{0x41, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14}},
					},
				},
			}, nil
		},
	}
	pool := newMockPool(t, mock)
	result := callTool(t, handleListProposals(pool), map[string]any{})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}

// makeProposals creates n Proposal protos with distinct IDs.
func makeProposals(n int) []*core.Proposal {
	ps := make([]*core.Proposal, n)
	for i := range n {
		addr := make([]byte, 21)
		addr[0] = 0x41
		addr[20] = byte(i + 1)
		ps[i] = &core.Proposal{
			ProposalId:      int64(i + 1),
			ProposerAddress: addr,
			Parameters:      map[int64]int64{0: int64(i)},
			ExpirationTime:  1700000000 + int64(i*86400),
			CreateTime:      1700000000,
			State:           core.Proposal_PENDING,
		}
	}
	return ps
}

func mockProposalServer(proposals []*core.Proposal) *mockWalletServer {
	return &mockWalletServer{
		ListProposalsFunc: func(_ context.Context, _ *api.EmptyMessage) (*api.ProposalList, error) {
			return &api.ProposalList{Proposals: proposals}, nil
		},
	}
}

func TestListProposals(t *testing.T) {
	proposals := makeProposals(15)

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
			wantReturned: 5,
			wantTotal:    15,
			wantHasMore:  true,
			wantNextOff:  5,
		},
		{
			name:         "custom limit",
			args:         map[string]any{"limit": float64(5)},
			wantReturned: 5,
			wantTotal:    15,
			wantHasMore:  true,
			wantNextOff:  5,
		},
		{
			name:         "offset and limit",
			args:         map[string]any{"limit": float64(10), "offset": float64(10)},
			wantReturned: 5,
			wantTotal:    15,
			wantHasMore:  false,
		},
		{
			name:         "offset beyond total",
			args:         map[string]any{"offset": float64(100)},
			wantReturned: 0,
			wantTotal:    15,
			wantHasMore:  false,
		},
		{
			name:         "exact boundary",
			args:         map[string]any{"limit": float64(15)},
			wantReturned: 15,
			wantTotal:    15,
			wantHasMore:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := newMockPool(t, mockProposalServer(proposals))
			result := callTool(t, handleListProposals(pool), tt.args)
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

			ps := data["proposals"].([]any)
			if len(ps) != tt.wantReturned {
				t.Errorf("proposals length = %d, want %d", len(ps), tt.wantReturned)
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

func TestListProposals_InvalidOrder(t *testing.T) {
	pool := newMockPool(t, mockProposalServer(makeProposals(3)))
	result := callTool(t, handleListProposals(pool), map[string]any{"order": "invalid"})
	if !result.IsError {
		t.Fatal("expected error for invalid order, got success")
	}
}

func TestListProposals_NegativeParams(t *testing.T) {
	proposals := makeProposals(5)
	pool := newMockPool(t, mockProposalServer(proposals))
	result := callTool(t, handleListProposals(pool), map[string]any{"limit": float64(-1), "offset": float64(-5)})
	if result.IsError {
		t.Fatalf("expected success with clamped values, got error: %v", result.Content)
	}
	data := parseJSONResult(t, result)
	if got := int(data["total"].(float64)); got != 5 {
		t.Errorf("total = %d, want 5", got)
	}
	// Negative limit should clamp to default 10, negative offset to 0
	if got := int(data["returned"].(float64)); got != 5 {
		t.Errorf("returned = %d, want 5", got)
	}
}

func TestListProposals_Empty(t *testing.T) {
	pool := newMockPool(t, mockProposalServer(nil))
	result := callTool(t, handleListProposals(pool), map[string]any{})
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

func TestListProposals_OrderDesc(t *testing.T) {
	proposals := makeProposals(5) // IDs 1..5
	pool := newMockPool(t, mockProposalServer(proposals))
	result := callTool(t, handleListProposals(pool), map[string]any{"limit": float64(5)})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	ps := data["proposals"].([]any)
	// Default order is desc — first item should have highest proposal_id
	firstID := int(ps[0].(map[string]any)["proposal_id"].(float64))
	lastID := int(ps[len(ps)-1].(map[string]any)["proposal_id"].(float64))
	if firstID <= lastID {
		t.Errorf("expected descending order, got first_id=%d last_id=%d", firstID, lastID)
	}
}

func TestListProposals_OrderAsc(t *testing.T) {
	proposals := makeProposals(5) // IDs 1..5
	pool := newMockPool(t, mockProposalServer(proposals))
	result := callTool(t, handleListProposals(pool), map[string]any{"limit": float64(5), "order": "asc"})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	ps := data["proposals"].([]any)
	firstID := int(ps[0].(map[string]any)["proposal_id"].(float64))
	lastID := int(ps[len(ps)-1].(map[string]any)["proposal_id"].(float64))
	if firstID >= lastID {
		t.Errorf("expected ascending order, got first_id=%d last_id=%d", firstID, lastID)
	}
}

// Verify proposal fields are correctly mapped.
func TestListProposals_Fields(t *testing.T) {
	addr := make([]byte, 21)
	addr[0] = 0x41
	addr[20] = 0x01

	proposals := []*core.Proposal{
		{
			ProposalId:      42,
			ProposerAddress: addr,
			Parameters:      map[int64]int64{1: 100},
			ExpirationTime:  1700086400,
			CreateTime:      1700000000,
			State:           core.Proposal_APPROVED,
		},
	}

	pool := newMockPool(t, mockProposalServer(proposals))
	result := callTool(t, handleListProposals(pool), map[string]any{"limit": float64(10)})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	data := parseJSONResult(t, result)
	ps := data["proposals"].([]any)
	if len(ps) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(ps))
	}

	p := ps[0].(map[string]any)
	if got := int(p["proposal_id"].(float64)); got != 42 {
		t.Errorf("proposal_id = %d, want 42", got)
	}
	if got := p["state"].(string); got != "APPROVED" {
		t.Errorf("state = %s, want APPROVED", got)
	}
	if _, ok := p["expiration_time"]; !ok {
		t.Error("expiration_time field missing")
	}
}
