// Command macd_eth is a MACD crossover strategy targeting the ETHUSDT
// perpetual on a 15m timeframe. Mirrors macd/main.go but with ETHUSDT and
// a larger order size appropriate for ETH's lower per-unit price.
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
	orderSize    = 0.1
	symbol       = "ETHUSDT"
	interval     = "15m"
)

type strat struct{}

func (s *strat) Meta() strategy.MetaResult {
	return strategy.MetaResult{
		ID:        "macd_eth_v1",
		Name:      "MACD Crossover (ETHUSDT)",
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
