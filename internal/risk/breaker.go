package risk

import (
	"fmt"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/types"
)

// BreakerState holds the current state of all circuit breakers.
type BreakerState struct {
	AllPaused       bool
	StrategyPaused  map[string]bool
	DailyPnL        float64
	DailyPnLDay     string
	ConsecutiveLoss int
	StartEquity     float64
}

// CircuitBreaker implements Layer 3: circuit breakers (daily loss, consecutive losses, drawdown).
type CircuitBreaker struct {
	cfg   config.BreakerConfig
	mu    sync.Mutex
	state BreakerState
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(cfg config.BreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		cfg: cfg,
		state: BreakerState{
			StrategyPaused: make(map[string]bool),
		},
	}
}

// Check validates the signal against circuit breaker limits.
func (c *CircuitBreaker) Check(signal *types.Signal, portfolio *types.Portfolio) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// All-paused check
	if c.state.AllPaused {
		return fmt.Errorf("risk: circuit breaker active — all trading paused")
	}

	// Strategy-paused check
	if c.state.StrategyPaused[signal.StrategyID] {
		return fmt.Errorf("risk: strategy %s paused by circuit breaker", signal.StrategyID)
	}

	// Daily loss check
	today := time.Now().Format("2006-01-02")
	if c.state.DailyPnLDay != today {
		c.state.DailyPnLDay = today
		c.state.DailyPnL = 0
	}

	equity := portfolioEquity(portfolio)
	if c.state.StartEquity == 0 {
		c.state.StartEquity = equity
	}

	if c.state.StartEquity > 0 {
		dailyDrawdown := c.state.DailyPnL / c.state.StartEquity
		if dailyDrawdown < -c.cfg.DailyLossPct {
			c.state.AllPaused = true
			return fmt.Errorf("risk: daily loss limit reached (%.2f%%)", dailyDrawdown*100)
		}

		totalDrawdown := (equity - c.state.StartEquity) / c.state.StartEquity
		if totalDrawdown < -c.cfg.MaxDrawdownPct {
			c.state.AllPaused = true
			return fmt.Errorf("risk: max drawdown reached (%.2f%%)", totalDrawdown*100)
		}
	}

	return nil
}

// RecordOrder updates circuit breaker state after an order is filled.
func (c *CircuitBreaker) RecordOrder(order types.Order) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if order.Status == types.OrdFilled {
		pnl := (order.FilledPrice - order.Price) * order.FilledSize
		if order.Side == types.DirSell {
			pnl = -pnl
		}
		c.state.DailyPnL += pnl

		if pnl < 0 {
			c.state.ConsecutiveLoss++
			if c.state.ConsecutiveLoss >= c.cfg.ConsecutiveLosses {
				c.state.StrategyPaused[order.StrategyID] = true
			}
		} else {
			c.state.ConsecutiveLoss = 0
		}
	}
}

// PauseStrategy pauses a specific strategy.
func (c *CircuitBreaker) PauseStrategy(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.StrategyPaused[id] = true
}

// ResumeStrategy resumes a paused strategy.
func (c *CircuitBreaker) ResumeStrategy(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.StrategyPaused[id] = false
}

// ResetAll clears all breaker states.
func (c *CircuitBreaker) ResetAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.AllPaused = false
	c.state.StrategyPaused = make(map[string]bool)
	c.state.DailyPnL = 0
	c.state.ConsecutiveLoss = 0
}
