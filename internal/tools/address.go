package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/txbuilder"
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
			mcp.WithDescription("Validate a TRON address and convert supported inputs to TRON base58/hex. Accepts base58 (T...), TRON hex (41...), or Ethereum/EVM format (0x...)."),
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
			rawBytes, err := hex.DecodeString(addr)
			if err != nil {
				return mcp.NewToolResultJSON(map[string]any{
					"input":    addr,
					"is_valid": false,
					"error":    fmt.Sprintf("invalid hex: %v", err),
				})
			}
			if len(rawBytes) != 21 {
				return mcp.NewToolResultJSON(map[string]any{
					"input":    addr,
					"is_valid": false,
					"error":    "invalid TRON hex address length: expected 21 bytes",
				})
			}
			a = address.BytesToAddress(rawBytes)
			format = "hex"
		default:
			return mcp.NewToolResultJSON(map[string]any{
				"input":    addr,
				"is_valid": false,
				"error":    "unsupported address format: expected T..., 41..., or 0x...",
			})
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

// builderOptions extracts common txbuilder options (memo, permission_id) from
// the MCP request. Used by all write tools that use the txbuilder API.
func builderOptions(req mcp.CallToolRequest) []txbuilder.Option {
	var opts []txbuilder.Option
	if memo := req.GetString("memo", ""); memo != "" {
		opts = append(opts, txbuilder.WithMemo(memo))
	}
	args := req.GetArguments()
	if _, has := args["permission_id"]; has {
		pid := req.GetInt("permission_id", 0)
		if pid >= 0 && pid <= math.MaxInt32 {
			opts = append(opts, txbuilder.WithPermissionID(int32(pid)))
		} else {
			log.Printf("warning: permission_id %d out of range (0-%d), ignoring", pid, math.MaxInt32)
		}
	}
	return opts
}
