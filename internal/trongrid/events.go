package trongrid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// GetContractEvents returns decoded events emitted by a smart contract.
func (c *Client) GetContractEvents(ctx context.Context, contractAddr string, opts QueryOpts) (*Response[ContractEvent], error) {
	path := fmt.Sprintf("/v1/contracts/%s/events", url.PathEscape(contractAddr))
	params := buildParams(opts)
	return c.getEvents(ctx, path, params)
}

// GetContractEventsByName returns events filtered by event name.
func (c *Client) GetContractEventsByName(ctx context.Context, contractAddr, eventName string, opts QueryOpts) (*Response[ContractEvent], error) {
	path := fmt.Sprintf("/v1/contracts/%s/events", url.PathEscape(contractAddr))
	params := buildParams(opts)
	params.Set("event_name", eventName)
	return c.getEvents(ctx, path, params)
}

func (c *Client) getEvents(ctx context.Context, path string, params url.Values) (*Response[ContractEvent], error) {
	body, err := c.doRequest(ctx, path, params)
	if err != nil {
		return nil, err
	}
	var resp Response[ContractEvent]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding contract events response: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("TronGrid returned success=false for contract events query")
	}
	return &resp, nil
}
