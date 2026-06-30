package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
)

const defaultBaseURL = "https://api.binance.com"

// RESTClient fetches market data from the Binance REST API.
type RESTClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRESTClient returns a REST client with the given base URL. If baseURL is
// empty the public Binance API endpoint is used.
func NewRESTClient(baseURL string) *RESTClient {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	transport := &http.Transport{
		Proxy:                 proxyFunc(),
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	return &RESTClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   120 * time.Second,
			Transport: transport,
		},
	}
}

// proxyFunc returns a proxy resolver that first checks the QUANT_BA_PROXY env
// var, then falls back to http.ProxyFromEnvironment.
func proxyFunc() func(*http.Request) (*url.URL, error) {
	override := os.Getenv("QUANT_BA_PROXY")
	if override != "" {
		return func(*http.Request) (*url.URL, error) {
			return url.Parse(override)
		}
	}
	return http.ProxyFromEnvironment
}

// klinesPath returns the REST path for klines based on the configured base URL.
// Spot uses /api/v3/klines; USDT-M futures uses /fapi/v1/klines.
func (c *RESTClient) klinesPath() string {
	if strings.Contains(c.baseURL, "fapi") {
		return "/fapi/v1/klines"
	}
	return "/api/v3/klines"
}

// klineRaw is the JSON representation returned by the Binance klines endpoint.
type klineRaw struct {
	OpenTime  int64  `json:"0"`
	Open      string `json:"1"`
	High      string `json:"2"`
	Low       string `json:"3"`
	Close     string `json:"4"`
	Volume    string `json:"5"`
	CloseTime int64  `json:"6"`
}

// FetchKlines retrieves klines from Binance REST API.
// The API returns an array of arrays: [[openTime, open, high, low, close, volume, closeTime, ...], ...]
// If endTime is non-zero the request uses the &endTime= parameter so callers can
// page backwards in time. Limit is clamped to the API's per-call max (1500 for
// spot, 1500 for futures).
func (c *RESTClient) FetchKlines(ctx context.Context, symbol string, interval string, limit int, endTime time.Time) ([]types.Kline, error) {
	if limit <= 0 {
		limit = 500
	}
	url := fmt.Sprintf("%s%s?symbol=%s&interval=%s&limit=%d",
		c.baseURL, c.klinesPath(), symbol, interval, limit)
	if !endTime.IsZero() {
		url += fmt.Sprintf("&endTime=%d", endTime.UnixMilli())
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "quant_ba/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch klines: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("binance API error %d: %s", resp.StatusCode, string(body))
	}

	// Binance returns [[number, string, string, ...], ...] — decode as [][]interface{}
	var raw [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode klines: %w", err)
	}

	klines := make([]types.Kline, 0, len(raw))
	for _, r := range raw {
		if len(r) < 7 {
			continue
		}
		k := types.Kline{
			Symbol:    symbol,
			Interval:  interval,
			OpenTime:  time.UnixMilli(toInt64(r[0])),
			CloseTime: time.UnixMilli(toInt64(r[6])),
			Open:      toFloat(r[1]),
			High:      toFloat(r[2]),
			Low:       toFloat(r[3]),
			Close:     toFloat(r[4]),
			Volume:    toFloat(r[5]),
		}
		klines = append(klines, k)
	}
	return klines, nil
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	}
	return 0
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case string:
		var f float64
		fmt.Sscanf(n, "%f", &f)
		return f
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// parseFloat parses a numeric string into a float64.
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
