// Command rsi is a mean-reversion strategy plugin: buy when RSI is oversold,
// close the position when RSI is overbought.
package main

import (
	"github.com/colinmyth/quant_ba/internal/indicator"
	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/strategy/pluginkit"
	"github.com/colinmyth/quant_ba/internal/types"
)

const (
	period     = 14
	oversold   = 30.0
	overbought = 70.0
	orderSize  = 0.01
	symbol     = "BTCUSDT"
	interval   = "15m"
)

type strat struct{}

func (s *strat) Meta() strategy.MetaResult {
	return strategy.MetaResult{
		ID:        "rsi_v1",
		Name:      "RSI Mean Reversion",
		Version:   "1.0.0",
		Symbols:   []string{symbol},
		Intervals: []string{interval},
		Params: types.StrategyParams{
			"period":     period,
			"oversold":   oversold,
			"overbought": overbought,
		},
	}
}

func (s *strat) OnBar(p strategy.OnBarParams) *types.Signal {
	prices := pluginkit.Closes(p.Bars)
	if len(prices) < period+1 {
		return nil
	}
	rsi := indicator.RSI(prices, period)
	hasPos := pluginkit.HasPosition(p.Symbol, p.Positions)

	switch {
	case rsi < oversold && !hasPos:
		return &types.Signal{
			Symbol:    p.Symbol,
			Direction: types.DirBuy,
			Size:      orderSize,
			Type:      types.OrdMarket,
			Reason:    "RSI oversold",
		}
	case rsi > overbought && hasPos:
		return &types.Signal{
			Symbol:    p.Symbol,
			Direction: types.DirSell,
			Size:      0, // close whole position
			Type:      types.OrdMarket,
			Reason:    "RSI overbought",
		}
	}
	return nil
}

func main() { pluginkit.Run(&strat{}) }
