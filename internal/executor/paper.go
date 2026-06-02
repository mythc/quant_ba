package executor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/market"
	"github.com/colinmyth/quant_ba/internal/order"
	"github.com/colinmyth/quant_ba/internal/portfolio"
	"github.com/colinmyth/quant_ba/internal/risk"
	"github.com/colinmyth/quant_ba/internal/strategy"
	"github.com/colinmyth/quant_ba/internal/types"
)

// PaperExecutor runs a strategy against live market data without real money.
// It wires together market data -> strategy -> risk -> order -> portfolio
// in a paper trading loop.
type PaperExecutor struct {
	feed      market.DataFeed
	loader    *strategy.Loader
	risk      risk.RiskManager
	orderMgr  order.OrderManager
	portfolio *portfolio.Service

	mu       sync.Mutex
	statuses map[string]*StrategyStatus
	cancels  map[string]context.CancelFunc
}

// NewPaperExecutor creates a PaperExecutor backed by the given dependencies.
func NewPaperExecutor(
	feed market.DataFeed,
	loader *strategy.Loader,
	risk risk.RiskManager,
	orderMgr order.OrderManager,
	portfolio *portfolio.Service,
) *PaperExecutor {
	return &PaperExecutor{
		feed:      feed,
		loader:    loader,
		risk:      risk,
		orderMgr:  orderMgr,
		portfolio: portfolio,
		statuses:  make(map[string]*StrategyStatus),
		cancels:   make(map[string]context.CancelFunc),
	}
}

// Run starts the paper trading loop for the given strategy. It loads the
// strategy plugin, initializes it, subscribes to market data for each
// symbol, and runs the event loop until ctx is cancelled or Stop is called.
func (e *PaperExecutor) Run(ctx context.Context, strategyID string) error {
	e.mu.Lock()
	if _, running := e.cancels[strategyID]; running {
		e.mu.Unlock()
		return fmt.Errorf("strategy %s already running", strategyID)
	}
	e.mu.Unlock()

	ls, err := e.loader.Get(strategyID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)

	e.mu.Lock()
	e.cancels[strategyID] = cancel
	e.statuses[strategyID] = &StrategyStatus{
		StrategyID: strategyID,
		Running:    true,
		Mode:       "paper",
		StartedAt:  time.Now().Format(time.RFC3339),
	}
	e.mu.Unlock()

	// Initialize the strategy with current portfolio state.
	port := e.portfolio.GetPortfolio()
	initParams := strategy.OnInitParams{
		Balances:  port.Balances,
		Positions: port.Positions,
	}
	if err := ls.Client.Call("init", initParams, nil); err != nil {
		return fmt.Errorf("strategy init: %w", err)
	}

	// Subscribe to data for each symbol the strategy tracks.
	var wg sync.WaitGroup
	for _, sym := range ls.Meta.Symbols {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			e.runSymbolLoop(ctx, ls, symbol)
		}(sym)
	}

	// Periodic portfolio snapshot.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := e.portfolio.Snapshot(); err != nil {
					log.Printf("snapshot error: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	return nil
}

// runSymbolLoop subscribes to kline data for a single symbol and runs the
// strategy on each bar: strategy -> risk pre-check -> order -> portfolio update.
func (e *PaperExecutor) runSymbolLoop(ctx context.Context, ls *strategy.LoadedStrategy, symbol string) {
	// Use the first interval from the strategy metadata, default to "1h".
	interval := "1h"
	if len(ls.Meta.Intervals) > 0 {
		interval = ls.Meta.Intervals[0]
	}

	ch, err := e.feed.SubscribeKline(ctx, symbol, interval)
	if err != nil {
		log.Printf("subscribe %s: %v", symbol, err)
		return
	}

	var bars []types.Kline
	for {
		select {
		case bar, ok := <-ch:
			if !ok {
				return
			}
			bars = append(bars, bar)
			if len(bars) > 100 {
				bars = bars[1:]
			}

			port := e.portfolio.GetPortfolio()

			// Call strategy.
			var sigResp strategy.SignalResult
			barParams := strategy.OnBarParams{
				Symbol:    symbol,
				Bars:      bars,
				Balances:  port.Balances,
				Positions: port.Positions,
			}
			if err := ls.Client.Call("bar", barParams, &sigResp); err != nil {
				log.Printf("strategy bar: %v", err)
				continue
			}

			if sigResp.Signal == nil || sigResp.Signal.Direction == types.DirHold {
				continue
			}

			sig := sigResp.Signal
			sig.StrategyID = ls.Meta.ID

			// Risk pre-check.
			if err := e.risk.PreCheck(ctx, sig, port); err != nil {
				log.Printf("risk blocked: %v", err)
				continue
			}

			// Place order (paper or live).
			ord, err := e.orderMgr.Place(ctx, sig)
			if err != nil {
				log.Printf("place order: %v", err)
				continue
			}

			// Update portfolio on fill.
			if ord.Status == types.OrdFilled {
				e.portfolio.OnOrderFilled(*ord)
				ls.Client.Call("order_update", strategy.OnOrderUpdateParams{
					Order:     *ord,
					Balances:  port.Balances,
					Positions: port.Positions,
				}, nil)
				e.risk.PostCheck(ctx, *ord, port)
			}

		case <-ctx.Done():
			return
		}
	}
}

// Stop cancels the context for the given strategy, shutting down its event loop.
func (e *PaperExecutor) Stop(strategyID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	cancel, ok := e.cancels[strategyID]
	if !ok {
		return fmt.Errorf("strategy %s not running", strategyID)
	}
	cancel()
	delete(e.cancels, strategyID)
	if s, ok := e.statuses[strategyID]; ok {
		s.Running = false
	}
	return nil
}

// Status returns the current runtime status of a strategy, or nil if not found.
func (e *PaperExecutor) Status(strategyID string) *StrategyStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.statuses[strategyID]
}
