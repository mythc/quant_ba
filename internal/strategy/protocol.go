package strategy

import (
	"encoding/json"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
)

// Protocol messages for stdin/stdout plugin communication.
// One JSON object per line.

// Request is a JSON-RPC request sent to a strategy plugin process.
type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Response is a JSON-RPC response returned by a strategy plugin process.
type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Method-specific params and results.

// OnInitParams is the payload for the "init" RPC method.
type OnInitParams struct {
	Balances  map[string]types.Balance   `json:"balances"`
	Positions map[string]*types.Position `json:"positions"`
}

// OnBarParams is the payload for the "bar" RPC method.
type OnBarParams struct {
	Symbol    string                     `json:"symbol"`
	Bars      []types.Kline              `json:"bars"`
	Balances  map[string]types.Balance   `json:"balances"`
	Positions map[string]*types.Position `json:"positions"`
}

// OnOrderUpdateParams is the payload for the "order_update" RPC method.
type OnOrderUpdateParams struct {
	Order     types.Order                `json:"order"`
	Balances  map[string]types.Balance   `json:"balances"`
	Positions map[string]*types.Position `json:"positions"`
}

// MetaResult is the result of the "meta" RPC method.
type MetaResult struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Version   string               `json:"version"`
	Symbols   []string             `json:"symbols"`
	Intervals []string             `json:"intervals"`
	Params    types.StrategyParams `json:"params"`
}

// SignalResult is the result of the "bar" RPC method.
type SignalResult struct {
	Signal *types.Signal `json:"signal"`
}

// PluginClient is a JSON-RPC client that communicates with a strategy
// plugin process over stdin/stdout pipes.
type PluginClient struct {
	enc *json.Encoder
	dec *json.Decoder
}

// NewPluginClient creates a PluginClient backed by the given encoder and decoder.
func NewPluginClient(enc *json.Encoder, dec *json.Decoder) *PluginClient {
	return &PluginClient{enc: enc, dec: dec}
}

// Call sends a JSON-RPC request and returns the decoded result.
func (c *PluginClient) Call(method string, params interface{}, result interface{}) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req := Request{
		ID:     method + "_" + time.Now().Format(time.RFC3339Nano),
		Method: method,
		Params: paramsJSON,
	}
	if err := c.enc.Encode(req); err != nil {
		return err
	}
	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		return err
	}
	if resp.Error != "" {
		return &PluginError{Message: resp.Error}
	}
	if result != nil {
		return json.Unmarshal(resp.Result, result)
	}
	return nil
}

// PluginError represents an error returned by a strategy plugin.
type PluginError struct {
	Message string
}

// Error implements the error interface.
func (e *PluginError) Error() string {
	return "plugin error: " + e.Message
}
