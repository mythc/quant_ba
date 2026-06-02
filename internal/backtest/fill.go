package backtest

import "github.com/colinmyth/quant_ba/internal/types"

const (
	slippageBuy  = 1.0005
	slippageSell = 0.9995
	feeRate      = 0.001 // 0.1%
)

// SimulateFill simulates order fill against a bar.
// Returns the order with filled size/price set, or nil if the order cannot fill.
func SimulateFill(signal *types.Signal, bar types.Kline) *types.Order {
	order := &types.Order{
		Symbol:     signal.Symbol,
		Side:       signal.Direction,
		Type:       signal.Type,
		Size:       signal.Size,
		Price:      signal.Price,
		Status:     types.OrdNew,
		StrategyID: signal.StrategyID,
	}

	if order.Size == 0 {
		return nil
	}

	if order.Type == types.OrdMarket {
		fillPrice := (bar.Open + bar.High + bar.Low + bar.Close) / 4 // VWAP approx
		if order.Side == types.DirBuy {
			fillPrice *= slippageBuy
		} else {
			fillPrice *= slippageSell
		}
		order.Status = types.OrdFilled
		order.FilledSize = order.Size
		order.FilledPrice = fillPrice
		return order
	}

	// Limit order: check if price was touched
	if order.Type == types.OrdLimit {
		if order.Side == types.DirBuy && bar.Low <= order.Price {
			fillPrice := order.Price * slippageBuy
			order.Status = types.OrdFilled
			order.FilledSize = order.Size
			order.FilledPrice = fillPrice
			return order
		}
		if order.Side == types.DirSell && bar.High >= order.Price {
			fillPrice := order.Price * slippageSell
			order.Status = types.OrdFilled
			order.FilledSize = order.Size
			order.FilledPrice = fillPrice
			return order
		}
		return nil // Limit order not filled this bar
	}

	return nil
}

// Fee returns the trading fee for a filled order.
func Fee(order *types.Order) float64 {
	return order.FilledSize * order.FilledPrice * feeRate
}
