package trongrid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// GetTransactionsByAddress returns the transaction history for a TRON address.
func (c *Client) GetTransactionsByAddress(ctx context.Context, addr string, opts QueryOpts) (*Response[Transaction], error) {
	path := fmt.Sprintf("/v1/accounts/%s/transactions", url.PathEscape(addr))
	body, err := c.doRequest(ctx, path, buildParams(opts))
	if err != nil {
		return nil, err
	}
	var resp Response[Transaction]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding transactions response: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("TronGrid returned success=false for transactions query")
	}
	return &resp, nil
}
