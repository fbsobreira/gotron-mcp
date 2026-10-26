package tools

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"testing"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func addMockFallback(t *testing.T, pool *nodepool.Pool, mock *mockWalletServer) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	api.RegisterWalletServer(srv, mock)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() { srv.Stop(); _ = lis.Close() })
	require.NoError(t, pool.AddFallback("passthrough:///fallback", []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}))
	t.Cleanup(func() { pool.FallbackClient().Stop() })
}

func TestContractReads_NodeFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *api.TransactionExtention
		want     string
	}{
		{"empty", &api.TransactionExtention{Result: &api.Return{Result: true}}, "empty constant result"},
		{"empty bytes", &api.TransactionExtention{Result: &api.Return{Result: true}, ConstantResult: [][]byte{nil}}, "empty constant result"},
		{"failure", &api.TransactionExtention{Result: &api.Return{Code: api.Return_CONTRACT_EXE_ERROR, Message: []byte("VM is unavailable")}}, "VM is unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := newMockPool(t, &mockWalletServer{TriggerConstantContractFunc: func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error) {
				return tc.response, nil
			}})
			for _, handler := range []struct {
				name    string
				handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
				args    map[string]any
			}{
				{"token", handleGetTRC20Balance(pool, nil), map[string]any{"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", "contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"}},
				{"raw", handleTriggerConstantContract(pool), map[string]any{"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", "data": "313ce567"}},
			} {
				t.Run(handler.name, func(t *testing.T) {
					r := callTool(t, handler.handler, handler.args)
					if handler.name == "raw" && tc.response.GetResult().GetResult() {
						require.False(t, r.IsError, "%v", r.Content)
					} else {
						require.True(t, r.IsError)
						assert.Contains(t, r.Content[0].(mcp.TextContent).Text, tc.want)
					}
				})
			}
		})
	}
}

func TestContractReads_EmptyPrimaryUsesFallback(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{TriggerConstantContractFunc: func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		return &api.TransactionExtention{Result: &api.Return{Result: true}}, nil
	}})
	addMockFallback(t, pool, mockTRC20Server(big.NewInt(1_000_000), 6, "Tether USD", "USDT"))
	r := callTool(t, handleGetTRC20Balance(pool, nil), map[string]any{"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", "contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"})
	require.False(t, r.IsError, "%v", r.Content)
	assert.Equal(t, "1000000", parseJSONResult(t, r)["balance_raw"])
	// Repeated token reads must still use fallback without changing block health.
	r = callTool(t, handleGetTRC20TokenInfo(pool, nil), map[string]any{"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"})
	require.False(t, r.IsError, "%v", r.Content)
	assert.Equal(t, float64(6), parseJSONResult(t, r)["decimals"])
}

func TestAnalyzeTransferCost_Activation(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		staked, used, free, rate, fee, bwFee int64
		activated                            bool
		want                                 string
	}{
		{"free bandwidth cannot activate", 0, 0, 600, 1, 1_000_000, 100_000, false, "1.100000"},
		{"enough staked", 267, 0, 600, 1, 1_000_000, 100_000, false, "1.000000"},
		{"partial staked", 266, 0, 600, 1, 1_000_000, 100_000, false, "1.100000"},
		{"used staked", 300, 34, 600, 1, 1_000_000, 100_000, false, "1.100000"},
		{"live parameters and multiplier", 533, 0, 600, 2, 2_000_000, 200_000, false, "2.200000"},
		{"multiplier covered", 534, 0, 600, 2, 2_000_000, 200_000, false, "2.000000"},
		{"already activated", 0, 0, 600, 1, 1_000_000, 100_000, true, "0.000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockWalletServer{
				GetAccountFunc: func(_ context.Context, in *core.Account) (*core.Account, error) {
					if tc.activated {
						return &core.Account{Address: in.Address}, nil
					}
					return &core.Account{}, nil
				},
				GetAccountResourceFunc: func(context.Context, *core.Account) (*api.AccountResourceMessage, error) {
					return &api.AccountResourceMessage{NetLimit: tc.staked, NetUsed: tc.used, FreeNetLimit: tc.free}, nil
				},
				GetEnergyPricesFunc: func(context.Context, *api.EmptyMessage) (*api.PricesResponseMessage, error) {
					return &api.PricesResponseMessage{Prices: "1000000:100"}, nil
				},
				GetBandwidthPricesFunc: func(context.Context, *api.EmptyMessage) (*api.PricesResponseMessage, error) {
					return &api.PricesResponseMessage{Prices: "1000000:1000"}, nil
				},
				GetChainParametersFunc: func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
					return &core.ChainParameters{ChainParameter: []*core.ChainParameters_ChainParameter{
						{Key: "getCreateNewAccountFeeInSystemContract", Value: tc.fee}, {Key: "getCreateAccountFee", Value: tc.bwFee}, {Key: "getCreateNewAccountBandwidthRate", Value: tc.rate},
					}}, nil
				},
			}
			r := callTool(t, handleAnalyzeTransferCost(newMockPool(t, mock), nil), map[string]any{"from": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", "to": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", "amount": "1"})
			require.False(t, r.IsError, "%v", r.Content)
			d := parseJSONResult(t, r)
			assert.Equal(t, tc.want, d["total_estimated_cost_trx"])
			assert.Equal(t, tc.activated, d["recipient_activated"])
		})
	}
}

func TestAnalyzeTransferCost_RecipientLookupFailure(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountResourceFunc: func(context.Context, *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{FreeNetLimit: 600}, nil
		},
		GetAccountFunc: func(context.Context, *core.Account) (*core.Account, error) {
			return nil, fmt.Errorf("recipient lookup unavailable")
		},
	}
	r := callTool(t, handleAnalyzeTransferCost(newMockPool(t, mock), nil), map[string]any{"from": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", "to": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", "amount": "1"})
	require.True(t, r.IsError)
	assert.Contains(t, r.Content[0].(mcp.TextContent).Text, "recipient lookup unavailable")
}

func TestContractReads_RevertDoesNotUseFallback(t *testing.T) {
	for _, response := range []*api.TransactionExtention{
		{Result: &api.Return{Code: api.Return_CONTRACT_EXE_ERROR, Message: []byte("REVERT: denied")}},
		{Result: &api.Return{Result: true}, Transaction: &core.Transaction{Ret: []*core.Transaction_Result{{ContractRet: core.Transaction_Result_REVERT}}}},
	} {
		pool := newMockPool(t, &mockWalletServer{TriggerConstantContractFunc: func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return response, nil
		}})
		addMockFallback(t, pool, mockTRC20Server(big.NewInt(1), 6, "Tether USD", "USDT"))
		r := callTool(t, handleGetTRC20Balance(pool, nil), map[string]any{"address": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", "contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"})
		require.True(t, r.IsError, "a successful fallback must not hide a contract revert")
		assert.Contains(t, r.Content[0].(mcp.TextContent).Text, "REVERT")
	}
}

func TestContractReads_UnsupportedPrimaryUsesFallback(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{TriggerConstantContractFunc: func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		return &api.TransactionExtention{Result: &api.Return{Code: api.Return_CONTRACT_EXE_ERROR, Message: []byte("this node does not support constant")}}, nil
	}})
	addMockFallback(t, pool, mockTRC20Server(big.NewInt(1), 6, "Tether USD", "USDT"))
	r := callTool(t, handleGetTRC20TokenInfo(pool, nil), map[string]any{"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"})
	require.False(t, r.IsError, "%v", r.Content)
	assert.Equal(t, float64(6), parseJSONResult(t, r)["decimals"])
}

func TestContractReads_FallbackFailureIncludesBothErrors(t *testing.T) {
	pool := newMockPool(t, &mockWalletServer{TriggerConstantContractFunc: func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		return &api.TransactionExtention{Result: &api.Return{Result: true}}, nil
	}})
	addMockFallback(t, pool, &mockWalletServer{TriggerConstantContractFunc: func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		return &api.TransactionExtention{Result: &api.Return{Code: api.Return_CONTRACT_EXE_ERROR, Message: []byte("fallback VM failed")}}, nil
	}})
	r := callTool(t, handleGetTRC20TokenInfo(pool, nil), map[string]any{"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"})
	require.True(t, r.IsError)
	assert.Contains(t, r.Content[0].(mcp.TextContent).Text, "empty constant result")
	assert.Contains(t, r.Content[0].(mcp.TextContent).Text, "fallback VM failed")
}

func TestAnalyzeTransferCost_ActivationParametersUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params *core.ChainParameters
		err    error
	}{
		{name: "missing", params: &core.ChainParameters{}},
		{name: "RPC failure", err: fmt.Errorf("chain parameters unavailable")},
		{name: "invalid rate", params: &core.ChainParameters{ChainParameter: []*core.ChainParameters_ChainParameter{
			{Key: "getCreateNewAccountFeeInSystemContract", Value: 1_000_000}, {Key: "getCreateAccountFee", Value: 100_000}, {Key: "getCreateNewAccountBandwidthRate", Value: 0},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockWalletServer{
				GetAccountResourceFunc: func(context.Context, *core.Account) (*api.AccountResourceMessage, error) {
					return &api.AccountResourceMessage{FreeNetLimit: 600}, nil
				},
				GetAccountFunc:         func(context.Context, *core.Account) (*core.Account, error) { return &core.Account{}, nil },
				GetChainParametersFunc: func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) { return tc.params, tc.err },
			}
			r := callTool(t, handleAnalyzeTransferCost(newMockPool(t, mock), nil), map[string]any{"from": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", "to": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", "amount": "1"})
			require.True(t, r.IsError, "must not quote zero when account creation parameters are unknown")
		})
	}
}

func TestAnalyzeTransferCost_EmptyPrimaryDecimalsUsesFallback(t *testing.T) {
	mock := &mockWalletServer{
		GetAccountResourceFunc: func(context.Context, *core.Account) (*api.AccountResourceMessage, error) {
			return &api.AccountResourceMessage{EnergyLimit: 50_000, FreeNetLimit: 600}, nil
		},
		GetEnergyPricesFunc: func(context.Context, *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:100"}, nil
		},
		GetBandwidthPricesFunc: func(context.Context, *api.EmptyMessage) (*api.PricesResponseMessage, error) {
			return &api.PricesResponseMessage{Prices: "1000000:1000"}, nil
		},
		TriggerConstantContractFunc: func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error) {
			return &api.TransactionExtention{Result: &api.Return{Result: true}}, nil
		},
		EstimateEnergyFunc: func(context.Context, *core.TriggerSmartContract) (*api.EstimateEnergyMessage, error) {
			return &api.EstimateEnergyMessage{Result: &api.Return{Result: true}, EnergyRequired: 29_000}, nil
		},
	}
	pool := newMockPool(t, mock)
	addMockFallback(t, pool, mockTRC20Server(big.NewInt(1_000_000), 6, "Tether USD", "USDT"))
	r := callTool(t, handleAnalyzeTransferCost(pool, nil), map[string]any{
		"from": "TKSXDA8HfE9E1y39RczVQ1ZascUEtaSToF", "to": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", "amount": "1",
	})
	require.False(t, r.IsError, "%v", r.Content)
	assert.Equal(t, float64(29_000), parseJSONResult(t, r)["energy_required"])
}

func TestTriggerConstantContract_SuccessWithoutReturnData(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		for _, results := range [][][]byte{nil, {nil}} {
			for _, packed := range []bool{false, true} {
				t.Run(fmt.Sprintf("fallback=%t/results=%v/packed=%t", fallback, results, packed), func(t *testing.T) {
					pool := newMockPool(t, &mockWalletServer{TriggerConstantContractFunc: func(_ context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
						assert.Equal(t, int64(1_000_000), in.CallValue)
						return &api.TransactionExtention{
							Result: &api.Return{Result: true}, ConstantResult: results,
							Transaction: &core.Transaction{Ret: []*core.Transaction_Result{{ContractRet: core.Transaction_Result_SUCCESS}}},
						}, nil
					}})
					if fallback {
						addMockFallback(t, pool, &mockWalletServer{TriggerConstantContractFunc: func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error) {
							t.Error("successful simulation must not be replayed on fallback")
							return &api.TransactionExtention{Result: &api.Return{Result: true}}, nil
						}})
					}
					args := map[string]any{"contract_address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", "call_value": float64(1_000_000)}
					if packed {
						args["data"] = "d0e30db0"
					} else {
						args["method"] = "deposit()"
					}
					result := callTool(t, handleTriggerConstantContract(pool), args)
					require.False(t, result.IsError, "%v", result.Content)
					data := parseJSONResult(t, result)
					assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", data["contract_address"])
				})
			}
		}
	}
}
