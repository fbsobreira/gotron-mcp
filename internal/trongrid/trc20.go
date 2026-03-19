package trongrid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// GetTRC20Transfers returns TRC20 token transfer history for a TRON address.
func (c *Client) GetTRC20Transfers(ctx context.Context, addr string, opts QueryOpts) (*Response[TRC20Transfer], error) {
	path := fmt.Sprintf("/v1/accounts/%s/transactions/trc20", url.PathEscape(addr))
	body, err := c.doRequest(ctx, path, buildParams(opts))
	if err != nil {
		return nil, err
	}
	var resp Response[TRC20Transfer]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding TRC20 transfers response: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("TronGrid returned success=false for TRC20 transfers query")
	}
	return &resp, nil
}
