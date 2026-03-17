package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func validateAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("address is required")
	}
	a, err := address.Base58ToAddress(addr)
	if err != nil {
		return fmt.Errorf("invalid TRON address %q: %v", addr, err)
	}
	if !a.IsValid() {
		return fmt.Errorf("invalid TRON address: must be base58 format starting with T")
	}
	return nil
}

// RegisterAddressTools registers address validation and conversion tools.
func RegisterAddressTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("validate_address",
			mcp.WithDescription("Validate and convert a TRON address between base58 and hex formats. Accepts both base58 (T...) and hex (41...) input."),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON address (base58 starting with T, or hex starting with 41)")),
		),
		handleValidateAddress(),
	)
}

func handleValidateAddress() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		if addr == "" {
			return mcp.NewToolResultError("address is required"), nil
		}

		var a address.Address
		var format string

		// Detect input format
		if strings.HasPrefix(addr, "T") {
			parsed, err := address.Base58ToAddress(addr)
			if err != nil {
				return mcp.NewToolResultJSON(map[string]any{
					"input":    addr,
					"is_valid": false,
					"error":    fmt.Sprintf("invalid base58 address: %v", err),
				})
			}
			a = parsed
			format = "base58"
		} else if strings.HasPrefix(addr, "41") || strings.HasPrefix(addr, "0x41") {
			clean := strings.TrimPrefix(addr, "0x")
			a = address.HexToAddress(clean)
			format = "hex"
		} else {
			a = address.HexToAddress(addr)
			format = "hex"
		}

		result := map[string]any{
			"input":        addr,
			"input_format": format,
			"is_valid":     a.IsValid(),
			"hex":          a.Hex(),
			"base58":       a.String(),
		}

		return mcp.NewToolResultJSON(result)
	}
}
