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

	klines, err := c.rest.FetchKlines(ctx, symbol, interval, limit, time.Time{})
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

// pageSize is the per-call max for Binance klines endpoints (1500).
const pageSize = 1500

// GetOrFetchRange returns all klines for symbol/interval within [start, end],
// paging backwards from end when the requested span exceeds a single REST
// page. Results are persisted to the cache store as they arrive.
func (c *KlineCache) GetOrFetchRange(ctx context.Context, symbol, interval string, start, end time.Time) ([]types.Kline, error) {
	// Try the SQLite cache first: if it already covers [start, end] we skip
	// the network entirely. A negative limit means "no limit" in the store.
	if cached, err := c.store.GetKlines(symbol, interval, start, end, -1); err == nil && len(cached) > 0 {
		first, last := cached[0].OpenTime, cached[len(cached)-1].OpenTime
		if !first.After(start) && !last.Before(end) {
			return cached, nil
		}
	}

	step := klineInterval(interval)
	var all []types.Kline
	cursor := end

	for cursor.After(start) {
		// Page covers [cursor - pageSize*step, cursor].
		pageStart := cursor.Add(-time.Duration(pageSize) * step)

		klines, err := c.rest.FetchKlines(ctx, symbol, interval, pageSize, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch klines page: %w", err)
		}
		if len(klines) == 0 {
			break
		}
		if err := c.store.SaveKlines(klines); err != nil {
			return all, fmt.Errorf("cache save: %w", err)
		}
		all = append(all, klines...)

		// Advance the cursor to just before the oldest bar we received.
		oldest := klines[0].OpenTime
		if !oldest.Before(cursor) {
			break // safety: server returned the same window twice
		}
		cursor = oldest.Add(-step)

		// Stop once the page no longer overlaps the requested start.
		if cursor.Before(start) || pageStart.Before(start) {
			break
		}
	}

	// Filter to [start, end] and de-duplicate by OpenTime.
	seen := make(map[int64]struct{})
	var out []types.Kline
	for _, k := range all {
		if k.OpenTime.Before(start) || k.OpenTime.After(end) {
			continue
		}
		if _, ok := seen[k.OpenTime.UnixMilli()]; ok {
			continue
		}
		seen[k.OpenTime.UnixMilli()] = struct{}{}
		out = append(out, k)
	}
	return out, nil
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
