package executor

import (
	"context"

	"github.com/colinmyth/quant_ba/internal/types"
)

// Executor is the top-level interface for running strategies in any mode
// (paper, live, or backtest). It wires together market data, strategy
// logic, risk checks, order management, and portfolio tracking.
type Executor interface {
	Run(ctx context.Context, strategyID string) error
	Stop(strategyID string) error
	Status(strategyID string) *StrategyStatus
}

// StrategyStatus holds the runtime status of a strategy managed by the executor.
type StrategyStatus struct {
	StrategyID string
	Running    bool
	Mode       string // "paper", "live", "backtest"
	Portfolio  *types.Portfolio
	StartedAt  string
}
