package nodepool

import (
	"context"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type constantWalletServer struct {
	api.UnimplementedWalletServer
	call func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error)
}

func (s *constantWalletServer) TriggerConstantContract(ctx context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
	return s.call(ctx, in)
}

func TestContractClient_Responses(t *testing.T) {
	word := make([]byte, 32)
	word[31] = 6
	success := &api.TransactionExtention{Result: &api.Return{Result: true}, ConstantResult: [][]byte{word}}
	empty := &api.TransactionExtention{Result: &api.Return{Result: true}}
	emptyBytes := &api.TransactionExtention{Result: &api.Return{Result: true}, ConstantResult: [][]byte{nil}}
	unsupported := &api.TransactionExtention{Result: &api.Return{Code: api.Return_CONTRACT_EXE_ERROR, Message: []byte("this node does not support constant")}}
	revert := &api.TransactionExtention{Result: &api.Return{Result: true}, Transaction: &core.Transaction{Ret: []*core.Transaction_Result{{ContractRet: core.Transaction_Result_REVERT}}}}
	for _, tc := range []struct {
		name              string
		token             bool
		response          *api.TransactionExtention
		rpcErr            error
		fallback          bool
		fallbackResponse  *api.TransactionExtention
		fallbackErr       error
		activeFallback    bool
		wantErr           string
		wantFallbackCalls int
	}{
		{name: "general successful output", response: success},
		{name: "general no output", response: empty, fallback: true},
		{name: "general empty bytes", response: emptyBytes, fallback: true},
		{name: "token successful output", token: true, response: success},
		{name: "token empty without fallback", token: true, response: empty, wantErr: "empty constant result"},
		{name: "token empty bytes without fallback", token: true, response: emptyBytes, wantErr: "empty constant result"},
		{name: "token empty uses fallback", token: true, response: empty, fallback: true, fallbackResponse: success, wantFallbackCalls: 1},
		{name: "general unsupported uses fallback", response: unsupported, fallback: true, fallbackResponse: empty, wantFallbackCalls: 1},
		{name: "unimplemented RPC uses fallback", token: true, rpcErr: status.Error(codes.Unimplemented, "constant execution unsupported"), fallback: true, fallbackResponse: success, wantFallbackCalls: 1},
		{name: "missing status uses fallback", token: true, response: &api.TransactionExtention{}, fallback: true, fallbackResponse: success, wantFallbackCalls: 1},
		{name: "node rejection preserved", token: true, response: &api.TransactionExtention{Result: &api.Return{Code: api.Return_CONTRACT_EXE_ERROR, Message: []byte("VM failed")}}, fallback: true, wantErr: "VM failed"},
		{name: "revert is not retried", token: true, response: revert, fallback: true, wantErr: "REVERT"},
		{name: "general revert is not retried", response: revert, fallback: true, wantErr: "REVERT"},
		{name: "transport error preserved", token: true, rpcErr: status.Error(codes.Unavailable, "node offline"), fallback: true, wantErr: "node offline"},
		{name: "fallback rejection preserved", token: true, response: empty, fallback: true, fallbackResponse: unsupported, wantErr: "fallback constant call", wantFallbackCalls: 1},
		{name: "fallback transport error preserved", token: true, response: empty, fallback: true, fallbackErr: status.Error(codes.Unavailable, "fallback offline"), wantErr: "fallback offline", wantFallbackCalls: 1},
		{name: "already on fallback is not replayed", token: true, response: empty, fallback: true, activeFallback: true, wantErr: "empty constant result", wantFallbackCalls: 1},
	} {
		for _, packed := range []bool{false, true} {
			mode := "method"
			if packed {
				mode = "data"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				checkRequest := func(in *core.TriggerSmartContract) {
					assert.Equal(t, int64(42), in.GetCallValue())
					assert.Equal(t, []byte{0x31, 0x3c, 0xe5, 0x67}, in.GetData())
				}
				primary := newBufconnClient(t, &constantWalletServer{call: func(_ context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
					checkRequest(in)
					return tc.response, tc.rpcErr
				}})
				p := NewFromClient(primary, "primary")
				fallbackCalls := 0
				if tc.fallback {
					fallback := newBufconnClient(t, &constantWalletServer{call: func(_ context.Context, in *core.TriggerSmartContract) (*api.TransactionExtention, error) {
						fallbackCalls++
						checkRequest(in)
						if tc.activeFallback {
							return tc.response, tc.rpcErr
						}
						return tc.fallbackResponse, tc.fallbackErr
					}})
					p.fallback = &node{client: fallback, address: "fallback"}
					if tc.activeFallback {
						p.Failover()
					}
				}
				conn := p.ContractClient()
				if tc.token {
					conn = p.TRC20Client()
				}
				var tx *api.TransactionExtention
				var err error
				const addr = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
				if packed {
					tx, err = conn.TriggerConstantContractWithDataCtx(context.Background(), addr, addr, []byte{0x31, 0x3c, 0xe5, 0x67}, client.WithCallValue(42))
				} else {
					tx, err = conn.TriggerConstantContractCtx(context.Background(), addr, addr, "decimals()", "[]", client.WithCallValue(42))
				}
				if tc.wantErr != "" {
					require.ErrorContains(t, err, tc.wantErr)
					assert.Nil(t, tx)
				} else {
					require.NoError(t, err)
					require.NotNil(t, tx)
					expected := tc.response
					if tc.wantFallbackCalls > 0 {
						expected = tc.fallbackResponse
					}
					assert.True(t, proto.Equal(expected, tx), "constant call response must be preserved")
				}
				assert.Equal(t, tc.wantFallbackCalls, fallbackCalls)
			})
		}
	}
}

func TestContractClient_CanceledContextDoesNotUseFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	primary := newBufconnClient(t, &constantWalletServer{call: func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		cancel()
		return &api.TransactionExtention{Result: &api.Return{Result: true}}, nil
	}})
	p := NewFromClient(primary, "primary")
	p.fallback = &node{client: newBufconnClient(t, &constantWalletServer{call: func(context.Context, *core.TriggerSmartContract) (*api.TransactionExtention, error) {
		t.Error("canceled request must not reach fallback")
		return nil, status.Error(codes.Canceled, "canceled")
	}}), address: "fallback"}
	_, err := p.TRC20Client().TriggerConstantContractWithDataCtx(ctx, "", "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", []byte{0x31, 0x3c, 0xe5, 0x67})
	require.Error(t, err)
}

func TestValidateConstantResult_NilResponse(t *testing.T) {
	err, canFallback := validateConstantResult(nil, true)
	require.ErrorContains(t, err, "missing constant call status")
	assert.True(t, canFallback)
}
