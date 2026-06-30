// Package pluginkit provides the runtime boilerplate for strategy plugins.
// A plugin implements the Strategy interface and calls Run; pluginkit handles
// the JSON-RPC loop over stdin/stdout and dispatches lifecycle methods.
package pluginkit

import (
	"encoding/json"
	"os"

	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/types"
)

// Strategy is the minimal interface a plugin must implement.
type Strategy interface {
	// Meta returns static metadata (id, name, symbols, intervals, params).
	Meta() strategy.MetaResult
	// OnBar receives the rolling bar window and returns a signal (or nil/hold).
	OnBar(params strategy.OnBarParams) *types.Signal
}

// Initializer is an optional hook invoked once with the starting portfolio.
type Initializer interface {
	OnInit(params strategy.OnInitParams)
}

// Run starts the plugin's request loop, reading one JSON request per line from
// stdin and writing one JSON response per line to stdout. It returns when
// stdin is closed.
func Run(s Strategy) {
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for {
		var req strategy.Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		_ = enc.Encode(handle(s, req))
	}
}

func handle(s Strategy, req strategy.Request) strategy.Response {
	switch req.Method {
	case "meta":
		return result(req.ID, s.Meta())
	case "init":
		if init, ok := s.(Initializer); ok {
			var p strategy.OnInitParams
			_ = json.Unmarshal(req.Params, &p)
			init.OnInit(p)
		}
		return empty(req.ID)
	case "bar":
		var p strategy.OnBarParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return strategy.Response{ID: req.ID, Error: err.Error()}
		}
		return result(req.ID, strategy.SignalResult{Signal: s.OnBar(p)})
	case "order_update", "pause", "resume", "stop":
		return empty(req.ID)
	default:
		return strategy.Response{ID: req.ID, Error: "unknown method: " + req.Method}
	}
}

func result(id string, v interface{}) strategy.Response {
	data, err := json.Marshal(v)
	if err != nil {
		return strategy.Response{ID: id, Error: err.Error()}
	}
	return strategy.Response{ID: id, Result: data}
}

func empty(id string) strategy.Response {
	return strategy.Response{ID: id, Result: []byte("{}")}
}

// Closes extracts the close prices from a bar window, oldest to newest.
func Closes(bars []types.Kline) []float64 {
	out := make([]float64, len(bars))
	for i, b := range bars {
		out[i] = b.Close
	}
	return out
}

// HasPosition reports whether there is an open long position for the symbol.
func HasPosition(symbol string, positions map[string]*types.Position) bool {
	pos, ok := positions[symbol]
	return ok && pos != nil && pos.Size > 0
}
