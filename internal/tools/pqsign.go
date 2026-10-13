package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/wallet"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/fbsobreira/gotron-sdk/pkg/client/transaction"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/fbsobreira/gotron-sdk/pkg/pqc"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/fbsobreira/gotron-sdk/pkg/signer"
	"github.com/fbsobreira/gotron-sdk/pkg/txcore"
)

// walletPreflight looks up walletName and runs pqPreflight for a local
// post-quantum wallet. Every signing handler calls it before any policy
// reservation, approval request or confirmation prompt. A wallet that is not
// local is not an error: local is false and the caller keeps its previous
// flow. Any other failure (an ambiguous name, an unreadable key, a failed
// preflight) is returned as the tool error text, prefixed with tool.
func walletPreflight(ctx context.Context, conn *client.GrpcClient, wm *wallet.Manager, tool, walletName string, tx *core.Transaction) (info wallet.WalletInfo, local bool, err error) {
	info, err = wm.LookupWallet(walletName)
	if errors.Is(err, wallet.ErrWalletNotFound) {
		return info, false, nil
	}
	if err != nil {
		return info, false, errors.New(signErrorMessage(tool, err))
	}
	if err := pqPreflight(ctx, conn, info, tx); err != nil {
		return info, true, fmt.Errorf("%s: %w", tool, err)
	}
	return info, true, nil
}

// pqPreflight runs the TIP-899 checks for a post-quantum wallet before any
// policy reservation, approval request or confirmation prompt: it asks the
// node whether the wallet's scheme is enabled, then checks that the wallet
// address is a key of the permission tx uses on the owner account. It
// returns nil at once for a non-PQ wallet.
func pqPreflight(ctx context.Context, conn *client.GrpcClient, info wallet.WalletInfo, tx *core.Transaction) error {
	if !info.KeyType.IsPQ() {
		return nil
	}
	if err := pqProbe(ctx, conn, info.KeyType, tx); err != nil {
		return err
	}
	return pqOwnerCheck(ctx, conn, info, tx)
}

// pqSchemeHint explains why a node can refuse a TIP-899 scheme.
const pqSchemeHint = "TIP-899 is live only on Nile, and governance can switch each scheme off"

// pqProbe asks the node whether it accepts the post-quantum signatures
// already on tx, then signatures of the wallet key type's scheme. Each is
// reported separately, so the error names the scheme the node refuses.
func pqProbe(ctx context.Context, conn *client.GrpcClient, kt wallet.KeyType, tx *core.Transaction) error {
	scheme, ok := kt.PQScheme()
	if !ok {
		return fmt.Errorf("key type %q is not a post-quantum scheme", kt)
	}
	if err := checkSignedPQSchemes(ctx, conn, tx); err != nil {
		return err
	}
	err := txcore.CheckPQSchemes(ctx, conn, &core.Transaction{}, scheme)
	if errors.Is(err, txcore.ErrPQSchemeDisabled) {
		conn.InvalidatePQCapabilities()
		return fmt.Errorf("node does not allow %s signatures (%s): %w", pqDisplayName(kt), pqSchemeHint, err)
	}
	if err != nil {
		return fmt.Errorf("checking node post-quantum support: %w", err)
	}
	return nil
}

// checkSignedPQSchemes asks the node whether it accepts every post-quantum
// signature already on tx. It does not call the node for a transaction
// without post-quantum signatures. A negative answer drops the client's
// cached capabilities, so a governance switch-on is seen on the next call.
func checkSignedPQSchemes(ctx context.Context, conn *client.GrpcClient, tx *core.Transaction) error {
	err := txcore.CheckPQSchemes(ctx, conn, tx)
	if errors.Is(err, txcore.ErrPQSchemeDisabled) {
		conn.InvalidatePQCapabilities()
		return fmt.Errorf("transaction carries a post-quantum signature the node does not accept (%s): %w", pqSchemeHint, err)
	}
	if err != nil {
		return fmt.Errorf("checking node post-quantum support: %w", err)
	}
	return nil
}

// pqOwnerCheck checks that the post-quantum wallet's address is a key of the
// permission tx uses on its owner account, and that the permission allows the
// contract type. On a migrated account the owner stays the old ECDSA address
// and the PQ address is an active-permission key.
func pqOwnerCheck(ctx context.Context, conn *client.GrpcClient, info wallet.WalletInfo, tx *core.Transaction) error {
	owner, permissionID, contractType, err := transaction.OwnerAndPermission(tx)
	if err != nil {
		return err
	}

	acct, err := conn.GetAccountCtx(ctx, owner.String())
	if err != nil {
		if errors.Is(err, client.ErrAccountNotFound) {
			return fmt.Errorf("owner account %s is not on chain yet; a new address must receive TRX before it can send", owner)
		}
		return fmt.Errorf("checking owner account %s: %w", owner, err)
	}

	perm, err := client.SigningPermission(acct, permissionID)
	if err != nil {
		return fmt.Errorf("account %s: %w", owner, err)
	}
	if !client.PermissionAllows(perm, contractType) {
		return fmt.Errorf("permission %d on account %s does not allow %s", permissionID, owner, contractType)
	}
	for _, k := range perm.GetKeys() {
		if address.BytesToAddress(k.GetAddress()).String() == info.Address {
			return nil
		}
	}
	return fmt.Errorf("post-quantum wallet %q (%s) is not a key of permission %d on account %s",
		info.Name, info.Address, permissionID, owner)
}

// signErrorMessage turns a wallet signing error into a tool error message.
// The keystore passphrase never appears in SDK errors, so err is safe to show.
func signErrorMessage(tool string, err error) string {
	switch {
	case errors.Is(err, pqc.ErrFalconUnavailable):
		return fmt.Sprintf("%s: this build cannot sign FN-DSA-512; it needs the gotron-mcp-pq build, or build locally with liboqs installed: CGO_ENABLED=1 go build -tags falcon ./cmd/gotron-mcp: %v", tool, err)
	case errors.Is(err, signer.ErrPQAlreadySigned):
		return fmt.Sprintf("%s: this wallet has already signed the transaction", tool)
	case errors.Is(err, keystore.ErrTooManySignatures):
		return fmt.Sprintf("%s: the transaction already carries the maximum of 5 signatures (ECDSA and post-quantum combined)", tool)
	case errors.Is(err, keystore.ErrDecrypt):
		return fmt.Sprintf("%s: cannot decrypt the wallet: wrong keystore passphrase (%v)", tool, err)
	default:
		return fmt.Sprintf("%s: %v", tool, err)
	}
}

// pqDisplayName returns the human name of a post-quantum key type.
func pqDisplayName(kt wallet.KeyType) string {
	switch kt {
	case wallet.KeyTypeMLDSA44:
		return "ML-DSA-44"
	case wallet.KeyTypeFNDSA512:
		return "FN-DSA-512"
	default:
		return string(kt)
	}
}

// txPermissionID returns the permission id of a transaction's first
// contract, or 0 when it has none.
func txPermissionID(tx *core.Transaction) int32 {
	contracts := tx.GetRawData().GetContract()
	if len(contracts) == 0 {
		return 0
	}
	return contracts[0].GetPermissionId()
}
