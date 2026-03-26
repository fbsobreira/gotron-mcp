package tools

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-sdk/pkg/contract"
	"github.com/fbsobreira/gotron-sdk/pkg/standards/trc20"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

// maxUint256 is the maximum value for a uint256, used to detect unlimited approvals.
var maxUint256 = func() *big.Int {
	n := new(big.Int)
	n.SetString("115792089237316195423570985008687907853269984665640564039457584007913129639935", 10)
	return n
}()

// unlimitedThreshold is 2^255 — any allowance above this is considered "unlimited".
var unlimitedThreshold = new(big.Int).Rsh(maxUint256, 1)

// RegisterApprovalReadTools registers get_trc20_allowance (read-only).
func RegisterApprovalReadTools(s *server.MCPServer, pool *nodepool.Pool, cache *trc20.MetadataCache) {
	s.AddTool(
		mcp.NewTool("get_trc20_allowance",
			mcp.WithDescription("Query how many tokens a spender is approved to transfer on behalf of the owner. Flags unlimited approvals as high risk."),
			mcp.WithString("owner", mcp.Required(), mcp.Description("Token owner address (base58)")),
			mcp.WithString("spender", mcp.Required(), mcp.Description("Approved spender address (base58)")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("TRC20 token contract address")),
		),
		handleGetTRC20Allowance(pool, cache),
	)
}

// RegisterApprovalWriteTools registers revoke_approval (transaction builder).
func RegisterApprovalWriteTools(s *server.MCPServer, pool *nodepool.Pool, cache *trc20.MetadataCache) {
	s.AddTool(
		mcp.NewTool("revoke_approval",
			mcp.WithDescription("Build unsigned transaction to revoke a TRC20 token approval (sets allowance to 0)"),
			mcp.WithString("owner", mcp.Required(), mcp.Description("Token owner address (base58) — the account revoking the approval")),
			mcp.WithString("spender", mcp.Required(), mcp.Description("Spender address to revoke")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("TRC20 token contract address")),
			mcp.WithNumber("fee_limit", mcp.Description("Fee limit in TRX (default 15, max 15000)")),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID for multi-sig transactions")),
		),
		handleRevokeApproval(pool, cache),
	)
}

func handleGetTRC20Allowance(pool *nodepool.Pool, cache *trc20.MetadataCache) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner := req.GetString("owner", "")
		spender := req.GetString("spender", "")
		contractAddr := req.GetString("contract_address", "")

		if err := validateAddress(owner); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid owner address: %v", err)), nil
		}
		if err := validateAddress(spender); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid spender address: %v", err)), nil
		}
		if err := validateAddress(contractAddr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		// Get allowance
		allowance, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*big.Int, error) {
			return trc20Token(pool.Client(), contractAddr, cache).Allowance(ctx, owner, spender)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_allowance: %v", err)), nil
		}

		// Get token decimals for display
		decimals, dErr := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (uint8, error) {
			return trc20Token(pool.Client(), contractAddr, cache).Decimals(ctx)
		})
		if dErr != nil {
			decimals = 0 // fallback — show raw value
		}

		isUnlimited := allowance.Cmp(unlimitedThreshold) >= 0
		var riskLevel string
		if isUnlimited {
			riskLevel = "high"
		} else if allowance.Sign() > 0 {
			riskLevel = "medium"
		} else {
			riskLevel = "none"
		}

		result := map[string]any{
			"owner":            owner,
			"spender":          spender,
			"contract_address": contractAddr,
			"allowance_raw":    allowance.String(),
			"is_unlimited":     isUnlimited,
			"risk_level":       riskLevel,
		}

		if isUnlimited {
			result["allowance_display"] = "Unlimited"
		} else if decimals > 0 {
			result["allowance_display"] = formatBigIntWithDecimals(allowance, int(decimals))
		} else {
			result["allowance_display"] = allowance.String()
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleRevokeApproval(pool *nodepool.Pool, cache *trc20.MetadataCache) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner := req.GetString("owner", "")
		spender := req.GetString("spender", "")
		contractAddr := req.GetString("contract_address", "")
		feeLimit := req.GetFloat("fee_limit", 15)

		if err := validateAddress(owner); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid owner address: %v", err)), nil
		}
		if err := validateAddress(spender); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid spender address: %v", err)), nil
		}
		if err := validateAddress(contractAddr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}
		if feeLimit < 0 || feeLimit > 15000 {
			return mcp.NewToolResultError("fee_limit must be between 0 and 15000 TRX"), nil
		}

		token := trc20Token(pool.Client(), contractAddr, cache)

		// approve(spender, 0) to revoke
		call := token.Approve(owner, spender, big.NewInt(0)).
			WithFeeLimit(int64(feeLimit * 1_000_000))

		if permID := req.GetFloat("permission_id", 0); permID > 0 {
			call = call.Apply(contract.WithPermissionID(int32(permID)))
		}

		tx, err := call.Build(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("revoke_approval: %v", err)), nil
		}

		txBytes, err := proto.Marshal(tx.Transaction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("revoke_approval: failed to serialize: %v", err)), nil
		}

		result := map[string]any{
			"type":             "RevokeApproval",
			"owner":            owner,
			"spender":          spender,
			"contract_address": contractAddr,
			"transaction_hex":  hex.EncodeToString(txBytes),
			"txid":             hex.EncodeToString(tx.Txid),
		}

		return mcp.NewToolResultJSON(result)
	}
}

// formatBigIntWithDecimals formats a big.Int token amount with the given decimals.
func formatBigIntWithDecimals(amount *big.Int, decimals int) string {
	if decimals == 0 {
		return amount.String()
	}
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int).Div(amount, divisor)
	frac := new(big.Int).Mod(amount, divisor)
	if frac.Sign() == 0 {
		return whole.String()
	}
	fracStr := fmt.Sprintf("%0*s", decimals, frac.String())
	// Trim trailing zeros
	for len(fracStr) > 0 && fracStr[len(fracStr)-1] == '0' {
		fracStr = fracStr[:len(fracStr)-1]
	}
	return fmt.Sprintf("%s.%s", whole, fracStr)
}
