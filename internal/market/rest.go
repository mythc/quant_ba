package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	return &RESTClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
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
func (c *RESTClient) FetchKlines(ctx context.Context, symbol string, interval string, limit int) ([]types.Kline, error) {
	if limit <= 0 {
		limit = 500
	}
	url := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&limit=%d",
		c.baseURL, symbol, interval, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch klines: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("binance API error %d: %s", resp.StatusCode, string(body))
	}

	var raw []klineRaw
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode klines: %w", err)
	}

	klines := make([]types.Kline, len(raw))
	for i, r := range raw {
		klines[i] = types.Kline{
			Symbol:    symbol,
			Interval:  interval,
			OpenTime:  time.UnixMilli(r.OpenTime),
			CloseTime: time.UnixMilli(r.CloseTime),
			Open:      parseFloat(r.Open),
			High:      parseFloat(r.High),
			Low:       parseFloat(r.Low),
			Close:     parseFloat(r.Close),
			Volume:    parseFloat(r.Volume),
		}
	}
	return klines, nil
}

// parseFloat parses a numeric string into a float64.
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
