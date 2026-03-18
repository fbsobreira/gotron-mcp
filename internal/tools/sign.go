package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-sdk/pkg/keystore"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

const maxHexInputLen = 1 << 20 // 1 MB max hex string input

// RegisterSignTools registers sign_transaction and broadcast_transaction (local mode + keystore only).
func RegisterSignTools(s *server.MCPServer, pool *nodepool.Pool, keystorePath string) {
	s.AddTool(
		mcp.NewTool("sign_transaction",
			mcp.WithDescription("Sign an unsigned transaction using local keystore. WARNING: passphrase is transmitted in cleartext through the MCP protocol. Only use in local (stdio) mode with trusted clients."),
			mcp.WithString("transaction_hex", mcp.Required(), mcp.Description("Unsigned transaction hex")),
			mcp.WithString("signer", mcp.Required(), mcp.Description("Account name or address in keystore")),
			mcp.WithString("passphrase", mcp.Required(), mcp.Description("Keystore passphrase (transmitted in cleartext)")),
		),
		handleSignTransaction(keystorePath),
	)

	s.AddTool(
		mcp.NewTool("broadcast_transaction",
			mcp.WithDescription("Broadcast a signed transaction to the TRON network"),
			mcp.WithString("signed_transaction_hex", mcp.Required(), mcp.Description("Signed transaction hex")),
		),
		handleBroadcastTransaction(pool),
	)
}

func handleSignTransaction(keystorePath string) server.ToolHandlerFunc {
	ks := keystore.NewKeyStore(keystorePath, keystore.StandardScryptN, keystore.StandardScryptP)

	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		txHex := req.GetString("transaction_hex", "")
		signer := req.GetString("signer", "")
		passphrase := req.GetString("passphrase", "")

		if txHex == "" {
			return mcp.NewToolResultError("transaction_hex is required"), nil
		}
		if len(txHex) > maxHexInputLen {
			return mcp.NewToolResultError("transaction_hex exceeds maximum length"), nil
		}
		if signer == "" {
			return mcp.NewToolResultError("signer is required"), nil
		}
		if passphrase == "" {
			return mcp.NewToolResultError("passphrase is required"), nil
		}

		txBytes, err := hex.DecodeString(txHex)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid transaction_hex: %v", err)), nil
		}

		var tx core.Transaction
		if err := proto.Unmarshal(txBytes, &tx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse transaction: %v", err)), nil
		}
		accs := ks.Accounts()

		var found *keystore.Account
		for _, acc := range accs {
			if acc.Address.String() == signer {
				found = &acc
				break
			}
		}

		if found == nil {
			return mcp.NewToolResultError(fmt.Sprintf("account '%s' not found in keystore at %s", signer, keystorePath)), nil
		}

		signedTx, err := ks.SignTxWithPassphrase(*found, passphrase, &tx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to sign transaction: %v", err)), nil
		}

		signedBytes, err := proto.Marshal(signedTx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to serialize signed transaction: %v", err)), nil
		}

		result := map[string]any{
			"signed_transaction_hex": hex.EncodeToString(signedBytes),
			"signer":                 signer,
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleBroadcastTransaction(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		txHex := req.GetString("signed_transaction_hex", "")
		conn := pool.Client()
		if txHex == "" {
			return mcp.NewToolResultError("signed_transaction_hex is required"), nil
		}
		if len(txHex) > maxHexInputLen {
			return mcp.NewToolResultError("signed_transaction_hex exceeds maximum length"), nil
		}

		txBytes, err := hex.DecodeString(txHex)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid signed_transaction_hex: %v", err)), nil
		}

		var tx core.Transaction
		if err := proto.Unmarshal(txBytes, &tx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse transaction: %v", err)), nil
		}

		ret, err := conn.BroadcastCtx(ctx, &tx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("broadcast_transaction: %v", err)), nil
		}

		rawData, err := proto.Marshal(tx.RawData)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to compute txid: %v", err)), nil
		}
		txHash := sha256.Sum256(rawData)

		result := map[string]any{
			"transaction_id": hex.EncodeToString(txHash[:]),
			"success":        ret.Result,
			"code":           ret.Code.String(),
			"message":        string(ret.Message),
		}

		return mcp.NewToolResultJSON(result)
	}
}
