package risk

import (
	"fmt"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/types"
)

// GlobalRisk implements Layer 2: global limits (leverage, concentration, daily trades, hold interval).
type GlobalRisk struct {
	cfg        config.GlobalRiskConfig
	mu         sync.Mutex
	tradeCount int
	tradeDay   string
	tradeTimes map[string]time.Time // symbol → last trade time
}

// NewGlobalRisk creates a new global risk checker.
func NewGlobalRisk(cfg config.GlobalRiskConfig) *GlobalRisk {
	return &GlobalRisk{
		cfg:        cfg,
		tradeTimes: make(map[string]time.Time),
	}
}

// Check validates the signal against global risk limits.
func (g *GlobalRisk) Check(signal *types.Signal, portfolio *types.Portfolio) error {
	// Daily trade count
	today := time.Now().Format("2006-01-02")
	g.mu.Lock()
	if g.tradeDay != today {
		g.tradeDay = today
		g.tradeCount = 0
	}
	if g.tradeCount >= g.cfg.DailyTradeLimit {
		g.mu.Unlock()
		return fmt.Errorf("risk: daily trade limit reached (%d)", g.cfg.DailyTradeLimit)
	}
	g.mu.Unlock()

	// Min hold interval
	if lastTrade, ok := g.tradeTimes[signal.Symbol]; ok {
		if time.Since(lastTrade).Seconds() < float64(g.cfg.MinHoldSeconds) {
			return fmt.Errorf("risk: min hold interval not met for %s (%.0fs < %ds)",
				signal.Symbol, time.Since(lastTrade).Seconds(), g.cfg.MinHoldSeconds)
		}
	}

	// Concentration check
	pos, ok := portfolio.Positions[signal.Symbol]
	if ok && pos.Size > 0 {
		equity := portfolioEquity(portfolio)
		if equity > 0 {
			posValue := pos.Size * pos.CurrentPrice
			conc := posValue / equity
			if conc > g.cfg.MaxConcentration {
				return fmt.Errorf("risk: concentration %.2f exceeds max %.2f", conc, g.cfg.MaxConcentration)
			}
		}
	}

	// Note: MaxLeverage is intentionally not enforced here. The portfolio is
	// spot-only (no borrowing), so gross exposure can never exceed equity and a
	// leverage cap has no meaning until margin trading is modeled.

	return nil
}

// RecordTrade records a completed trade for daily count and hold interval tracking.
func (g *GlobalRisk) RecordTrade(symbol string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tradeCount++
	g.tradeTimes[symbol] = time.Now()
}
