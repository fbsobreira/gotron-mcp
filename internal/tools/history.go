package tools

import (
	"context"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/trongrid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const defaultHistoryLimit = 10

// HistoryClient defines the TronGrid REST API methods used by history tools.
type HistoryClient interface {
	GetTransactionsByAddress(ctx context.Context, addr string, opts trongrid.QueryOpts) (*trongrid.Response[trongrid.Transaction], error)
	GetTRC20Transfers(ctx context.Context, addr string, opts trongrid.QueryOpts) (*trongrid.Response[trongrid.TRC20Transfer], error)
	GetContractEvents(ctx context.Context, contractAddr string, opts trongrid.QueryOpts) (*trongrid.Response[trongrid.ContractEvent], error)
	GetContractEventsByName(ctx context.Context, contractAddr, eventName string, opts trongrid.QueryOpts) (*trongrid.Response[trongrid.ContractEvent], error)
}

// RegisterHistoryTools registers TronGrid REST API tools for transaction history,
// TRC20 transfers, and contract events.
func RegisterHistoryTools(s *server.MCPServer, client HistoryClient) {
	paginationOpts := []mcp.ToolOption{
		mcp.WithNumber("limit", mcp.Description("Number of results to return (1-200, default 10). Start small to avoid large responses; use fingerprint to paginate for more")),
		mcp.WithString("fingerprint", mcp.Description("Pagination cursor from previous response meta.fingerprint — pass to fetch the next page")),
		mcp.WithBoolean("only_confirmed", mcp.Description("Only return confirmed transactions")),
		mcp.WithNumber("min_timestamp", mcp.Description("Minimum block timestamp in milliseconds")),
		mcp.WithNumber("max_timestamp", mcp.Description("Maximum block timestamp in milliseconds")),
	}

	s.AddTool(
		mcp.NewTool("get_transaction_history",
			append([]mcp.ToolOption{
				mcp.WithDescription("Get transaction history for a TRON address via TronGrid REST API. Returns a compact summary per transaction. Use a small limit and paginate with fingerprint to control response size"),
				mcp.WithString("address", mcp.Required(), mcp.Description("TRON base58 address (starts with T)")),
				mcp.WithBoolean("only_to", mcp.Description("Only show incoming transactions")),
				mcp.WithBoolean("only_from", mcp.Description("Only show outgoing transactions")),
			}, paginationOpts...)...,
		),
		handleGetTransactionHistory(client),
	)

	s.AddTool(
		mcp.NewTool("get_trc20_transfers",
			append([]mcp.ToolOption{
				mcp.WithDescription("Get TRC20 token transfer history for a TRON address via TronGrid REST API. Returns a compact summary per transfer with token info. Use a small limit and paginate with fingerprint to control response size"),
				mcp.WithString("address", mcp.Required(), mcp.Description("TRON base58 address (starts with T)")),
				mcp.WithBoolean("only_to", mcp.Description("Only show incoming transfers")),
				mcp.WithBoolean("only_from", mcp.Description("Only show outgoing transfers")),
			}, paginationOpts...)...,
		),
		handleGetTRC20Transfers(client),
	)

	s.AddTool(
		mcp.NewTool("get_contract_events",
			append([]mcp.ToolOption{
				mcp.WithDescription("Get decoded events emitted by a smart contract via TronGrid REST API. Returns a compact summary per event. Use a small limit and paginate with fingerprint to control response size"),
				mcp.WithString("contract_address", mcp.Required(), mcp.Description("TRON smart contract base58 address (starts with T)")),
				mcp.WithString("event_name", mcp.Description("Filter by specific event name (e.g. Transfer, Approval)")),
			}, paginationOpts...)...,
		),
		handleGetContractEvents(client),
	)
}

func parseQueryOpts(req mcp.CallToolRequest) (trongrid.QueryOpts, error) {
	opts := trongrid.QueryOpts{}

	limit := req.GetInt("limit", defaultHistoryLimit)
	if limit < 1 {
		limit = defaultHistoryLimit
	}
	opts.Limit = min(limit, 200)

	opts.Fingerprint = req.GetString("fingerprint", "")
	opts.OnlyConfirmed = req.GetBool("only_confirmed", false)
	opts.OnlyTo = req.GetBool("only_to", false)
	opts.OnlyFrom = req.GetBool("only_from", false)

	if opts.OnlyTo && opts.OnlyFrom {
		return opts, fmt.Errorf("only_to and only_from are mutually exclusive")
	}

	if v := req.GetInt("min_timestamp", 0); v > 0 {
		opts.MinTimestamp = int64(v)
	}
	if v := req.GetInt("max_timestamp", 0); v > 0 {
		opts.MaxTimestamp = int64(v)
	}

	if opts.MinTimestamp > 0 && opts.MaxTimestamp > 0 && opts.MinTimestamp > opts.MaxTimestamp {
		return opts, fmt.Errorf("min_timestamp cannot be greater than max_timestamp")
	}

	return opts, nil
}

func handleGetTransactionHistory(client HistoryClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid address: %v", err)), nil
		}

		opts, err := parseQueryOpts(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		resp, err := client.GetTransactionsByAddress(ctx, addr, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_transaction_history: %v", err)), nil
		}
		if resp == nil {
			return mcp.NewToolResultError("get_transaction_history: nil response"), nil
		}

		txs := make([]map[string]any, 0, len(resp.Data))
		for _, tx := range resp.Data {
			entry := map[string]any{
				"txid":            tx.TransactionID,
				"block_number":    tx.BlockNumber,
				"block_timestamp": tx.BlockTimestamp,
			}
			if len(tx.RawData.Contract) > 0 {
				c := tx.RawData.Contract[0]
				entry["type"] = c.Type
				if v, ok := c.Parameter.Value["owner_address"]; ok {
					entry["from"] = v
				}
				if v, ok := c.Parameter.Value["to_address"]; ok {
					entry["to"] = v
				}
				if v, ok := c.Parameter.Value["amount"]; ok {
					entry["amount"] = v
				}
				if v, ok := c.Parameter.Value["contract_address"]; ok {
					entry["contract_address"] = v
				}
			}
			if len(tx.Ret) > 0 {
				entry["result"] = tx.Ret[0].ContractRet
			}
			txs = append(txs, entry)
		}

		result := map[string]any{
			"address":      addr,
			"count":        len(txs),
			"transactions": txs,
			"meta": map[string]any{
				"page_size":   resp.Meta.PageSize,
				"fingerprint": resp.Meta.Fingerprint,
			},
		}
		return mcp.NewToolResultJSON(result)
	}
}

func handleGetTRC20Transfers(client HistoryClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("address", "")
		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid address: %v", err)), nil
		}

		opts, err := parseQueryOpts(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		resp, err := client.GetTRC20Transfers(ctx, addr, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_trc20_transfers: %v", err)), nil
		}
		if resp == nil {
			return mcp.NewToolResultError("get_trc20_transfers: nil response"), nil
		}

		transfers := make([]map[string]any, 0, len(resp.Data))
		for _, tr := range resp.Data {
			transfers = append(transfers, map[string]any{
				"txid":            tr.TransactionID,
				"from":            tr.From,
				"to":              tr.To,
				"value":           tr.Value,
				"block_timestamp": tr.BlockTimestamp,
				"token_symbol":    tr.TokenInfo.Symbol,
				"token_name":      tr.TokenInfo.Name,
				"token_decimals":  tr.TokenInfo.Decimals,
				"token_address":   tr.TokenInfo.Address,
			})
		}

		result := map[string]any{
			"address":   addr,
			"count":     len(transfers),
			"transfers": transfers,
			"meta": map[string]any{
				"page_size":   resp.Meta.PageSize,
				"fingerprint": resp.Meta.Fingerprint,
			},
		}
		return mcp.NewToolResultJSON(result)
	}
}

func handleGetContractEvents(client HistoryClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr := req.GetString("contract_address", "")
		if err := validateAddress(addr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract_address: %v", err)), nil
		}

		opts, err := parseQueryOpts(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		eventName := req.GetString("event_name", "")

		var resp *trongrid.Response[trongrid.ContractEvent]
		if eventName != "" {
			resp, err = client.GetContractEventsByName(ctx, addr, eventName, opts)
		} else {
			resp, err = client.GetContractEvents(ctx, addr, opts)
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_contract_events: %v", err)), nil
		}
		if resp == nil {
			return mcp.NewToolResultError("get_contract_events: nil response"), nil
		}

		events := make([]map[string]any, 0, len(resp.Data))
		for _, ev := range resp.Data {
			events = append(events, map[string]any{
				"txid":            ev.TransactionID,
				"block_number":    ev.BlockNumber,
				"block_timestamp": ev.BlockTimestamp,
				"event_name":      ev.EventName,
				"result":          ev.Result,
			})
		}

		result := map[string]any{
			"contract_address": addr,
			"count":            len(events),
			"events":           events,
			"meta": map[string]any{
				"page_size":   resp.Meta.PageSize,
				"fingerprint": resp.Meta.Fingerprint,
			},
		}
		return mcp.NewToolResultJSON(result)
	}
}
