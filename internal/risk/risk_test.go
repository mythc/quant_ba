package risk

import (
	"testing"

	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/types"
)

func TestBasicRisk_Blacklist(t *testing.T) {
	b := &BasicRisk{cfg: config.BasicRiskConfig{
		Blacklist:      []string{"SHITCOIN"},
		MaxPositionPct: 0.2,
	}}

	sig := &types.Signal{Symbol: "SHITCOIN", Direction: types.DirBuy, Size: 1, Price: 1}
	err := b.Check(sig, &types.Portfolio{})
	if err == nil {
		t.Fatal("expected blacklist error")
	}
}

func TestBasicRisk_Pass(t *testing.T) {
	b := &BasicRisk{cfg: config.BasicRiskConfig{
		MaxPositionPct: 0.2,
	}}

	sig := &types.Signal{Symbol: "BTCUSDT", Direction: types.DirBuy, Size: 0.01, Price: 50000}
	portfolio := &types.Portfolio{
		Balances: map[string]types.Balance{
			"USDT": {Asset: "USDT", Free: 10000},
		},
	}
	err := b.Check(sig, portfolio)
	if err != nil {
		t.Fatalf("expected pass, got: %v", err)
	}
}

func TestBasicRisk_PositionSizeTooLarge(t *testing.T) {
	b := &BasicRisk{cfg: config.BasicRiskConfig{
		MaxPositionPct: 0.2,
	}}

	// Position = 0.1 * 50000 = 5000, equity = 10000, position = 50% > 20%
	sig := &types.Signal{Symbol: "BTCUSDT", Direction: types.DirBuy, Size: 0.1, Price: 50000}
	portfolio := &types.Portfolio{
		Balances: map[string]types.Balance{
			"USDT": {Asset: "USDT", Free: 10000},
		},
	}
	err := b.Check(sig, portfolio)
	if err == nil {
		t.Fatal("expected position size error")
	}
}

func TestCircuitBreaker_DailyLossLimit(t *testing.T) {
	cb := NewCircuitBreaker(config.BreakerConfig{
		DailyLossPct:      0.05,
		ConsecutiveLosses: 5,
		MaxDrawdownPct:    0.20,
	})

	portfolio := &types.Portfolio{
		Balances: map[string]types.Balance{
			"USDT": {Asset: "USDT", Free: 10000},
		},
	}

	// First check sets start equity
	err := cb.Check(&types.Signal{StrategyID: "test"}, portfolio)
	if err != nil {
		t.Fatalf("expected pass, got: %v", err)
	}

	// Record a big loss → should trigger daily loss breaker
	cb.state.DailyPnL = -600 // -6% of 10000
	err = cb.Check(&types.Signal{StrategyID: "test"}, portfolio)
	if err == nil {
		t.Fatal("expected daily loss limit error")
	}
}

func TestCircuitBreaker_ConsecutiveLosses(t *testing.T) {
	cb := NewCircuitBreaker(config.BreakerConfig{
		DailyLossPct:      0.05,
		ConsecutiveLosses: 3,
		MaxDrawdownPct:    0.20,
	})

	// Record 3 consecutive losing sells
	// For sells: pnl = -(FilledPrice - Price) * Size
	// To get negative PnL: FilledPrice > Price
	for i := 0; i < 3; i++ {
		cb.RecordOrder(types.Order{
			Status:      types.OrdFilled,
			Side:        types.DirSell,
			FilledPrice: 50000,
			Price:       49000,
			FilledSize:  0.01,
			StrategyID:  "test",
		})
	}

	// Strategy should be paused
	if !cb.state.StrategyPaused["test"] {
		t.Fatal("expected strategy to be paused")
	}

	// Check should block
	err := cb.Check(&types.Signal{StrategyID: "test"}, &types.Portfolio{})
	if err == nil {
		t.Fatal("expected strategy paused error")
	}
}
