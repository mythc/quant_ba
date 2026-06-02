package market

import (
	"context"

	"github.com/colinmyth/quant_ba/internal/types"
)

// DataFeed provides an abstraction over market data sources (REST, WebSocket, cache).
// Subscribers receive a channel that emits live data; historical data is available
// via FetchKlines.
type DataFeed interface {
	// SubscribeKline returns a channel that receives live klines for the given
	// symbol and interval.
	SubscribeKline(ctx context.Context, symbol string, interval string) (<-chan types.Kline, error)

	// FetchKlines retrieves historical klines via REST.
	FetchKlines(ctx context.Context, symbol string, interval string, limit int) ([]types.Kline, error)

	// SubscribeOrderBook returns a channel that receives order book snapshots.
	SubscribeOrderBook(ctx context.Context, symbol string, depth int) (<-chan types.OrderBook, error)

	// Unsubscribe stops the subscription for the given symbol and interval.
	Unsubscribe(symbol string, interval string) error
}
