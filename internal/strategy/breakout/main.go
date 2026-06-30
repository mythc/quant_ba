// Command breakout is a momentum strategy plugin using a Donchian channel: buy
// when price breaks above the prior N-bar high, close when it breaks below the
// prior N-bar low.
package main

import (
	"github.com/colinmyth/quant_ba/internal/indicator"
	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/strategy/pluginkit"
	"github.com/colinmyth/quant_ba/internal/types"
)

const (
	channel   = 20
	orderSize = 0.01
	symbol    = "BTCUSDT"
	interval  = "1h"
)

type strat struct{}

func (s *strat) Meta() strategy.MetaResult {
	return strategy.MetaResult{
		ID:        "breakout_v1",
		Name:      "Donchian Breakout",
		Version:   "1.0.0",
		Symbols:   []string{symbol},
		Intervals: []string{interval},
		Params:    types.StrategyParams{"channel": channel},
	}
}

func (s *strat) OnBar(p strategy.OnBarParams) *types.Signal {
	// Need `channel` prior bars plus the current bar.
	if len(p.Bars) < channel+1 {
		return nil
	}
	prior := p.Bars[:len(p.Bars)-1]
	highs := make([]float64, len(prior))
	lows := make([]float64, len(prior))
	for i, b := range prior {
		highs[i] = b.High
		lows[i] = b.Low
	}
	upper := indicator.Highest(highs, channel)
	lower := indicator.Lowest(lows, channel)
	last := p.Bars[len(p.Bars)-1].Close
	hasPos := pluginkit.HasPosition(p.Symbol, p.Positions)

	switch {
	case last > upper && !hasPos:
		return &types.Signal{
			Symbol:    p.Symbol,
			Direction: types.DirBuy,
			Size:      orderSize,
			Type:      types.OrdMarket,
			Reason:    "breakout above channel high",
		}
	case last < lower && hasPos:
		return &types.Signal{
			Symbol:    p.Symbol,
			Direction: types.DirSell,
			Size:      0, // close whole position
			Type:      types.OrdMarket,
			Reason:    "breakdown below channel low",
		}
	}
	return nil
}

func main() { pluginkit.Run(&strat{}) }
