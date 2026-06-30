// Command bollinger is a mean-reversion strategy plugin: buy when price closes
// below the lower Bollinger band, close when it reverts back above the middle
// band.
package main

import (
	"github.com/colinmyth/quant_ba/internal/indicator"
	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/strategy/pluginkit"
	"github.com/colinmyth/quant_ba/internal/types"
)

const (
	period    = 20
	stddevK   = 2.0
	orderSize = 0.01
	symbol    = "BTCUSDT"
	interval  = "15m"
)

type strat struct{}

func (s *strat) Meta() strategy.MetaResult {
	return strategy.MetaResult{
		ID:        "bollinger_v1",
		Name:      "Bollinger Band Reversion",
		Version:   "1.0.0",
		Symbols:   []string{symbol},
		Intervals: []string{interval},
		Params: types.StrategyParams{
			"period":   period,
			"stddev_k": stddevK,
		},
	}
}

func (s *strat) OnBar(p strategy.OnBarParams) *types.Signal {
	prices := pluginkit.Closes(p.Bars)
	if len(prices) < period {
		return nil
	}
	mid, _, lower := indicator.Bollinger(prices, period, stddevK)
	last := prices[len(prices)-1]
	hasPos := pluginkit.HasPosition(p.Symbol, p.Positions)

	switch {
	case last < lower && !hasPos:
		return &types.Signal{
			Symbol:    p.Symbol,
			Direction: types.DirBuy,
			Size:      orderSize,
			Type:      types.OrdMarket,
			Reason:    "price below lower band",
		}
	case last > mid && hasPos:
		return &types.Signal{
			Symbol:    p.Symbol,
			Direction: types.DirSell,
			Size:      0, // close whole position
			Type:      types.OrdMarket,
			Reason:    "price reverted to mean",
		}
	}
	return nil
}

func main() { pluginkit.Run(&strat{}) }
