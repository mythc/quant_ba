// Command macd is a trend/momentum strategy plugin: buy when the MACD line
// crosses above its signal line, close when it crosses back below.
package main

import (
	"github.com/colinmyth/quant_ba/internal/indicator"
	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/strategy/pluginkit"
	"github.com/colinmyth/quant_ba/internal/types"
)

const (
	fastPeriod   = 12
	slowPeriod   = 26
	signalPeriod = 9
	orderSize    = 0.01
	symbol       = "BTCUSDT"
	interval     = "1h"
)

type strat struct{}

func (s *strat) Meta() strategy.MetaResult {
	return strategy.MetaResult{
		ID:        "macd_v1",
		Name:      "MACD Crossover",
		Version:   "1.0.0",
		Symbols:   []string{symbol},
		Intervals: []string{interval},
		Params: types.StrategyParams{
			"fast":   fastPeriod,
			"slow":   slowPeriod,
			"signal": signalPeriod,
		},
	}
}

func (s *strat) OnBar(p strategy.OnBarParams) *types.Signal {
	prices := pluginkit.Closes(p.Bars)
	if len(prices) < slowPeriod+signalPeriod+1 {
		return nil
	}
	macd, sig, _ := indicator.MACD(prices, fastPeriod, slowPeriod, signalPeriod)
	prevMACD, prevSig, _ := indicator.MACD(prices[:len(prices)-1], fastPeriod, slowPeriod, signalPeriod)
	hasPos := pluginkit.HasPosition(p.Symbol, p.Positions)

	crossedUp := prevMACD <= prevSig && macd > sig
	crossedDown := prevMACD >= prevSig && macd < sig

	switch {
	case crossedUp && !hasPos:
		return &types.Signal{
			Symbol:    p.Symbol,
			Direction: types.DirBuy,
			Size:      orderSize,
			Type:      types.OrdMarket,
			Reason:    "MACD bullish cross",
		}
	case crossedDown && hasPos:
		return &types.Signal{
			Symbol:    p.Symbol,
			Direction: types.DirSell,
			Size:      0, // close whole position
			Type:      types.OrdMarket,
			Reason:    "MACD bearish cross",
		}
	}
	return nil
}

func main() { pluginkit.Run(&strat{}) }
