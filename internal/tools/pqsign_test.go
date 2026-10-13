package tools

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fbsobreira/gotron-mcp/internal/approval"
	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/fbsobreira/gotron-sdk/pkg/pqc"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/fbsobreira/gotron-sdk/pkg/signer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// testAddr returns a valid base58 TRON address built from a repeated byte.
func testAddr(b byte) string {
	return address.BytesToAddress(bytes.Repeat([]byte{b}, 20)).String()
}

// buildTransferTx builds a TransferContract transaction from owner to to,
// using the given permission id and an expiration one minute ahead.
func buildTransferTx(t *testing.T, owner, to string, amountSUN int64, permissionID int32) *core.Transaction {
	t.Helper()
	param, err := anypb.New(&core.TransferContract{
		OwnerAddress: mustDecodeAddr(owner),
		ToAddress:    mustDecodeAddr(to),
		Amount:       amountSUN,
	})
	require.NoError(t, err)
	return &core.Transaction{
		RawData: &core.TransactionRaw{
			Contract: []*core.Transaction_Contract{{
				Type:         core.Transaction_Contract_TransferContract,
				Parameter:    param,
				PermissionId: permissionID,
			}},
			Expiration: time.Now().Add(time.Minute).UnixMilli(),
		},
	}
}

// buildTransferTxHexPerm is buildTransferTx encoded as hex.
func buildTransferTxHexPerm(t *testing.T, owner, to string, amountSUN int64, permissionID int32) string {
	t.Helper()
	b, err := proto.Marshal(buildTransferTx(t, owner, to, amountSUN, permissionID))
	require.NoError(t, err)
	return hex.EncodeToString(b)
}

// transferOnlyOps is an active permission operations bitmap that allows
// TransferContract (contract type 1) and nothing else.
var transferOnlyOps = func() []byte {
	ops := make([]byte, 32)
	ops[0] = 1 << byte(core.Transaction_Contract_TransferContract)
	return ops
}()

// testPermission builds a permission with the given id and key addresses.
// Active permissions allow TransferContract only.
func testPermission(id int32, keys ...string) *core.Permission {
	p := &core.Permission{Id: id, Threshold: 1}
	switch id {
	case 0:
		p.Type = core.Permission_Owner
	case 1:
		p.Type = core.Permission_Witness
	default:
		p.Type = core.Permission_Active
		p.Operations = transferOnlyOps
	}
	for _, k := range keys {
		p.Keys = append(p.Keys, &core.Key{Address: mustDecodeAddr(k), Weight: 1})
	}
	return p
}

// withPQSig returns tx with a pq_auth_sig entry of scheme appended. Only the
// scheme is set: the node probe reads nothing else.
func withPQSig(tx *core.Transaction, scheme core.PQScheme) *core.Transaction {
	tx.PqAuthSig = append(tx.PqAuthSig, &core.PQAuthSig{Scheme: scheme})
	return tx
}

// testAccount builds an on-chain account for owner with an optional owner
// permission and any number of active permissions.
func testAccount(owner string, ownerPerm *core.Permission, active ...*core.Permission) *core.Account {
	return &core.Account{
		Address:          mustDecodeAddr(owner),
		OwnerPermission:  ownerPerm,
		ActivePermission: active,
	}
}

// chainParams returns a GetChainParameters mock reporting the given
// key/value pairs.
func chainParams(kv map[string]int64) func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
	return func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
		cp := &core.ChainParameters{}
		for k, v := range kv {
			cp.ChainParameter = append(cp.ChainParameter, &core.ChainParameters_ChainParameter{Key: k, Value: v})
		}
		return cp, nil
	}
}

// mlDsaParams is a chain-parameter mock with getAllowMlDsa44 set to v.
func mlDsaParams(v int64) func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
	return chainParams(map[string]int64{"getAllowMlDsa44": v})
}

// accountFor returns a GetAccount mock serving accounts by base58 address.
// Unknown addresses get an empty account, which the SDK reports as
// ErrAccountNotFound.
func accountFor(accts ...*core.Account) func(context.Context, *core.Account) (*core.Account, error) {
	return func(_ context.Context, in *core.Account) (*core.Account, error) {
		for _, a := range accts {
			if bytes.Equal(a.Address, in.Address) {
				return a, nil
			}
		}
		return &core.Account{}, nil
	}
}

// countingApprover records how many approval requests it received.
type countingApprover struct {
	approved bool
	calls    atomic.Int32
}

func (c *countingApprover) RequestApproval(_ context.Context, _ approval.Request) (approval.Result, error) {
	c.calls.Add(1)
	return approval.Result{Approved: c.approved}, nil
}

func TestSignErrorMessage(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    string
		notWant string
	}{
		{
			name: "falcon unavailable",
			err:  fmt.Errorf("sign: %w", pqc.ErrFalconUnavailable),
			want: "sign_and_broadcast: this build cannot sign FN-DSA-512; it needs the gotron-mcp-pq build, or build locally with liboqs installed: CGO_ENABLED=1 go build -tags falcon ./cmd/gotron-mcp: sign: " + pqc.ErrFalconUnavailable.Error(),
		},
		{
			name: "already signed",
			err:  fmt.Errorf("sign: %w", signer.ErrPQAlreadySigned),
			want: "sign_and_broadcast: this wallet has already signed the transaction",
		},
		{
			name: "too many signatures",
			err:  fmt.Errorf("sign: %w", keystore.ErrTooManySignatures),
			want: "sign_and_broadcast: the transaction already carries the maximum of 5 signatures (ECDSA and post-quantum combined)",
		},
		{
			name: "decrypt keeps the cause",
			err:  fmt.Errorf("decrypt post-quantum wallet %q: %w", "w", keystore.ErrDecrypt),
			want: `sign_and_broadcast: cannot decrypt the wallet: wrong keystore passphrase (decrypt post-quantum wallet "w": ` + keystore.ErrDecrypt.Error() + ")",
		},
		{
			name: "default",
			err:  errors.New(`wallet "x" not found`),
			want: `sign_and_broadcast: wallet "x" not found`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := signErrorMessage("sign_and_broadcast", tt.err)
			assert.Equal(t, tt.want, got)
			if tt.notWant != "" {
				assert.NotContains(t, got, tt.notWant)
			}
		})
	}
}

func TestPQPreflight(t *testing.T) {
	pqAddr := testAddr(0x51)    // the PQ wallet's own address
	ecdsaAddr := testAddr(0x52) // a migrated account's ECDSA owner
	otherAddr := testAddr(0x53)
	to := testAddr(0x54)
	pqInfo := wallet.WalletInfo{Name: "pq", Address: pqAddr, KeyType: wallet.KeyTypeMLDSA44}

	tests := []struct {
		name        string
		info        wallet.WalletInfo
		params      func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error)
		account     func(context.Context, *core.Account) (*core.Account, error)
		tx          *core.Transaction
		wantErr     string
		wantAccount bool // GetAccount must be called
	}{
		{
			name:        "allowed, owner is the wallet with default owner permission",
			info:        pqInfo,
			params:      mlDsaParams(1),
			account:     accountFor(testAccount(pqAddr, nil)),
			tx:          buildTransferTx(t, pqAddr, to, 1, 0),
			wantAccount: true,
		},
		{
			name:        "allowed, wallet in explicit owner permission",
			info:        pqInfo,
			params:      mlDsaParams(1),
			account:     accountFor(testAccount(ecdsaAddr, testPermission(0, ecdsaAddr, pqAddr))),
			tx:          buildTransferTx(t, ecdsaAddr, to, 1, 0),
			wantAccount: true,
		},
		{
			name:    "scheme disabled",
			info:    pqInfo,
			params:  mlDsaParams(0),
			tx:      buildTransferTx(t, pqAddr, to, 1, 0),
			wantErr: "node does not allow ML-DSA-44 signatures (TIP-899 is live only on Nile",
		},
		{
			name:    "mainnet reports no TIP-899 keys",
			info:    pqInfo,
			params:  chainParams(map[string]int64{"getEnergyFee": 420}),
			tx:      buildTransferTx(t, pqAddr, to, 1, 0),
			wantErr: "node does not allow ML-DSA-44",
		},
		{
			name: "only the other scheme enabled",
			info: pqInfo,
			params: chainParams(map[string]int64{
				"getAllowFnDsa512": 1, "getAllowMlDsa44": 0,
			}),
			tx:      buildTransferTx(t, pqAddr, to, 1, 0),
			wantErr: "node does not allow ML-DSA-44",
		},
		{
			name: "co-signature scheme disabled, wallet scheme enabled",
			info: pqInfo,
			params: chainParams(map[string]int64{
				"getAllowFnDsa512": 0, "getAllowMlDsa44": 1,
			}),
			tx:      withPQSig(buildTransferTx(t, pqAddr, to, 1, 0), core.PQScheme_FN_DSA_512),
			wantErr: "transaction carries a post-quantum signature the node does not accept (TIP-899 is live only on Nile, and governance can switch each scheme off): post-quantum signature scheme disabled on node: FN_DSA_512",
		},
		{
			name:        "co-signature scheme enabled",
			info:        pqInfo,
			params:      chainParams(map[string]int64{"getAllowFnDsa512": 1, "getAllowMlDsa44": 1}),
			account:     accountFor(testAccount(pqAddr, nil)),
			tx:          withPQSig(buildTransferTx(t, pqAddr, to, 1, 0), core.PQScheme_FN_DSA_512),
			wantAccount: true,
		},
		{
			name: "probe RPC error",
			info: pqInfo,
			params: func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
				return nil, status.Error(codes.Unavailable, "node down")
			},
			tx:      buildTransferTx(t, pqAddr, to, 1, 0),
			wantErr: "checking node post-quantum support",
		},
		{
			name:        "migrated account, PQ key in active permission 2, tx uses 2",
			info:        pqInfo,
			params:      mlDsaParams(1),
			account:     accountFor(testAccount(ecdsaAddr, testPermission(0, ecdsaAddr), testPermission(2, pqAddr))),
			tx:          buildTransferTx(t, ecdsaAddr, to, 1, 2),
			wantAccount: true,
		},
		{
			name:        "migrated account, tx uses owner permission without the PQ key",
			info:        pqInfo,
			params:      mlDsaParams(1),
			account:     accountFor(testAccount(ecdsaAddr, testPermission(0, ecdsaAddr), testPermission(2, pqAddr))),
			tx:          buildTransferTx(t, ecdsaAddr, to, 1, 0),
			wantErr:     fmt.Sprintf("post-quantum wallet %q (%s) is not a key of permission 0 on account %s", "pq", pqAddr, ecdsaAddr),
			wantAccount: true,
		},
		{
			name:        "default owner permission of another address",
			info:        pqInfo,
			params:      mlDsaParams(1),
			account:     accountFor(testAccount(ecdsaAddr, nil)),
			tx:          buildTransferTx(t, ecdsaAddr, to, 1, 0),
			wantErr:     "is not a key of permission 0",
			wantAccount: true,
		},
		{
			name:        "PQ address not in the active permission keys",
			info:        pqInfo,
			params:      mlDsaParams(1),
			account:     accountFor(testAccount(ecdsaAddr, testPermission(0, ecdsaAddr), testPermission(2, otherAddr))),
			tx:          buildTransferTx(t, ecdsaAddr, to, 1, 2),
			wantErr:     "is not a key of permission 2",
			wantAccount: true,
		},
		{
			name:        "permission id missing",
			info:        pqInfo,
			params:      mlDsaParams(1),
			account:     accountFor(testAccount(ecdsaAddr, testPermission(0, ecdsaAddr), testPermission(2, pqAddr))),
			tx:          buildTransferTx(t, ecdsaAddr, to, 1, 3),
			wantErr:     fmt.Sprintf("account %s: signing permission: id 3: no active permission with that id", ecdsaAddr),
			wantAccount: true,
		},
		{
			name:   "active permission without an operations bitmap fails closed",
			info:   pqInfo,
			params: mlDsaParams(1),
			account: accountFor(testAccount(ecdsaAddr, testPermission(0, ecdsaAddr), &core.Permission{
				Type: core.Permission_Active, Id: 2, Threshold: 1,
				Keys: []*core.Key{{Address: mustDecodeAddr(pqAddr), Weight: 1}},
			})),
			tx:          buildTransferTx(t, ecdsaAddr, to, 1, 2),
			wantErr:     fmt.Sprintf("permission 2 on account %s does not allow TransferContract", ecdsaAddr),
			wantAccount: true,
		},
		{
			name:   "active permission does not allow the contract type",
			info:   pqInfo,
			params: mlDsaParams(1),
			account: accountFor(testAccount(ecdsaAddr, testPermission(0, ecdsaAddr), &core.Permission{
				Type: core.Permission_Active, Id: 2, Threshold: 1, Operations: make([]byte, 32),
				Keys: []*core.Key{{Address: mustDecodeAddr(pqAddr), Weight: 1}},
			})),
			tx:          buildTransferTx(t, ecdsaAddr, to, 1, 2),
			wantErr:     fmt.Sprintf("permission 2 on account %s does not allow TransferContract", ecdsaAddr),
			wantAccount: true,
		},
		{
			name:        "witness permission cannot sign, even for the owner key",
			info:        pqInfo,
			params:      mlDsaParams(1),
			account:     accountFor(testAccount(pqAddr, nil)),
			tx:          buildTransferTx(t, pqAddr, to, 1, 1),
			wantErr:     "signing permission: id 1: witness permission: cannot sign a transaction",
			wantAccount: true,
		},
		{
			name:        "witness permission set still cannot sign",
			info:        pqInfo,
			params:      mlDsaParams(1),
			account:     accountFor(&core.Account{Address: mustDecodeAddr(pqAddr), WitnessPermission: testPermission(1, pqAddr)}),
			tx:          buildTransferTx(t, pqAddr, to, 1, 1),
			wantErr:     "signing permission: id 1: witness permission: cannot sign a transaction",
			wantAccount: true,
		},
		{
			name:        "owner account not found",
			info:        pqInfo,
			params:      mlDsaParams(1),
			account:     accountFor(),
			tx:          buildTransferTx(t, pqAddr, to, 1, 0),
			wantErr:     fmt.Sprintf("owner account %s is not on chain yet; a new address must receive TRX before it can send", pqAddr),
			wantAccount: true,
		},
		{
			name:   "GetAccount RPC error",
			info:   pqInfo,
			params: mlDsaParams(1),
			account: func(context.Context, *core.Account) (*core.Account, error) {
				return nil, status.Error(codes.Internal, "boom")
			},
			tx:          buildTransferTx(t, pqAddr, to, 1, 0),
			wantErr:     "checking owner account",
			wantAccount: true,
		},
		{
			name:        "tx without owner fails closed",
			info:        pqInfo,
			params:      mlDsaParams(1),
			tx:          &core.Transaction{RawData: &core.TransactionRaw{Contract: []*core.Transaction_Contract{{}}}},
			wantErr:     "no parameter",
			wantAccount: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var accountCalls atomic.Int32
			mock := &mockWalletServer{
				GetChainParametersFunc: tt.params,
				GetAccountFunc: func(ctx context.Context, in *core.Account) (*core.Account, error) {
					accountCalls.Add(1)
					if tt.account == nil {
						t.Errorf("GetAccount must not be called")
						return nil, status.Error(codes.Internal, "unexpected")
					}
					return tt.account(ctx, in)
				},
			}
			conn := newMockClient(t, mock)

			err := pqPreflight(context.Background(), conn, tt.info, tt.tx)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantAccount, accountCalls.Load() > 0, "GetAccount called")
		})
	}
}

func TestPQPreflight_ECDSASkipsNode(t *testing.T) {
	mock := &mockWalletServer{
		GetChainParametersFunc: func(context.Context, *api.EmptyMessage) (*core.ChainParameters, error) {
			t.Errorf("GetChainParameters must not be called for an ECDSA wallet")
			return nil, status.Error(codes.Internal, "unexpected")
		},
		GetAccountFunc: func(context.Context, *core.Account) (*core.Account, error) {
			t.Errorf("GetAccount must not be called for an ECDSA wallet")
			return nil, status.Error(codes.Internal, "unexpected")
		},
	}
	conn := newMockClient(t, mock)
	info := wallet.WalletInfo{Name: "e", Address: testAddr(0x61), KeyType: wallet.KeyTypeECDSA}
	require.NoError(t, pqPreflight(context.Background(), conn, info, &core.Transaction{}))
}

func TestPQPreflight_DisabledInvalidatesCache(t *testing.T) {
	pqAddr := testAddr(0x71)
	var enabled atomic.Int64
	var probes atomic.Int32
	mock := &mockWalletServer{
		GetChainParametersFunc: func(ctx context.Context, in *api.EmptyMessage) (*core.ChainParameters, error) {
			probes.Add(1)
			return mlDsaParams(enabled.Load())(ctx, in)
		},
		GetAccountFunc: accountFor(testAccount(pqAddr, nil)),
	}
	conn := newMockClient(t, mock)
	info := wallet.WalletInfo{Name: "pq", Address: pqAddr, KeyType: wallet.KeyTypeMLDSA44}
	tx := buildTransferTx(t, pqAddr, testAddr(0x72), 1, 0)

	err := pqPreflight(context.Background(), conn, info, tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "node does not allow ML-DSA-44")

	// Governance switches the scheme on: the negative answer must not be
	// served from the client's 1-minute cache.
	enabled.Store(1)
	require.NoError(t, pqPreflight(context.Background(), conn, info, tx))
	assert.Equal(t, int32(2), probes.Load(), "second call must ask the node again")
}
