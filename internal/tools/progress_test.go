package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestProgressReporter_NilToken(t *testing.T) {
	req := mcp.CallToolRequest{}
	p := newProgressReporter(context.Background(), req, 3)
	// Must not panic
	p.Send(1, "step one")
	p.Send(2, "step two")
	p.Send(3, "step three")
}

func TestProgressReporter_NilMeta(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Meta = nil
	p := newProgressReporter(context.Background(), req, 2)
	p.Send(1, "should not panic")
}

func TestProgressReporter_NilReceiver(t *testing.T) {
	var p *progressReporter
	// Must not panic on nil receiver
	p.Send(1, "noop")
}

func TestProgressReporter_NoServerInContext(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Meta = &mcp.Meta{ProgressToken: "test-token"}
	p := newProgressReporter(context.Background(), req, 5)
	// Server is nil (no server in context), must not panic
	p.Send(1, "validating")
	p.Send(2, "signing")
}
