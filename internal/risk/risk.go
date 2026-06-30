package risk

import (
	"context"

	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/types"
)

// RiskManager defines the interface for risk checks.
type RiskManager interface {
	PreCheck(ctx context.Context, signal *types.Signal, portfolio *types.Portfolio) error
	PostCheck(ctx context.Context, order types.Order, portfolio *types.Portfolio) error
}

// Manager is the composite risk manager that chains all layers.
type Manager struct {
	cfg     config.RiskConfig
	basic   *BasicRisk
	global  *GlobalRisk
	breaker *CircuitBreaker
}

// New creates a new composite risk manager.
func New(cfg config.RiskConfig) *Manager {
	return &Manager{
		cfg:     cfg,
		basic:   &BasicRisk{cfg: cfg.Basic},
		global:  NewGlobalRisk(cfg.Global),
		breaker: NewCircuitBreaker(cfg.CircuitBreaker),
	}
}

// PreCheck runs all pre-trade risk layers in order.
func (m *Manager) PreCheck(ctx context.Context, signal *types.Signal, portfolio *types.Portfolio) error {
	if err := m.basic.Check(signal, portfolio); err != nil {
		return err
	}
	if err := m.global.Check(signal, portfolio); err != nil {
		return err
	}
	if err := m.breaker.Check(signal, portfolio); err != nil {
		return err
	}
	return nil
}

// PostCheck records order results and updates breaker/global state.
func (m *Manager) PostCheck(ctx context.Context, order types.Order, portfolio *types.Portfolio) error {
	m.breaker.RecordOrder(order)
	m.global.RecordTrade(order.Symbol)
	return nil
}
