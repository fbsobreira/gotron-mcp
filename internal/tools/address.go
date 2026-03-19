package tools

import (
	"context"
	"encoding/hex"
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
			mcp.WithDescription("Validate and convert a TRON address. Accepts base58 (T...), hex (41...), or Ethereum/EVM format (0x...). Converts between all formats."),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON address (base58 T..., hex 41..., or Ethereum 0x...)")),
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
		switch {
		case strings.HasPrefix(addr, "T"):
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
		case strings.HasPrefix(addr, "0x") || strings.HasPrefix(addr, "0X"):
			// 0x-prefixed: disambiguate by decoded length
			// 20 bytes (40 hex chars) = Ethereum address
			// 21 bytes (42 hex chars) = TRON hex address (41-prefixed)
			hexStr := stripHexPrefix(addr)
			rawBytes, err := hex.DecodeString(hexStr)
			if err != nil {
				return mcp.NewToolResultJSON(map[string]any{
					"input":    addr,
					"is_valid": false,
					"error":    fmt.Sprintf("invalid hex: %v", err),
				})
			}
			if len(rawBytes) == 20 {
				// 20-byte Ethereum address
				converted, err := address.EthAddressToAddress(rawBytes)
				if err != nil {
					return mcp.NewToolResultJSON(map[string]any{
						"input":    addr,
						"is_valid": false,
						"error":    fmt.Sprintf("invalid Ethereum address: %v", err),
					})
				}
				a = converted
				format = "ethereum"
			} else if len(rawBytes) == 21 {
				// 21-byte TRON hex address (41-prefixed)
				a = address.BytesToAddress(rawBytes)
				format = "hex"
			} else {
				return mcp.NewToolResultJSON(map[string]any{
					"input":    addr,
					"is_valid": false,
					"error":    "invalid 0x address length: expected 20 (Ethereum) or 21 (TRON) bytes",
				})
			}
		case strings.HasPrefix(addr, "41"):
			a = address.HexToAddress(addr)
			format = "hex"
		default:
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
