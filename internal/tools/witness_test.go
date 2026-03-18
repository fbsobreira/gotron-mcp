package tools

import (
	"context"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
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
