package tools

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"

	"math/big"

	"github.com/fbsobreira/gotron-mcp/internal/nodepool"
	"github.com/fbsobreira/gotron-mcp/internal/retry"
	"github.com/fbsobreira/gotron-sdk/pkg/abi"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

// RegisterContractReadTools registers get_contract_abi, estimate_energy, and trigger_constant_contract.
func RegisterContractReadTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("trigger_constant_contract",
			mcp.WithDescription("Call a read-only (view/pure) smart contract method. No transaction created, no fees. Returns decoded result."),
			mcp.WithString("from", mcp.Description("Caller address (optional, defaults to zero address)")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Contract address")),
			mcp.WithString("method", mcp.Required(), mcp.Description("Method signature (e.g., 'totalSupply()', 'balanceOf(address)')")),
			mcp.WithString("params", mcp.Description("Method parameters as JSON string (optional)")),
		),
		handleTriggerConstantContract(pool),
	)

	s.AddTool(
		mcp.NewTool("get_contract_abi",
			mcp.WithDescription("Get the ABI of a smart contract on TRON. Automatically resolves proxy contracts (ERC-1967)."),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Smart contract address")),
		),
		handleGetContractABI(pool),
	)

	s.AddTool(
		mcp.NewTool("list_contract_methods",
			mcp.WithDescription("Get a human-readable summary of a smart contract's methods with signatures, inputs, outputs, and mutability. Auto-resolves proxy contracts."),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Smart contract address")),
		),
		handleListContractMethods(pool),
	)

	s.AddTool(
		mcp.NewTool("decode_abi_output",
			mcp.WithDescription("Decode ABI-encoded output hex from a contract call. Handles return values, revert reasons (Error(string)), and panic codes (Panic(uint256))."),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Contract address (needed to fetch ABI for decoding)")),
			mcp.WithString("method", mcp.Required(), mcp.Description("Method signature (e.g., 'balanceOf(address)')")),
			mcp.WithString("data", mcp.Required(), mcp.Description("Hex-encoded output bytes to decode")),
		),
		handleDecodeABIOutput(pool),
	)

	s.AddTool(
		mcp.NewTool("estimate_energy",
			mcp.WithDescription("Estimate energy cost for a smart contract call"),
			mcp.WithString("from", mcp.Required(), mcp.Description("Caller address")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Contract address")),
			mcp.WithString("method", mcp.Required(), mcp.Description("Contract method signature (e.g., 'transfer(address,uint256)')")),
			mcp.WithString("params", mcp.Required(), mcp.Description("Method parameters as JSON string")),
		),
		handleEstimateEnergy(pool),
	)
}

// RegisterContractWriteTools registers trigger_contract (local mode only).
func RegisterContractWriteTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("trigger_contract",
			mcp.WithDescription("Call a smart contract method. Returns unsigned transaction hex."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Caller address")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Contract address")),
			mcp.WithString("method", mcp.Required(), mcp.Description("Method signature (e.g., 'transfer(address,uint256)')")),
			mcp.WithString("params", mcp.Required(), mcp.Description("Method parameters as JSON string")),
			mcp.WithNumber("fee_limit", mcp.Description("Fee limit in TRX (default: 100)")),
			mcp.WithNumber("call_value", mcp.Description("Amount to send with call in SUN (default: 0)")),
		),
		handleTriggerContract(pool),
	)
}

func handleListContractMethods(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		contract := req.GetString("contract_address", "")
		conn := pool.Client()
		if err := validateAddress(contract); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		contractABI, err := conn.GetContractABIResolvedCtx(ctx, contract)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list_contract_methods: %v", err)), nil
		}

		formatted := abi.FormatABI(contractABI)

		// Add human-readable signatures for functions
		for i, entry := range contractABI.Entrys {
			if entry.Type == 2 { // Function
				inputTypes := make([]string, len(entry.Inputs))
				for j, p := range entry.Inputs {
					inputTypes[j] = p.Type
				}
				outputTypes := make([]string, len(entry.Outputs))
				for j, p := range entry.Outputs {
					outputTypes[j] = p.Type
				}
				sig := fmt.Sprintf("%s(%s)", entry.Name, strings.Join(inputTypes, ","))
				if len(outputTypes) > 0 {
					sig += fmt.Sprintf(" → (%s)", strings.Join(outputTypes, ","))
				}
				mutability, _ := formatted[i]["stateMutability"].(string)
				if mutability != "" {
					sig += fmt.Sprintf(" [%s]", mutability)
				}
				formatted[i]["signature"] = sig
			}
		}

		result := map[string]any{
			"contract_address": contract,
			"methods":          formatted,
			"count":            len(formatted),
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleDecodeABIOutput(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		contract := req.GetString("contract_address", "")
		method := req.GetString("method", "")
		dataHex := req.GetString("data", "")

		if err := validateAddress(contract); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		dataHex = strings.TrimPrefix(dataHex, "0x")
		if len(dataHex) > 1<<20 {
			return mcp.NewToolResultError("decode_abi_output: data exceeds maximum length"), nil
		}
		data, err := hex.DecodeString(dataHex)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decode_abi_output: invalid hex data: %v", err)), nil
		}
		if len(data) == 0 {
			return mcp.NewToolResultError("decode_abi_output: data is empty"), nil
		}

		result := map[string]any{
			"contract_address": contract,
			"method":           method,
		}

		// Check for revert reason first (Error(string) or Panic(uint256))
		if reason, decErr := abi.DecodeRevertReason(data); decErr == nil {
			result["revert_reason"] = reason
			result["reverted"] = true
			return mcp.NewToolResultJSON(result)
		}

		// Decode output using contract ABI
		conn := pool.Client()
		contractABI, err := conn.GetContractABIResolvedCtx(ctx, contract)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decode_abi_output: failed to fetch ABI: %v", err)), nil
		}

		decoded, err := abi.DecodeOutput(contractABI, method, data)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decode_abi_output: %v", err)), nil
		}

		result["decoded"] = stringifyDecoded(decoded)
		return mcp.NewToolResultJSON(result)
	}
}

func handleGetContractABI(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		contract := req.GetString("contract_address", "")
		conn := pool.Client()
		if err := validateAddress(contract); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		contractABI, err := conn.GetContractABIResolvedCtx(ctx, contract)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_contract_abi: %v", err)), nil
		}

		formatted := abi.FormatABI(contractABI)

		result := map[string]any{
			"contract_address": contract,
			"abi":              formatted,
		}

		if len(formatted) == 0 {
			result["note"] = "No ABI found on-chain. This contract may not have its ABI published. Check TronScan for verified source code and ABI."
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleEstimateEnergy(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		contract := req.GetString("contract_address", "")
		method := req.GetString("method", "")
		params := req.GetString("params", "")

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}
		if err := validateAddress(contract); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		estimate, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (*api.EstimateEnergyMessage, error) {
			return pool.Client().EstimateEnergyCtx(ctx, from, contract, method, params, 0, "", 0)
		})
		if err != nil && isEstimateEnergyUnsupported(err) {
			// Active node doesn't support EstimateEnergy RPC — try fallback
			if fallback := pool.FallbackClient(); fallback != nil {
				log.Printf("estimate_energy: trying fallback node")
				estimate, err = fallback.EstimateEnergyCtx(ctx, from, contract, method, params, 0, "", 0)
			}
		}
		if err != nil {
			if isEstimateEnergyUnsupported(err) {
				return mcp.NewToolResultError("estimate_energy: no available node supports energy estimation. Ensure at least one node has vm.estimateEnergy=true and vm.supportConstant=true configured."), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("estimate_energy: %v", err)), nil
		}

		result := map[string]any{
			"from":             from,
			"contract_address": contract,
			"method":           method,
			"estimated_energy": estimate.EnergyRequired,
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleTriggerConstantContract(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb") // zero address default
		contract := req.GetString("contract_address", "")
		method := req.GetString("method", "")
		params := req.GetString("params", "[]")
		conn := pool.Client()

		if from != "" {
			if err := validateAddress(from); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
			}
		}
		if err := validateAddress(contract); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		tx, err := conn.TriggerConstantContractCtx(ctx, from, contract, method, params)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("trigger_constant_contract: %v", err)), nil
		}

		result := map[string]any{
			"contract_address": contract,
			"method":           method,
		}

		if len(tx.ConstantResult) > 0 {
			rawResult := tx.ConstantResult[0]
			result["result_hex"] = hex.EncodeToString(rawResult)

			// Try to decode the result using ABI if available
			contractABI, abiErr := conn.GetContractABIResolvedCtx(ctx, contract)
			if abiErr == nil && contractABI != nil {
				decoded, decErr := abi.DecodeOutput(contractABI, method, rawResult)
				if decErr == nil && decoded != nil {
					result["result_decoded"] = stringifyDecoded(decoded)
				}
			}

			// Check for revert reason
			if reason, decErr := abi.DecodeRevertReason(rawResult); decErr == nil {
				result["revert_reason"] = reason
			}
		}

		if tx.Result != nil {
			result["energy_used"] = tx.EnergyUsed
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleTriggerContract(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		contract := req.GetString("contract_address", "")
		method := req.GetString("method", "")
		params := req.GetString("params", "")
		feeLimit := req.GetInt("fee_limit", 100)
		callValue := req.GetInt("call_value", 0)
		conn := pool.Client()

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}
		if err := validateAddress(contract); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		if feeLimit < 0 || feeLimit > 15000 {
			return mcp.NewToolResultError("fee_limit must be between 0 and 15000 TRX"), nil
		}
		feeLimitSun := int64(feeLimit) * 1_000_000

		tx, err := conn.TriggerContractCtx(ctx, from, contract, method, params, feeLimitSun, int64(callValue), "", 0)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("trigger_contract: %v", err)), nil
		}

		txBytes, err := proto.Marshal(tx.Transaction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("trigger_contract: failed to serialize: %v", err)), nil
		}

		result := map[string]any{
			"transaction_hex":  hex.EncodeToString(txBytes),
			"txid":             hex.EncodeToString(tx.Txid),
			"from":             from,
			"contract_address": contract,
			"method":           method,
			"fee_limit_trx":    feeLimit,
			"type":             "TriggerSmartContract",
		}

		if len(tx.ConstantResult) > 0 {
			result["constant_result"] = hex.EncodeToString(tx.ConstantResult[0])
		}

		return mcp.NewToolResultJSON(result)
	}
}

// stringifyDecoded converts decoded ABI output values to JSON-safe types.
// address.Address ([]byte) → base58 string, *big.Int → string, slices recursed.
func stringifyDecoded(values []interface{}) []interface{} {
	out := make([]interface{}, len(values))
	for i, v := range values {
		out[i] = stringifyValue(v)
	}
	return out
}

func stringifyValue(v interface{}) interface{} {
	switch val := v.(type) {
	case address.Address:
		return val.String()
	case []address.Address:
		result := make([]string, len(val))
		for i, a := range val {
			result[i] = a.String()
		}
		return result
	case *big.Int:
		if val == nil {
			return "0"
		}
		return val.String()
	case []interface{}:
		return stringifyDecoded(val)
	default:
		return v
	}
}

func isEstimateEnergyUnsupported(err error) bool {
	if errors.Is(err, client.ErrEstimateEnergyNotSupported) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "does not support estimate energy")
}
