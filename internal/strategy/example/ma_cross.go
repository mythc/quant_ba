package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/types"
)

func main() {
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	handler := &MACrossHandler{}

	for {
		var req strategy.Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		resp := handler.Handle(req)
		enc.Encode(resp)
	}
}

// MACrossHandler implements a simple moving average crossover strategy.
type MACrossHandler struct {
	shortPeriod int
	longPeriod  int
	symbols     []string
	interval    string
	prices      map[string][]float64
}

// Handle dispatches a JSON-RPC request to the appropriate handler.
func (h *MACrossHandler) Handle(req strategy.Request) strategy.Response {
	switch req.Method {
	case "meta":
		return strategy.Response{
			ID: req.ID,
			Result: mustMarshal(strategy.MetaResult{
				ID:        "ma_cross_v1",
				Name:      "MA Cross",
				Version:   "1.0.0",
				Symbols:   []string{"BTCUSDT"},
				Intervals: []string{"5m"},
				Params:    types.StrategyParams{"short_period": 5, "long_period": 20},
			}),
		}

	case "init":
		var params strategy.OnInitParams
		json.Unmarshal(req.Params, &params)
		h.shortPeriod = 5
		h.longPeriod = 20
		h.symbols = []string{"BTCUSDT"}
		h.interval = "5m"
		h.prices = make(map[string][]float64)
		return strategy.Response{ID: req.ID, Result: []byte("{}")}

	case "bar":
		var params strategy.OnBarParams
		json.Unmarshal(req.Params, &params)
		return h.onBar(params, req.ID)

	case "order_update":
		return strategy.Response{ID: req.ID, Result: []byte("{}")}

	case "pause":
		return strategy.Response{ID: req.ID, Result: []byte("{}")}

	case "resume":
		return strategy.Response{ID: req.ID, Result: []byte("{}")}

	case "stop":
		return strategy.Response{ID: req.ID, Result: []byte("{}")}

	default:
		return strategy.Response{ID: req.ID, Error: "unknown method: " + req.Method}
	}
}

func (h *MACrossHandler) onBar(params strategy.OnBarParams, id string) strategy.Response {
	symbol := params.Symbol

	// Extract close prices from the current bar window.
	prices := make([]float64, len(params.Bars))
	for i, bar := range params.Bars {
		prices[i] = bar.Close
	}
	h.prices[symbol] = prices
	if len(prices) < h.longPeriod+1 {
		return strategy.Response{ID: id, Result: mustMarshal(strategy.SignalResult{Signal: nil})}
	}

	shortMA := sma(prices, h.shortPeriod)
	longMA := sma(prices, h.longPeriod)
	prevShortMA := sma(prices[:len(prices)-1], h.shortPeriod)
	prevLongMA := sma(prices[:len(prices)-1], h.longPeriod)

	// Check if already in position
	hasPosition := false
	for _, pos := range params.Positions {
		if pos.Symbol == symbol && pos.Size > 0 {
			hasPosition = true
			break
		}
	}

	var sig *types.Signal

	fmt.Fprintf(os.Stderr, "[%s] short=%.2f long=%.2f prev=%.2f/%.2f pos=%v\n",
			symbol, shortMA, longMA, prevShortMA, prevLongMA, hasPosition)

		if prevShortMA <= prevLongMA && shortMA > longMA && !hasPosition {
		// Golden cross -> Buy
		sig = &types.Signal{
			Symbol:    symbol,
			Direction: types.DirBuy,
			Size:      0.01,
			Type:      types.OrdMarket,
			Reason:    "MA golden cross",
		}
	} else if prevShortMA >= prevLongMA && shortMA < longMA && hasPosition {
		// Death cross -> Sell
		sig = &types.Signal{
			Symbol:    symbol,
			Direction: types.DirSell,
			Size:      0, // close position
			Type:      types.OrdMarket,
			Reason:    "MA death cross",
		}
	}

	return strategy.Response{ID: id, Result: mustMarshal(strategy.SignalResult{Signal: sig})}
}

// sma computes a simple moving average over the last `period` prices.
func sma(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}
	sum := 0.0
	for _, p := range prices[len(prices)-period:] {
		sum += p
	}
	return sum / float64(period)
}

// mustMarshal serializes v to JSON or returns an empty object on failure.
func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
