package order

import (
	"context"

	"github.com/colinmyth/quant_ba/internal/types"
)

// OrderManager is the interface for order execution, whether paper or live.
type OrderManager interface {
	Place(ctx context.Context, signal *types.Signal) (*types.Order, error)
	Cancel(ctx context.Context, orderID string) error
	Status(ctx context.Context, orderID string) (*types.Order, error)
	OpenOrders(ctx context.Context, symbol string) ([]types.Order, error)
}
