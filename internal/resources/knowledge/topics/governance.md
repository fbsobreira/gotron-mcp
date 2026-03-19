# TRON Governance

## Super Representatives (SR)

- 27 Super Representatives produce blocks on the TRON network
- Elected by TRX holders through voting
- Must stake TRX to gain voting power (1 staked TRX = 1 vote)
- SRs earn block rewards and voting rewards
- Block time: ~3 seconds

## Voting

- Stake TRX first to get voting power
- Votes can be split across multiple SRs
- Votes reset when you unstake
- Re-vote to change your vote distribution

## Proposals

- SRs can create network parameter change proposals
- Other SRs vote to approve or reject
- Parameters include energy prices, bandwidth prices, and other network settings

## SDK: Governance Operations

```go
// List all super representatives
witnesses, err := conn.ListWitnesses()
for _, w := range witnesses.Witnesses {
    // w.Address ([]byte), w.VoteCount, w.Url
    // w.TotalProduced, w.TotalMissed, w.LatestBlockNum
}

// Paginated witness listing (for large result sets)
witnesses, err := conn.ListWitnessesPaginated(0, 27)  // page 0, limit 27

// Vote for SRs (requires staked TRX)
votes := map[string]int64{
    "TSRAddress1...": 1000,
    "TSRAddress2...": 500,
}
tx, err := conn.VoteWitnessAccount("TVoterAddr...", votes)

// List governance proposals
proposals, err := conn.ProposalsList()
for _, p := range proposals.Proposals {
    // p.ProposalId, p.ProposerAddress, p.Parameters
    // p.ExpirationTime, p.State, p.Approvals
}
```

## SDK: Fluent Vote Builder (v0.25.2+)

```go
import "github.com/fbsobreira/gotron-sdk/pkg/txbuilder"

builder := txbuilder.New(conn)

// Fluent vote chaining
receipt, err := builder.VoteWitness(from).
    Vote("TSR1addr...", 1000).
    Vote("TSR2addr...", 500).
    Send(ctx, signer)

// Or from a map
votes := map[string]int64{"TSR1addr...": 1000, "TSR2addr...": 500}
receipt, err := builder.VoteWitness(from).
    Votes(votes).
    Send(ctx, signer)

// Build unsigned (for external signing)
tx, err := builder.VoteWitness(from).
    Vote("TSR1addr...", 1000).
    Build(ctx)
```

## MCP Tools

- `list_witnesses` — List all super representatives
- `vote_witness` — Vote for SRs (returns unsigned tx)
- `list_proposals` — List governance proposals
