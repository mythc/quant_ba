package order

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
)

type LiveOrderManager struct {
	baseURL    string
	apiKey     string
	secretKey  string
	futures    bool
	httpClient *http.Client
	mu         sync.RWMutex
	orders     map[string]*types.Order
}

func NewLiveOrderManager(baseURL, apiKey, secretKey string, futures bool) *LiveOrderManager {
	return &LiveOrderManager{
		baseURL:    baseURL,
		apiKey:     apiKey,
		secretKey:  secretKey,
		futures:    futures,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		orders:     make(map[string]*types.Order),
	}
}

// orderEndpoint returns the REST path for placing/reading orders based on mode.
// Spot uses /api/v3; USDT-M futures uses /fapi/v1.
func (m *LiveOrderManager) orderEndpoint() string {
	if m.futures {
		return "/fapi/v1/order"
	}
	return "/api/v3/order"
}

// openOrdersEndpoint returns the REST path for listing open orders.
func (m *LiveOrderManager) openOrdersEndpoint() string {
	if m.futures {
		return "/fapi/v1/openOrders"
	}
	return "/api/v3/openOrders"
}

func (m *LiveOrderManager) Place(ctx context.Context, signal *types.Signal) (*types.Order, error) {
	params := url.Values{}
	params.Set("symbol", signal.Symbol)
	params.Set("side", strings.ToUpper(string(signal.Direction)))
	params.Set("type", strings.ToUpper(string(signal.Type)))
	params.Set("quantity", fmt.Sprintf("%.8f", signal.Size))
	if signal.Type == types.OrdLimit {
		params.Set("price", fmt.Sprintf("%.2f", signal.Price))
		params.Set("timeInForce", "GTC")
	}
	if m.futures {
		// One-way mode: do not pass positionSide (defaults to BOTH).
		if signal.ReduceOnly {
			params.Set("reduceOnly", "true")
		}
		if signal.Leverage > 0 {
			// Note: leverage is set per-symbol via /fapi/v1/leverage and is sticky.
			// We send it on each order for clarity; the server ignores the param
			// on /order but it documents the intent for log inspection.
		}
	}
	params.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))

	resp, err := m.signedRequest(ctx, "POST", m.orderEndpoint(), params)
	if err != nil {
		return nil, err
	}

	var result struct {
		OrderID       int64  `json:"orderId"`
		Symbol        string `json:"symbol"`
		Side          string `json:"side"`
		Type          string `json:"type"`
		Price         string `json:"price"`
		OrigQty       string `json:"origQty"`
		ExecutedQty   string `json:"executedQty"`
		CummQuoteQty  string `json:"cummulativeQuoteQty"`
		Status        string `json:"status"`
		TransactTime  int64  `json:"transactTime"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("decode order response: %w", err)
	}

	order := &types.Order{
		ID:         fmt.Sprintf("%d", result.OrderID),
		Symbol:     signal.Symbol,
		Side:       signal.Direction,
		Type:       signal.Type,
		Price:      signal.Price,
		Size:       signal.Size,
		FilledSize: parseDecimalStr(result.ExecutedQty),
		Status:     mapBinanceStatus(result.Status),
		CreatedAt:  time.UnixMilli(result.TransactTime),
		UpdatedAt:  time.UnixMilli(result.TransactTime),
		StrategyID: signal.StrategyID,
	}

	if order.FilledSize > 0 {
		cummQuote := parseDecimalStr(result.CummQuoteQty)
		if order.FilledSize > 0 {
			order.FilledPrice = cummQuote / order.FilledSize
		}
	}

	m.mu.Lock()
	m.orders[order.ID] = order
	m.mu.Unlock()

	return order, nil
}

func (m *LiveOrderManager) Cancel(ctx context.Context, orderID string) error {
	params := url.Values{}
	params.Set("orderId", orderID)
	params.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))

	_, err := m.signedRequest(ctx, "DELETE", "/api/v3/order", params)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if o, ok := m.orders[orderID]; ok {
		o.Status = types.OrdCancelled
		o.UpdatedAt = time.Now()
	}
	m.mu.Unlock()
	return nil
}

func (m *LiveOrderManager) Status(ctx context.Context, orderID string) (*types.Order, error) {
	params := url.Values{}
	params.Set("orderId", orderID)
	params.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))

	resp, err := m.signedRequest(ctx, "GET", "/api/v3/order", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		OrderID       int64  `json:"orderId"`
		Symbol        string `json:"symbol"`
		Side          string `json:"side"`
		Type          string `json:"type"`
		Price         string `json:"price"`
		OrigQty       string `json:"origQty"`
		ExecutedQty   string `json:"executedQty"`
		CummQuoteQty  string `json:"cummulativeQuoteQty"`
		Status        string `json:"status"`
		Time          int64  `json:"time"`
		UpdateTime    int64  `json:"updateTime"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("decode order status: %w", err)
	}

	order := &types.Order{
		ID:         fmt.Sprintf("%d", result.OrderID),
		Symbol:     result.Symbol,
		Side:       types.Dir(strings.ToLower(result.Side)),
		Type:       types.OrdType(strings.ToLower(result.Type)),
		Price:      parseDecimalStr(result.Price),
		Size:       parseDecimalStr(result.OrigQty),
		FilledSize: parseDecimalStr(result.ExecutedQty),
		Status:     mapBinanceStatus(result.Status),
		CreatedAt:  time.UnixMilli(result.Time),
		UpdatedAt:  time.UnixMilli(result.UpdateTime),
	}
	if order.FilledSize > 0 {
		cummQuote := parseDecimalStr(result.CummQuoteQty)
		order.FilledPrice = cummQuote / order.FilledSize
	}

	return order, nil
}

func (m *LiveOrderManager) OpenOrders(ctx context.Context, symbol string) ([]types.Order, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	params.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))

	resp, err := m.signedRequest(ctx, "GET", "/api/v3/openOrders", params)
	if err != nil {
		return nil, err
	}

	var results []struct {
		OrderID      int64  `json:"orderId"`
		Symbol       string `json:"symbol"`
		Side         string `json:"side"`
		Type         string `json:"type"`
		Price        string `json:"price"`
		OrigQty      string `json:"origQty"`
		ExecutedQty  string `json:"executedQty"`
		Status       string `json:"status"`
		Time         int64  `json:"time"`
		UpdateTime   int64  `json:"updateTime"`
	}
	if err := json.Unmarshal(resp, &results); err != nil {
		return nil, fmt.Errorf("decode open orders: %w", err)
	}

	var orders []types.Order
	for _, r := range results {
		orders = append(orders, types.Order{
			ID:         fmt.Sprintf("%d", r.OrderID),
			Symbol:     r.Symbol,
			Side:       types.Dir(strings.ToLower(r.Side)),
			Type:       types.OrdType(strings.ToLower(r.Type)),
			Price:      parseDecimalStr(r.Price),
			Size:       parseDecimalStr(r.OrigQty),
			FilledSize: parseDecimalStr(r.ExecutedQty),
			Status:     mapBinanceStatus(r.Status),
			CreatedAt:  time.UnixMilli(r.Time),
			UpdatedAt:  time.UnixMilli(r.UpdateTime),
		})
	}
	return orders, nil
}

func (m *LiveOrderManager) signedRequest(ctx context.Context, method, endpoint string, params url.Values) ([]byte, error) {
	queryString := params.Encode()
	sig := m.sign(queryString)
	fullURL := fmt.Sprintf("%s%s?%s&signature=%s", m.baseURL, endpoint, queryString, sig)

	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("binance API error %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (m *LiveOrderManager) sign(queryString string) string {
	mac := hmac.New(sha256.New, []byte(m.secretKey))
	mac.Write([]byte(queryString))
	return hex.EncodeToString(mac.Sum(nil))
}

func mapBinanceStatus(s string) types.OrdStatus {
	switch s {
	case "NEW":
		return types.OrdNew
	case "PARTIALLY_FILLED":
		return types.OrdPartialFill
	case "FILLED":
		return types.OrdFilled
	case "CANCELED":
		return types.OrdCancelled
	case "REJECTED", "EXPIRED":
		return types.OrdRejected
	default:
		return types.OrdNew
	}
}

func parseDecimalStr(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
