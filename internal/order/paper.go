package order

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
)

// Fill modeling constants for paper trading, kept consistent with the
// backtest engine so paper results approximate backtested ones.
const (
	paperSlippageBuy  = 1.0005 // market buys fill 0.05% above reference
	paperSlippageSell = 0.9995 // market sells fill 0.05% below reference
	paperFeeRate      = 0.001  // 0.1% taker fee
)

// PaperOrderManager simulates order execution with immediate fills for market orders.
type PaperOrderManager struct {
	mu     sync.RWMutex
	orders map[string]*types.Order
	nextID int
}

// NewPaperOrderManager returns an initialized PaperOrderManager.
func NewPaperOrderManager() *PaperOrderManager {
	return &PaperOrderManager{
		orders: make(map[string]*types.Order),
		nextID: 1,
	}
}

// Place submits a new order based on a signal. Market orders are filled immediately.
func (p *PaperOrderManager) Place(ctx context.Context, signal *types.Signal) (*types.Order, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := fmt.Sprintf("paper_%d", p.nextID)
	p.nextID++

	order := &types.Order{
		ID:         id,
		Symbol:     signal.Symbol,
		Side:       signal.Direction,
		Type:       signal.Type,
		Price:      signal.Price,
		Size:       signal.Size,
		Status:     types.OrdNew,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		StrategyID: signal.StrategyID,
	}
	p.orders[id] = order

	// Simulate immediate fill for market orders, applying slippage and fees.
	if order.Type == types.OrdMarket {
		fillPrice := order.Price
		if order.Side == types.DirBuy {
			fillPrice *= paperSlippageBuy
		} else {
			fillPrice *= paperSlippageSell
		}
		order.Status = types.OrdFilled
		order.FilledSize = order.Size
		order.FilledPrice = fillPrice
		order.Fee = order.FilledSize * fillPrice * paperFeeRate
		order.UpdatedAt = time.Now()
	}

	return order, nil
}

// Cancel marks an order as cancelled. Returns an error if the order does not exist.
func (p *PaperOrderManager) Cancel(ctx context.Context, orderID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	order, ok := p.orders[orderID]
	if !ok {
		return fmt.Errorf("order %s not found", orderID)
	}
	order.Status = types.OrdCancelled
	order.UpdatedAt = time.Now()
	return nil
}

// Status returns the current state of an order.
func (p *PaperOrderManager) Status(ctx context.Context, orderID string) (*types.Order, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	order, ok := p.orders[orderID]
	if !ok {
		return nil, fmt.Errorf("order %s not found", orderID)
	}
	return order, nil
}

// OpenOrders returns all open (non-terminal) orders for a given symbol.
func (p *PaperOrderManager) OpenOrders(ctx context.Context, symbol string) ([]types.Order, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var open []types.Order
	for _, o := range p.orders {
		if o.Symbol == symbol && o.Status != types.OrdFilled &&
			o.Status != types.OrdCancelled && o.Status != types.OrdRejected {
			open = append(open, *o)
		}
	}
	return open, nil
}
