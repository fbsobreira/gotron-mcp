package nodepool

import (
	"context"
	"fmt"
	"strings"

	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/fbsobreira/gotron-sdk/pkg/contract"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ContractClient validates execution status while allowing successful simulations
// with no return values. Unsupported constant execution uses the configured
// fallback for that call only. Writes are never automatically replayed.
func (p *Pool) ContractClient() contract.Client {
	return p.newContractClient(false)
}

// TRC20Client additionally requires return data for token reads. Empty responses
// use the configured fallback independently of block health checks.
func (p *Pool) TRC20Client() contract.Client {
	return p.newContractClient(true)
}

func (p *Pool) newContractClient(requireResult bool) contract.Client {
	c, node := p.ClientAndNode()
	return &contractClient{Client: c, primaryNode: node, fallback: p.FallbackClient(), requireResult: requireResult}
}

type contractClient struct {
	contract.Client
	primaryNode   string
	fallback      *client.GrpcClient
	requireResult bool
}

func (c *contractClient) TriggerConstantContractCtx(ctx context.Context, from, addr, method, params string, opts ...client.ConstantCallOption) (*api.TransactionExtention, error) {
	return c.constant(ctx, func(conn contract.Client) (*api.TransactionExtention, error) {
		return conn.TriggerConstantContractCtx(ctx, from, addr, method, params, opts...)
	})
}

func (c *contractClient) TriggerConstantContractWithDataCtx(ctx context.Context, from, addr string, data []byte, opts ...client.ConstantCallOption) (*api.TransactionExtention, error) {
	return c.constant(ctx, func(conn contract.Client) (*api.TransactionExtention, error) {
		return conn.TriggerConstantContractWithDataCtx(ctx, from, addr, data, opts...)
	})
}

func (c *contractClient) constant(ctx context.Context, call func(contract.Client) (*api.TransactionExtention, error)) (*api.TransactionExtention, error) {
	tx, err := call(c.Client)
	retryFallback := status.Code(err) == codes.Unimplemented
	if err == nil {
		err, retryFallback = validateConstantResult(tx, c.requireResult)
	}
	if err == nil {
		return tx, nil
	}
	primaryErr := fmt.Errorf("constant call on %s: %w", c.primaryNode, err)
	if !retryFallback || ctx.Err() != nil || c.fallback == nil || c.Client == c.fallback {
		return nil, primaryErr
	}
	tx, err = call(c.fallback)
	if err == nil {
		err, _ = validateConstantResult(tx, c.requireResult)
	}
	if err != nil {
		return nil, fmt.Errorf("%v; fallback constant call: %w", primaryErr, err)
	}
	return tx, nil
}

// The boolean identifies a missing node capability or malformed response, never
// a contract execution failure such as REVERT. Those must not be hidden by retry.
func validateConstantResult(tx *api.TransactionExtention, requireResult bool) (error, bool) {
	if tx == nil || tx.GetResult() == nil {
		return fmt.Errorf("missing constant call status; check node VM configuration"), true
	}
	r := tx.GetResult()
	if !r.GetResult() || r.GetCode() != api.Return_SUCCESS {
		msg := string(r.GetMessage())
		lower := strings.ToLower(msg)
		unsupported := strings.Contains(lower, "supportconstant") || strings.Contains(lower, "does not support constant") || strings.Contains(lower, "constant call is not supported")
		return fmt.Errorf("node rejected constant call (%s): %s", r.GetCode(), msg), unsupported
	}
	for _, ret := range tx.GetTransaction().GetRet() {
		if code := ret.GetContractRet(); code != core.Transaction_Result_DEFAULT && code != core.Transaction_Result_SUCCESS {
			return fmt.Errorf("contract execution failed: %s (result: %x)", code, tx.GetConstantResult()), false
		}
	}
	if results := tx.GetConstantResult(); requireResult && (len(results) == 0 || len(results[0]) == 0) {
		return fmt.Errorf("empty constant result; check vm.supportConstant and node health"), true
	}
	return nil, false
}
