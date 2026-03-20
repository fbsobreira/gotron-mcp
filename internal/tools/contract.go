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
	"github.com/fbsobreira/gotron-sdk/pkg/contract"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

// RegisterContractReadTools registers get_contract_abi, estimate_energy, and trigger_constant_contract.
func RegisterContractReadTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("trigger_constant_contract",
			mcp.WithDescription("Call a read-only (view/pure) smart contract method. No transaction created, no fees. Returns decoded result. Provide either method+params OR data (pre-packed calldata)."),
			mcp.WithString("from", mcp.Description("Caller address (base58, starts with T; optional — defaults to zero address)")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Smart contract address (base58, starts with T)")),
			mcp.WithString("method", mcp.Description("Method signature (e.g., 'totalSupply()', 'balanceOf(address)'). Required unless data is provided.")),
			mcp.WithString("params", mcp.Description("Method parameters as JSON array. Plain values: [\"TJD...\", \"1000\"] (types inferred from method signature). Typed objects also accepted: [{\"address\": \"TJD...\"}, {\"uint256\": \"1000\"}]")),
			mcp.WithString("data", mcp.Description("Pre-packed ABI calldata as hex (0x prefix optional). When provided, method and params are ignored.")),
			mcp.WithNumber("call_value", mcp.Description("Amount in SUN to send with call (default: 0, 1 TRX = 1000000 SUN). Required for simulating payable functions.")),
			mcp.WithString("token_id", mcp.Description("TRC10 token ID for simulating TRC10 token transfers")),
			mcp.WithNumber("token_value", mcp.Description("TRC10 token amount to send with call (default: 0)")),
		),
		handleTriggerConstantContract(pool),
	)

	s.AddTool(
		mcp.NewTool("get_contract_abi",
			mcp.WithDescription("Get the ABI of a smart contract on TRON. Automatically resolves proxy contracts (ERC-1967)."),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Smart contract address (base58, starts with T)")),
		),
		handleGetContractABI(pool),
	)

	s.AddTool(
		mcp.NewTool("list_contract_methods",
			mcp.WithDescription("Get a human-readable summary of a smart contract's methods with signatures, inputs, outputs, and mutability. Auto-resolves proxy contracts."),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Smart contract address (base58, starts with T)")),
		),
		handleListContractMethods(pool),
	)

	s.AddTool(
		mcp.NewTool("decode_abi_output",
			mcp.WithDescription("Decode ABI-encoded output hex from a contract call. Handles return values, revert reasons (Error(string)), and panic codes (Panic(uint256))."),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Contract address (base58, starts with T; needed to fetch ABI for decoding)")),
			mcp.WithString("method", mcp.Required(), mcp.Description("Method signature (e.g., 'balanceOf(address)')")),
			mcp.WithString("data", mcp.Required(), mcp.Description("Hex-encoded output bytes to decode (0x prefix optional)")),
		),
		handleDecodeABIOutput(pool),
	)

	s.AddTool(
		mcp.NewTool("estimate_energy",
			mcp.WithDescription("Estimate energy cost for a smart contract call. Requires a full node with vm.supportConstant=true and vm.estimateEnergy=true; falls back to secondary node if primary does not support it."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Caller address (base58, starts with T)")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Smart contract address (base58, starts with T)")),
			mcp.WithString("method", mcp.Required(), mcp.Description("Contract method signature (e.g., 'transfer(address,uint256)')")),
			mcp.WithString("params", mcp.Required(), mcp.Description("Method parameters as JSON array. Plain values: [\"TJD...\", \"1000\"] (types inferred from method signature). Typed objects also accepted: [{\"address\": \"TJD...\"}, {\"uint256\": \"1000\"}]")),
		),
		handleEstimateEnergy(pool),
	)
}

// RegisterContractWriteTools registers trigger_contract (local mode only).
func RegisterContractWriteTools(s *server.MCPServer, pool *nodepool.Pool) {
	s.AddTool(
		mcp.NewTool("trigger_contract",
			mcp.WithDescription("Call a smart contract method. Returns unsigned transaction hex for signing. Provide either method+params OR data (pre-packed calldata)."),
			mcp.WithString("from", mcp.Required(), mcp.Description("Caller address (base58, starts with T)")),
			mcp.WithString("contract_address", mcp.Required(), mcp.Description("Smart contract address (base58, starts with T)")),
			mcp.WithString("method", mcp.Description("Method signature (e.g., 'transfer(address,uint256)'). Required unless data is provided.")),
			mcp.WithString("params", mcp.Description("Method parameters as JSON array. Plain values: [\"TJD...\", \"1000\"] (types inferred from method signature). Typed objects also accepted: [{\"address\": \"TJD...\"}, {\"uint256\": \"1000\"}]")),
			mcp.WithString("data", mcp.Description("Pre-packed ABI calldata as hex (0x prefix optional). When provided, method and params are ignored.")),
			mcp.WithNumber("fee_limit", mcp.Description("Fee limit in whole TRX (integer), range 0-15000 (default: 100)")),
			mcp.WithNumber("call_value", mcp.Description("Amount to send with call in SUN (default: 0)")),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID for multi-sig transactions")),
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

		dataHex = stripHexPrefix(dataHex)
		if len(dataHex) > maxCalldataHexLen {
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
		contractAddr := req.GetString("contract_address", "")
		method := req.GetString("method", "")
		params := req.GetString("params", "")

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}
		if err := validateAddress(contractAddr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		newCall := func(c contract.Client) *contract.ContractCall {
			return contract.New(c, contractAddr).From(from).Method(method).Params(params)
		}

		energy, err := retry.DoWithFailover(ctx, pool, func(ctx context.Context) (int64, error) {
			return newCall(pool.Client()).EstimateEnergy(ctx)
		})
		if err != nil && isEstimateEnergyUnsupported(err) {
			// Active node doesn't support EstimateEnergy RPC — try fallback
			if fallback := pool.FallbackClient(); fallback != nil {
				log.Printf("estimate_energy: trying fallback node")
				energy, err = newCall(fallback).EstimateEnergy(ctx)
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
			"contract_address": contractAddr,
			"method":           method,
			"estimated_energy": energy,
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleTriggerConstantContract(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		contractAddr := req.GetString("contract_address", "")
		method := req.GetString("method", "")
		params := req.GetString("params", "[]")
		dataHex := req.GetString("data", "")
		callValue := int64(req.GetInt("call_value", 0))
		tokenID := req.GetString("token_id", "")
		tokenValue := int64(req.GetInt("token_value", 0))
		args := req.GetArguments()
		_, hasTokenValue := args["token_value"]
		conn := pool.Client()

		if from != "" {
			if err := validateAddress(from); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
			}
		}
		if err := validateAddress(contractAddr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		// Validate value params
		if callValue < 0 {
			return mcp.NewToolResultError("trigger_constant_contract: call_value must be non-negative"), nil
		}
		if tokenValue < 0 {
			return mcp.NewToolResultError("trigger_constant_contract: token_value must be non-negative"), nil
		}
		if (tokenID != "") != hasTokenValue {
			return mcp.NewToolResultError("trigger_constant_contract: token_id and token_value must both be provided together"), nil
		}

		call := contract.New(conn, contractAddr).From(from)
		if callValue > 0 {
			call = call.WithCallValue(callValue)
		}
		if tokenID != "" && hasTokenValue {
			call = call.WithTokenValue(tokenID, tokenValue)
		}

		if dataHex != "" {
			// Pre-packed calldata mode — ignore method entirely
			method = ""
			dataHex = stripHexPrefix(dataHex)
			if len(dataHex) > maxCalldataHexLen {
				return mcp.NewToolResultError("trigger_constant_contract: data exceeds maximum length"), nil
			}
			data, decErr := hex.DecodeString(dataHex)
			if decErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("trigger_constant_contract: invalid data hex: %v", decErr)), nil
			}
			call = call.WithData(data)
		} else {
			if method == "" {
				return mcp.NewToolResultError("trigger_constant_contract: method is required when data is not provided"), nil
			}
			call = call.Method(method).Params(params)
		}

		callResult, err := call.Call(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("trigger_constant_contract: %v", err)), nil
		}

		result := map[string]any{
			"contract_address": contractAddr,
		}
		if method != "" {
			result["method"] = method
		}

		if len(callResult.RawResults) > 0 {
			rawResult := callResult.RawResults[0]
			result["result_hex"] = hex.EncodeToString(rawResult)

			// Try to decode the result using ABI if available
			if method != "" {
				contractABI, abiErr := conn.GetContractABIResolvedCtx(ctx, contractAddr)
				if abiErr == nil && contractABI != nil {
					decoded, decErr := abi.DecodeOutput(contractABI, method, rawResult)
					if decErr == nil && decoded != nil {
						result["result_decoded"] = stringifyDecoded(decoded)
					}
				}
			}

			// Check for revert reason
			if reason, decErr := abi.DecodeRevertReason(rawResult); decErr == nil {
				result["revert_reason"] = reason
			}
		}

		if callResult.EnergyUsed > 0 {
			result["energy_used"] = callResult.EnergyUsed
		}

		return mcp.NewToolResultJSON(result)
	}
}

func handleTriggerContract(pool *nodepool.Pool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from := req.GetString("from", "")
		contractAddr := req.GetString("contract_address", "")
		method := req.GetString("method", "")
		params := req.GetString("params", "")
		dataHex := req.GetString("data", "")
		feeLimit := req.GetInt("fee_limit", 100)
		callValue := int64(req.GetInt("call_value", 0))
		conn := pool.Client()

		if err := validateAddress(from); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid from address: %v", err)), nil
		}
		if err := validateAddress(contractAddr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid contract address: %v", err)), nil
		}

		if callValue < 0 {
			return mcp.NewToolResultError("trigger_contract: call_value must be non-negative"), nil
		}
		if feeLimit < 0 || feeLimit > 15000 {
			return mcp.NewToolResultError("fee_limit must be between 0 and 15000 TRX"), nil
		}
		feeLimitSun := int64(feeLimit) * 1_000_000

		call := contract.New(conn, contractAddr).
			From(from).
			WithFeeLimit(feeLimitSun)

		if callValue > 0 {
			call = call.WithCallValue(callValue)
		}

		if dataHex != "" {
			// Pre-packed calldata mode — ignore method entirely
			method = ""
			dataHex = stripHexPrefix(dataHex)
			if len(dataHex) > maxCalldataHexLen {
				return mcp.NewToolResultError("trigger_contract: data exceeds maximum length"), nil
			}
			data, decErr := hex.DecodeString(dataHex)
			if decErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("trigger_contract: invalid data hex: %v", decErr)), nil
			}
			call = call.WithData(data)
		} else {
			if method == "" {
				return mcp.NewToolResultError("trigger_contract: method is required when data is not provided"), nil
			}
			call = call.Method(method).Params(params)
		}

		// Apply optional permission_id for multi-sig
		args := req.GetArguments()
		if _, has := args["permission_id"]; has {
			pid := req.GetInt("permission_id", 0)
			if pid < 0 {
				return mcp.NewToolResultError("trigger_contract: permission_id must be non-negative"), nil
			}
			call = call.WithPermissionID(int32(pid))
		}

		tx, err := call.Build(ctx)
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
			"contract_address": contractAddr,
			"fee_limit_trx":    feeLimit,
			"type":             "TriggerSmartContract",
		}
		if method != "" {
			result["method"] = method
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

// stripHexPrefix removes a leading 0x or 0X prefix from a hex string.
func stripHexPrefix(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
}

// maxCalldataHexLen is the maximum hex-encoded calldata length accepted (1 MiB decoded).
const maxCalldataHexLen = 2 * (1 << 20)

func isEstimateEnergyUnsupported(err error) bool {
	if errors.Is(err, client.ErrEstimateEnergyNotSupported) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "does not support estimate energy")
}
