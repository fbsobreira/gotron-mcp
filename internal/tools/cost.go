package tools

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/fbsobreira/gotron-sdk/pkg/contract"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/standards/trc20"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterCostTools registers the analyze_transfer_cost tool (read-only).
func RegisterCostTools(s *server.MCPServer, pool *nodepool.Pool, cache *trc20.MetadataCache) {
	s.AddTool(
		mcp.NewTool("analyze_transfer_cost",
			mcp.WithDescription("Estimate the cost of a TRX or TRC20 transfer: energy, bandwidth, and TRX burn options"),
			mcp.WithString("from", mcp.Required(), mcp.Description("Sender TRON address (base58)")),
			mcp.WithString("to", mcp.Required(), mcp.Description("Recipient TRON address (base58)")),
			mcp.WithString("amount", mcp.Required(), mcp.Description("Amount to transfer (e.g., '100')")),
			mcp.WithString("contract_address", mcp.Description("TRC20 contract address (omit for TRX transfer)")),
		),
		handleAnalyzeTransferCost(pool, cache),
	)
}

func handleAnalyzeTransferCost(pool *nodepool.Pool, cache *trc20.MetadataCache) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		amountStr := req.GetString("amount", "")
		contractAddr := req.GetString("contract_address", "")

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}
		if err := validateAddress(to); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid to address: %v", err)), nil
		}

		isTRC20 := contractAddr != ""
		if isTRC20 {
			if err := validateAddress(contractAddr); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
			}
		}

		progress := newProgressReporter(ctx, req, 3)

		// 1. Get account resources
		progress.Send(1, "Fetching account resources...")
		res, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*api.AccountResourceMessage, error) {
			return pool.Client().GetAccountResourceCtx(ctx, from)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("analyze_transfer_cost: %v", err)), nil
		}

		// 2. Get current energy/bandwidth prices
		progress.Send(2, "Fetching current prices...")
		energyPrice := getCurrentPrice(ctx, pool.Client(), "energy")
		bandwidthPrice := getCurrentPrice(ctx, pool.Client(), "bandwidth")

		// 3. Estimate energy (TRC20 only)
		var energyRequired int64
		var transferType string
		var energyEstimated bool

		if isTRC20 {
			transferType = "TRC20"
			progress.Send(3, "Estimating energy for TRC20 transfer...")

			// Get decimals to build proper transfer call
			token := trc20Token(pool.Client(), contractAddr, cache)
			decimals, dErr := token.Decimals(ctx)
			if dErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("analyze_transfer_cost: failed to get token decimals: %v", dErr)), nil
			}

			amount, pErr := util.ParseTRC20Amount(amountStr, int(decimals))
			if pErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid amount: %v", pErr)), nil
			}
			if amount.Sign() <= 0 {
				return mcp.NewToolResultError("amount must be greater than zero"), nil
			}

			energyRequired, err = estimateTRC20Energy(ctx, pool, from, to, contractAddr, amount)
			if err != nil {
				errMsg := err.Error()
				if strings.Contains(errMsg, "REVERT") {
					return mcp.NewToolResultError(fmt.Sprintf("analyze_transfer_cost: contract reverted — the sender may not hold enough tokens for this transfer. Detail: %v", err)), nil
				}
				// Non-fatal for unsupported estimation — still return cost analysis
				log.Printf("analyze_transfer_cost: energy estimation failed: %v", err)
			} else {
				energyEstimated = true
			}
		} else {
			transferType = "TRX"
			energyEstimated = true // no energy needed for TRX
			progress.Send(3, "Calculating TRX transfer cost...")
		}

		// Bandwidth: TRX transfers ~267 bytes, TRC20 ~345 bytes
		var bandwidthRequired int64
		if isTRC20 {
			bandwidthRequired = 345
		} else {
			bandwidthRequired = 267
		}

		result := buildCostResult(
			transferType, from, to, contractAddr,
			energyRequired, bandwidthRequired,
			res, energyPrice, bandwidthPrice,
			energyEstimated,
		)

		return mcp.NewToolResultJSON(result)
	}
}

func buildCostResult(
	transferType, from, to, contractAddr string,
	energyRequired, bandwidthRequired int64,
	res *api.AccountResourceMessage,
	energyPrice, bandwidthPrice int64,
	energyEstimated bool,
) map[string]any {
	energyAvailable := res.EnergyLimit - res.EnergyUsed
	bandwidthAvailable := (res.NetLimit - res.NetUsed) + (res.FreeNetLimit - res.FreeNetUsed)

	result := map[string]any{
		"transfer_type":               transferType,
		"from":                        from,
		"to":                          to,
		"energy_required":             energyRequired,
		"bandwidth_required":          bandwidthRequired,
		"account_energy_available":    energyAvailable,
		"account_bandwidth_available": bandwidthAvailable,
	}

	if contractAddr != "" {
		result["contract_address"] = contractAddr
	}

	// Build cost options
	var options []map[string]any
	var totalCost int64

	// Energy cost
	if energyRequired > 0 {
		if energyAvailable >= energyRequired {
			options = append(options, map[string]any{
				"resource": "energy",
				"method":   "use_staked",
				"cost_sun": int64(0),
				"cost_trx": "0.000000",
				"note":     fmt.Sprintf("Sufficient staked energy (%d available, %d needed)", energyAvailable, energyRequired),
			})
		} else {
			deficit := energyRequired - energyAvailable
			burnCost := deficit * energyPrice
			totalCost += burnCost
			options = append(options, map[string]any{
				"resource":       "energy",
				"method":         "burn_trx",
				"cost_sun":       burnCost,
				"cost_trx":       util.SunToTRX(burnCost),
				"energy_deficit": deficit,
				"note":           fmt.Sprintf("Need %d more energy (have %d, need %d)", deficit, energyAvailable, energyRequired),
			})
		}
	}

	// Bandwidth cost
	if bandwidthAvailable >= bandwidthRequired {
		options = append(options, map[string]any{
			"resource": "bandwidth",
			"method":   "use_staked",
			"cost_sun": int64(0),
			"cost_trx": "0.000000",
			"note":     fmt.Sprintf("Sufficient bandwidth (%d available, %d needed)", bandwidthAvailable, bandwidthRequired),
		})
	} else {
		deficit := bandwidthRequired - bandwidthAvailable
		burnCost := deficit * bandwidthPrice
		totalCost += burnCost
		options = append(options, map[string]any{
			"resource":          "bandwidth",
			"method":            "burn_trx",
			"cost_sun":          burnCost,
			"cost_trx":          util.SunToTRX(burnCost),
			"bandwidth_deficit": deficit,
			"note":              fmt.Sprintf("Need %d more bandwidth (have %d, need %d)", deficit, bandwidthAvailable, bandwidthRequired),
		})
	}

	result["cost_breakdown"] = options
	result["total_estimated_cost_sun"] = totalCost
	result["total_estimated_cost_trx"] = util.SunToTRX(totalCost)
	result["energy_price_sun"] = energyPrice
	result["bandwidth_price_sun"] = bandwidthPrice

	if !energyEstimated && transferType == "TRC20" {
		result["warning"] = "Energy estimation unavailable on this node. Use --fallback-node with a node that supports EstimateEnergy, or call estimate_energy separately. Typical USDT transfer requires ~29,000-65,000 energy."
	}

	return result
}

// getCurrentPrice fetches the latest energy or bandwidth price.
// Returns the price in SUN per unit, or 0 if unavailable.
func getCurrentPrice(ctx context.Context, conn *client.GrpcClient, resource string) int64 {
	var entries []client.PriceEntry
	var err error
	if resource == "energy" {
		entries, err = conn.GetEnergyPriceHistoryCtx(ctx)
	} else {
		entries, err = conn.GetBandwidthPriceHistoryCtx(ctx)
	}
	if err != nil || len(entries) == 0 {
		return 0
	}
	return entries[len(entries)-1].Price
}

// estimateTRC20Energy estimates energy for a TRC20 transfer using the TRC20 token interface.
func estimateTRC20Energy(ctx context.Context, pool *nodepool.Pool, from, to, contractAddr string, amount *big.Int) (int64, error) {
	newCall := func(c contract.Client) *contract.ContractCall {
		token := trc20.New(c, contractAddr)
		return token.Transfer(from, to, amount)
	}

	energy, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (int64, error) {
		return newCall(pool.Client()).EstimateEnergy(ctx)
	})
	if err != nil && isEstimateEnergyUnsupported(err) {
		if fallback := pool.FallbackClient(); fallback != nil {
			energy, err = newCall(fallback).EstimateEnergy(ctx)
		}
	}
	return energy, err
}
