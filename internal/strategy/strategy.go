package strategy

import "github.com/colinmyth/quant_ba/internal/types"

// Strategy is the interface that all trading strategies must implement.
type Strategy interface {
	ID() string
	Name() string
	Version() string
	Symbols() []string
	Intervals() []string
	Params() types.StrategyParams

	OnInit(portfolio *types.Portfolio) error
	OnBar(symbol string, bars []types.Kline, portfolio *types.Portfolio) (*types.Signal, error)
	OnOrderUpdate(order types.Order, portfolio *types.Portfolio) error

	OnPause() error
	OnResume() error
	OnStop() error
}
