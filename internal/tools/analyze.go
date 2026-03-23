package tools

import (
	"bytes"
	"context"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-mcp/internal/util"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterAnalyzeTools registers the analyze_account tool (read-only).
func RegisterAnalyzeTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("analyze_account",
			mcp.WithDescription("Comprehensive account overview: balance, resources, staking, voting, permissions, and delegations in a single call"),
			mcp.WithString("address", mcp.Required(), mcp.Description("TRON base58 address (starts with T)")),
		),
		handleAnalyzeAccount(pool),
	)
}

func handleAnalyzeAccount(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		progress := newProgressReporter(ctx, req, 4)

		// 1. Account info
		progress.Send(1, "Fetching account info...")
		acc, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*core.Account, error) {
			return pool.Client().GetAccountCtx(ctx, addr)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("analyze_account: %v", err)), nil
		}

		// 2. Resources
		progress.Send(2, "Fetching resources...")
		res, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*api.AccountResourceMessage, error) {
			return pool.Client().GetAccountResourceCtx(ctx, addr)
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("analyze_account: %v", err)), nil
		}

		// 3. Delegations
		progress.Send(3, "Fetching delegations...")
		delegatedTo, _ := retry.DoWithFailover(ctx, pool, func(ctx context.Context) ([]*api.DelegatedResourceList, error) {
			return pool.Client().GetDelegatedResourcesV2Ctx(ctx, addr)
		})
		receivedFrom, _ := retry.DoWithFailover(ctx, pool, func(ctx context.Context) ([]*api.DelegatedResourceList, error) {
			return pool.Client().GetReceivedDelegatedResourcesV2Ctx(ctx, addr)
		})

		// 4. Witness info (only if account is an SR)
		var witness *core.Witness
		if acc.IsWitness {
			progress.Send(4, "Fetching witness info...")
			witnesses, wErr := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*api.WitnessList, error) {
				return pool.Client().ListWitnessesCtx(ctx)
			})
			if wErr == nil {
				witness = findWitness(witnesses, acc.Address)
			}
		}

		result := buildAnalyzeResult(addr, acc, res, delegatedTo, receivedFrom, witness)
		return mcp.NewToolResultJSON(result)
	}
}

func buildAnalyzeResult(
	addr string,
	acc *core.Account,
	res *api.AccountResourceMessage,
	delegatedTo []*api.DelegatedResourceList,
	receivedFrom []*api.DelegatedResourceList,
	witness *core.Witness,
) map[string]any {
	result := map[string]any{
		"address":      addr,
		"balance_trx":  util.SunToTRX(acc.Balance),
		"balance_sun":  acc.Balance,
		"account_type": acc.Type.String(),
		"is_witness":   acc.IsWitness,
	}

	if len(acc.AccountName) > 0 {
		result["account_name"] = string(acc.AccountName)
	}

	// Witness / SR info
	if witness != nil {
		sr := map[string]any{
			"total_votes":    witness.VoteCount,
			"total_produced": witness.TotalProduced,
			"total_missed":   witness.TotalMissed,
			"is_active":      witness.IsJobs,
		}
		if witness.Url != "" {
			sr["url"] = witness.Url
		}
		if witness.LatestBlockNum > 0 {
			sr["latest_block_num"] = witness.LatestBlockNum
		}
		result["witness"] = sr
	}

	// Staking (Stake 2.0)
	staking := buildStakingInfo(acc)
	if len(staking) > 0 {
		result["staking"] = staking
	}

	// Resources
	result["resources"] = map[string]any{
		"energy_used":          res.EnergyUsed,
		"energy_limit":         res.EnergyLimit,
		"bandwidth_used":       res.NetUsed,
		"bandwidth_limit":      res.NetLimit,
		"free_bandwidth_used":  res.FreeNetUsed,
		"free_bandwidth_limit": res.FreeNetLimit,
	}

	// Voting
	if len(acc.Votes) > 0 {
		result["voting"] = buildVotingInfo(acc)
	}

	// Delegations
	delegations := buildDelegationSummary(delegatedTo, receivedFrom)
	if len(delegations) > 0 {
		result["delegations"] = delegations
	}

	// Permissions summary
	if acc.OwnerPermission != nil || len(acc.ActivePermission) > 0 || acc.WitnessPermission != nil {
		perms := buildPermissionsSummary(acc)
		result["permissions"] = perms
	}

	return result
}

func buildPermissionsSummary(acc *core.Account) map[string]any {
	perms := map[string]any{}
	isMultiSig := false

	if acc.OwnerPermission != nil {
		ownerKeys := make([]string, len(acc.OwnerPermission.Keys))
		for i, k := range acc.OwnerPermission.Keys {
			ownerKeys[i] = address.BytesToAddress(k.Address).String()
		}
		perms["owner"] = map[string]any{
			"threshold": acc.OwnerPermission.Threshold,
			"keys":      ownerKeys,
		}
		if acc.OwnerPermission.Threshold > 1 || len(acc.OwnerPermission.Keys) > 1 {
			isMultiSig = true
		}
	}

	if len(acc.ActivePermission) > 0 {
		active := make([]map[string]any, len(acc.ActivePermission))
		for i, p := range acc.ActivePermission {
			active[i] = map[string]any{
				"id":        p.Id,
				"name":      p.PermissionName,
				"threshold": p.Threshold,
			}
			if p.Threshold > 1 || len(p.Keys) > 1 {
				isMultiSig = true
			}
		}
		perms["active_permissions"] = active
	}

	if acc.WitnessPermission != nil {
		witnessKeyDiffers := false
		if acc.OwnerPermission != nil && len(acc.OwnerPermission.Keys) > 0 && len(acc.WitnessPermission.Keys) > 0 {
			witnessKeyDiffers = !bytes.Equal(acc.WitnessPermission.Keys[0].Address, acc.OwnerPermission.Keys[0].Address)
		}
		perms["witness_key_differs"] = witnessKeyDiffers
	}

	perms["is_multi_sig"] = isMultiSig
	return perms
}

func buildStakingInfo(acc *core.Account) map[string]any {
	staking := map[string]any{}
	var frozenEnergy, frozenBandwidth int64

	for _, f := range acc.FrozenV2 {
		switch f.Type {
		case core.ResourceCode_ENERGY:
			frozenEnergy += f.Amount
		case core.ResourceCode_BANDWIDTH:
			frozenBandwidth += f.Amount
		}
	}

	if frozenEnergy > 0 {
		staking["frozen_energy_sun"] = frozenEnergy
		staking["frozen_energy_trx"] = util.SunToTRX(frozenEnergy)
	}
	if frozenBandwidth > 0 {
		staking["frozen_bandwidth_sun"] = frozenBandwidth
		staking["frozen_bandwidth_trx"] = util.SunToTRX(frozenBandwidth)
	}

	// Tron power (voting power) = total frozen / 1_000_000
	tronPower := frozenEnergy + frozenBandwidth
	if tronPower > 0 {
		staking["tron_power"] = tronPower / 1_000_000
	}

	// Pending unfreezes
	var pendingUnfreeze int64
	for _, u := range acc.UnfrozenV2 {
		pendingUnfreeze += u.UnfreezeAmount
	}
	if pendingUnfreeze > 0 {
		staking["pending_unfreeze_sun"] = pendingUnfreeze
		staking["pending_unfreeze_trx"] = util.SunToTRX(pendingUnfreeze)
		staking["pending_unfreeze_count"] = len(acc.UnfrozenV2)
	}

	return staking
}

func buildVotingInfo(acc *core.Account) map[string]any {
	var totalVotes int64
	votedFor := make([]map[string]any, len(acc.Votes))
	for i, v := range acc.Votes {
		totalVotes += v.VoteCount
		votedFor[i] = map[string]any{
			"address": address.BytesToAddress(v.VoteAddress).String(),
			"votes":   v.VoteCount,
		}
	}
	return map[string]any{
		"total_votes": totalVotes,
		"voted_for":   votedFor,
	}
}

func buildDelegationSummary(
	delegatedTo []*api.DelegatedResourceList,
	receivedFrom []*api.DelegatedResourceList,
) map[string]any {
	summary := map[string]any{}
	var energyOut, bwOut, energyIn, bwIn int64

	for _, list := range delegatedTo {
		if list == nil {
			continue
		}
		for _, dr := range list.DelegatedResource {
			energyOut += dr.FrozenBalanceForEnergy
			bwOut += dr.FrozenBalanceForBandwidth
		}
	}
	for _, list := range receivedFrom {
		if list == nil {
			continue
		}
		for _, dr := range list.DelegatedResource {
			energyIn += dr.FrozenBalanceForEnergy
			bwIn += dr.FrozenBalanceForBandwidth
		}
	}

	if energyOut > 0 {
		summary["energy_delegated_out_sun"] = energyOut
		summary["energy_delegated_out_trx"] = util.SunToTRX(energyOut)
	}
	if bwOut > 0 {
		summary["bandwidth_delegated_out_sun"] = bwOut
		summary["bandwidth_delegated_out_trx"] = util.SunToTRX(bwOut)
	}
	if energyIn > 0 {
		summary["energy_received_sun"] = energyIn
		summary["energy_received_trx"] = util.SunToTRX(energyIn)
	}
	if bwIn > 0 {
		summary["bandwidth_received_sun"] = bwIn
		summary["bandwidth_received_trx"] = util.SunToTRX(bwIn)
	}

	return summary
}

// findWitness searches a WitnessList for the witness matching the given address bytes.
func findWitness(witnesses *api.WitnessList, addr []byte) *core.Witness {
	if witnesses == nil {
		return nil
	}
	for _, w := range witnesses.Witnesses {
		if bytes.Equal(w.Address, addr) {
			return w
		}
	}
	return nil
}
