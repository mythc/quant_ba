package backtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/colinmyth/quant_ba/internal/market"
	"github.com/colinmyth/quant_ba/internal/portfolio"
	"github.com/colinmyth/quant_ba/internal/risk"
	"github.com/colinmyth/quant_ba/internal/store"
	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/types"
)

type Engine struct {
	cache     *market.KlineCache
	risk      risk.RiskManager
	portfolio *portfolio.Service
	store     *store.Store
}

func NewEngine(cache *market.KlineCache, risk risk.RiskManager, portfolio *portfolio.Service, store *store.Store) *Engine {
	return &Engine{
		cache:     cache,
		risk:      risk,
		portfolio: portfolio,
		store:     store,
	}
}

func (e *Engine) Run(ctx context.Context, strat *strategy.LoadedStrategy, symbols []string, interval string, start, end time.Time, startCapital float64) (*BacktestResult, error) {
	// Init portfolio with starting capital
	e.portfolio.Init(map[string]types.Balance{
		"USDT": {Asset: "USDT", Free: startCapital},
	})

	port := e.portfolio.GetPortfolio()
	initParams := strategy.OnInitParams{
		Balances:  port.Balances,
		Positions: port.Positions,
	}
	if err := strat.Client.Call("init", initParams, nil); err != nil {
		return nil, fmt.Errorf("strategy init: %w", err)
	}

	// Fetch ALL historical data upfront, paging through the API for long spans.
	symbolData := make(map[string][]types.Kline)
	for _, sym := range symbols {
		klines, err := e.cache.GetOrFetchRange(ctx, sym, interval, start, end)
		if err != nil {
			return nil, fmt.Errorf("fetch %s klines: %w", sym, err)
		}
		symbolData[sym] = klines
	}

	// Build unified timeline
	timeMap := make(map[int64][]types.Kline)
	for _, klines := range symbolData {
		for _, k := range klines {
			ts := k.OpenTime.UnixMilli()
			timeMap[ts] = append(timeMap[ts], k)
		}
	}

	var timestamps []int64
	for ts := range timeMap {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	// Run simulation bar by bar
	var trades []types.Trade
	var equityCurve []types.EquityPoint
	equity := startCapital
	equityCurve = append(equityCurve, types.EquityPoint{Time: start, Equity: equity})

	// seen accumulates the bars revealed so far per symbol. The strategy is
	// only ever shown data up to and including the current bar to avoid
	// lookahead bias.
	seen := make(map[string][]types.Kline)

	for _, ts := range timestamps {
		bars := timeMap[ts]
		if len(bars) == 0 {
			continue
		}

		port := e.portfolio.GetPortfolio()

		for _, bar := range bars {
			seen[bar.Symbol] = append(seen[bar.Symbol], bar)

			var sigResp strategy.SignalResult
			barParams := strategy.OnBarParams{
				Symbol:    bar.Symbol,
				Bars:      seen[bar.Symbol],
				Balances:  port.Balances,
				Positions: port.Positions,
			}
			if err := strat.Client.Call("bar", barParams, &sigResp); err != nil {
				continue
			}

			if sigResp.Signal == nil || sigResp.Signal.Direction == types.DirHold {
				continue
			}

			sig := sigResp.Signal
			sig.StrategyID = strat.Meta.ID

			// Risk check
			if err := e.risk.PreCheck(ctx, sig, port); err != nil {
				continue
			}

			// Simulate fill
			order := SimulateFill(sig, bar)
			if order == nil {
				continue
			}

			// Apply to portfolio
			e.portfolio.OnOrderFilled(*order)
			e.risk.PostCheck(ctx, *order, port)

			// Record trade
			trade := types.Trade{
				ID:         fmt.Sprintf("bt_%d", len(trades)+1),
				Symbol:     order.Symbol,
				Side:       order.Side,
				Size:       order.FilledSize,
				EntryPrice: order.FilledPrice,
				PnL:        order.PnL(),
				PnLPct:     0,
				EntryTime:  bar.OpenTime,
				ExitTime:   bar.CloseTime,
				StrategyID: strat.Meta.ID,
			}
			trades = append(trades, trade)
			equity += order.PnL() - Fee(order)
		}

		equityCurve = append(equityCurve, types.EquityPoint{
			Time:   time.UnixMilli(ts),
			Equity: equity,
		})
	}

	result := ComputeStats(trades, equityCurve, startCapital)
	return result, nil
}

func (e *Engine) SaveResult(result *BacktestResult, path string) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
