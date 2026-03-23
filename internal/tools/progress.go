package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// progressReporter sends MCP progress notifications during long-running tool
// operations. It is a best-effort helper: if the client did not provide a
// progress token or the server is unavailable from context, all calls are
// silent no-ops.
type progressReporter struct {
	ctx   context.Context
	srv   *server.MCPServer
	token mcp.ProgressToken
	total int
}

// newProgressReporter creates a reporter from the current request context.
// The returned reporter is always safe to use — Send is a no-op when the
// client does not support progress notifications.
func newProgressReporter(ctx context.Context, req mcp.CallToolRequest, total int) *progressReporter {
	var token mcp.ProgressToken
	if req.Params.Meta != nil {
		token = req.Params.Meta.ProgressToken
	}
	return &progressReporter{
		ctx:   ctx,
		srv:   server.ServerFromContext(ctx),
		token: token,
		total: total,
	}
}

// Send emits a progress notification. Step is 1-based. Errors are swallowed
// because progress is informational — a failed notification must never fail
// the tool itself.
func (p *progressReporter) Send(step int, message string) {
	if p == nil || p.srv == nil || p.token == nil {
		return
	}
	_ = p.srv.SendNotificationToClient(
		p.ctx,
		"notifications/progress",
		map[string]any{
			"progressToken": p.token,
			"progress":      step,
			"total":         p.total,
			"message":       message,
		},
	)
}
