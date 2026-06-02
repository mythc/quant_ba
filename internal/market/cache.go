package market

import (
	"context"
	"fmt"
	"time"

	"github.com/colinmyth/quant_ba/internal/store"
	"github.com/colinmyth/quant_ba/internal/types"
)

// KlineCache provides cache-or-fetch logic backed by the SQLite store.
type KlineCache struct {
	store *store.Store
	rest  *RESTClient
}

// NewKlineCache creates a cache that uses the given store and REST client.
func NewKlineCache(store *store.Store, rest *RESTClient) *KlineCache {
	return &KlineCache{store: store, rest: rest}
}

// GetOrFetch returns cached klines for the requested window. When the cache
// contains enough entries the REST call is skipped. When the cache is
// incomplete the REST API is called and the result is persisted. If the
// REST call fails but stale cache entries exist they are returned as a
// fallback.
func (c *KlineCache) GetOrFetch(ctx context.Context, symbol, interval string, limit int) ([]types.Kline, error) {
	end := time.Now()
	start := end.Add(-time.Duration(limit) * klineInterval(interval))

	cached, err := c.store.GetKlines(symbol, interval, start, end, limit)
	if err == nil && len(cached) >= limit {
		return cached[:limit], nil
	}

	klines, err := c.rest.FetchKlines(ctx, symbol, interval, limit)
	if err != nil {
		if len(cached) > 0 {
			return cached, nil // stale cache is better than nothing
		}
		return nil, err
	}

	if len(klines) > 0 {
		if err := c.store.SaveKlines(klines); err != nil {
			return klines, fmt.Errorf("cache save: %w", err)
		}
	}
	return klines, nil
}

// klineInterval maps a Binance interval string to a time.Duration.
func klineInterval(interval string) time.Duration {
	switch interval {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	case "4h":
		return 4 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return time.Hour
	}
}
