package risk

import (
	"fmt"

	"github.com/colinmyth/quant_ba/internal/config"
	"github.com/colinmyth/quant_ba/internal/types"
)

// BasicRisk implements Layer 1: basic limits (blacklist, position size, slippage).
type BasicRisk struct {
	cfg config.BasicRiskConfig
}

// Check validates the signal against basic risk limits.
func (b *BasicRisk) Check(signal *types.Signal, portfolio *types.Portfolio) error {
	// Blacklist check
	for _, sym := range b.cfg.Blacklist {
		if sym == signal.Symbol {
			return fmt.Errorf("risk: symbol %s is blacklisted", signal.Symbol)
		}
	}

	// Position size check
	equity := portfolioEquity(portfolio)
	if equity > 0 && signal.Price > 0 {
		posValue := signal.Size * signal.Price
		maxValue := equity * b.cfg.MaxPositionPct
		if posValue > maxValue {
			return fmt.Errorf("risk: position value %.2f exceeds max %.2f (%.0f%% of equity)",
				posValue, maxValue, b.cfg.MaxPositionPct*100)
		}
	}

	return nil
}

// portfolioEquity calculates total portfolio equity from balances and positions.
func portfolioEquity(p *types.Portfolio) float64 {
	total := 0.0
	for _, bal := range p.Balances {
		if bal.Asset == "USDT" {
			total += bal.Free + bal.Locked
		}
	}
	for _, pos := range p.Positions {
		total += pos.Size * pos.CurrentPrice
	}
	return total
}
