package trongrid

// Response is the generic TronGrid API response wrapper.
type Response[T any] struct {
	Data    []T  `json:"data"`
	Success bool `json:"success"`
	Meta    Meta `json:"meta"`
}

// Meta contains pagination metadata from TronGrid responses.
type Meta struct {
	At          int64  `json:"at"`
	PageSize    int    `json:"page_size"`
	Fingerprint string `json:"fingerprint"`
}

// Transaction represents a transaction from the TronGrid API.
type Transaction struct {
	TransactionID  string `json:"txID"`
	BlockNumber    int64  `json:"blockNumber"`
	BlockTimestamp int64  `json:"block_timestamp"`
	RawData        struct {
		Contract []struct {
			Parameter struct {
				Value map[string]any `json:"value"`
			} `json:"parameter"`
			Type string `json:"type"`
		} `json:"contract"`
	} `json:"raw_data"`
	Ret []struct {
		ContractRet string `json:"contractRet"`
	} `json:"ret"`
	EnergyUsage    int64 `json:"energy_usage"`
	EnergyFee      int64 `json:"energy_fee"`
	NetUsage       int64 `json:"net_usage"`
	NetFee         int64 `json:"net_fee"`
	InternalTxList []any `json:"internal_transactions"`
}

// TRC20Transfer represents a TRC20 token transfer from the TronGrid API.
type TRC20Transfer struct {
	TransactionID  string    `json:"transaction_id"`
	From           string    `json:"from"`
	To             string    `json:"to"`
	Value          string    `json:"value"`
	Type           string    `json:"type"`
	BlockTimestamp int64     `json:"block_timestamp"`
	TokenInfo      TokenInfo `json:"token_info"`
}

// TokenInfo describes the token involved in a TRC20 transfer.
type TokenInfo struct {
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
	Name     string `json:"name"`
}

// ContractEvent represents a decoded smart contract event from the TronGrid API.
type ContractEvent struct {
	TransactionID   string            `json:"transaction_id"`
	BlockNumber     int64             `json:"block_number"`
	BlockTimestamp  int64             `json:"block_timestamp"`
	EventName       string            `json:"event_name"`
	ContractAddress string            `json:"contract_address"`
	Result          map[string]any    `json:"result"`
	ResultType      map[string]string `json:"result_type"`
}

// QueryOpts holds optional query parameters for TronGrid API requests.
type QueryOpts struct {
	Limit         int
	Fingerprint   string
	OnlyConfirmed bool
	OnlyTo        bool
	OnlyFrom      bool
	MinTimestamp  int64
	MaxTimestamp  int64
	OrderBy       string
}
