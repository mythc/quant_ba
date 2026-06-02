package market

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
	"github.com/gorilla/websocket"
)

const defaultWSURL = "wss://stream.binance.com:9443/ws"

type WSClient struct {
	url         string
	conn        *websocket.Conn
	mu          sync.Mutex
	subscribers map[string]map[chan types.Kline]struct{} // key: symbol@interval
	done        chan struct{}
}

func NewWSClient(url string) *WSClient {
	if url == "" {
		url = defaultWSURL
	}
	return &WSClient{
		url:         url,
		subscribers: make(map[string]map[chan types.Kline]struct{}),
		done:        make(chan struct{}),
	}
}

// Connect establishes the WebSocket connection and starts the read loop.
func (w *WSClient) Connect(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, w.url, nil)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}
	w.conn = conn

	go w.readLoop()
	go w.heartbeat(ctx)
	return nil
}

// SubscribeKline subscribes to kline stream, returns a channel receiving completed bars.
func (w *WSClient) SubscribeKline(ctx context.Context, symbol string, interval string) (<-chan types.Kline, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	stream := fmt.Sprintf("%s@kline_%s", strings.ToLower(symbol), interval)
	ch := make(chan types.Kline, 100)

	if _, ok := w.subscribers[stream]; !ok {
		w.subscribers[stream] = make(map[chan types.Kline]struct{})
	}
	w.subscribers[stream][ch] = struct{}{}

	// Subscribe on the wire
	msg := map[string]interface{}{
		"method": "SUBSCRIBE",
		"params": []string{stream},
		"id":     time.Now().UnixMilli(),
	}
	if err := w.conn.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("ws subscribe: %w", err)
	}

	return ch, nil
}

// Unsubscribe removes a subscription.
func (w *WSClient) Unsubscribe(symbol string, interval string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	stream := fmt.Sprintf("%s@kline_%s", strings.ToLower(symbol), interval)

	msg := map[string]interface{}{
		"method": "UNSUBSCRIBE",
		"params": []string{stream},
		"id":     time.Now().UnixMilli(),
	}
	return w.conn.WriteJSON(msg)
}

// SubscribeOrderBook is a placeholder for order book subscription (not used by kline strategies yet).
func (w *WSClient) SubscribeOrderBook(ctx context.Context, symbol string, depth int) (<-chan types.OrderBook, error) {
	return nil, fmt.Errorf("order book subscription not yet implemented")
}

// Close shuts down the WebSocket client.
func (w *WSClient) Close() error {
	close(w.done)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn != nil {
		return w.conn.Close()
	}
	return nil
}

func (w *WSClient) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			if w.conn != nil {
				w.conn.WriteMessage(websocket.PingMessage, nil)
			}
			w.mu.Unlock()
		case <-w.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (w *WSClient) readLoop() {
	for {
		_, msg, err := w.conn.ReadMessage()
		if err != nil {
			w.mu.Lock()
			for _, subs := range w.subscribers {
				for ch := range subs {
					close(ch)
				}
			}
			w.subscribers = make(map[string]map[chan types.Kline]struct{})
			w.mu.Unlock()
			return
		}

		var event struct {
			Stream string          `json:"stream"`
			Data   json.RawMessage `json:"data"`
		}

		// Try parsing as combined stream event first
		if err := json.Unmarshal(msg, &event); err != nil || event.Stream == "" {
			// Parse as direct kline data
			var klineData struct {
				E int64 `json:"E"`
				K struct {
					T  int64  `json:"t"`
					Tc int64  `json:"T"`
					S  string `json:"s"`
					I  string `json:"i"`
					O  string `json:"o"`
					H  string `json:"h"`
					L  string `json:"l"`
					C  string `json:"c"`
					V  string `json:"v"`
					X  bool   `json:"x"`
				} `json:"k"`
			}
			if err := json.Unmarshal(msg, &klineData); err != nil {
				continue
			}
			k := klineData.K
			if !k.X {
				continue // only emit closed bars
			}
			bar := types.Kline{
				Symbol:    strings.ToUpper(k.S),
				Interval:  k.I,
				OpenTime:  time.UnixMilli(k.T),
				CloseTime: time.UnixMilli(k.Tc),
				Open:      parseFloat(k.O),
				High:      parseFloat(k.H),
				Low:       parseFloat(k.L),
				Close:     parseFloat(k.C),
				Volume:    parseFloat(k.V),
			}
			w.broadcast(k.S, k.I, bar)
			continue
		}

		// Handle combined stream event
		var klineData struct {
			E int64 `json:"E"`
			K struct {
				T  int64  `json:"t"`
				Tc int64  `json:"T"`
				S  string `json:"s"`
				I  string `json:"i"`
				O  string `json:"o"`
				H  string `json:"h"`
				L  string `json:"l"`
				C  string `json:"c"`
				V  string `json:"v"`
				X  bool   `json:"x"`
			} `json:"k"`
		}
		if err := json.Unmarshal(event.Data, &klineData); err != nil {
			continue
		}
		k := klineData.K
		if !k.X {
			continue
		}
		bar := types.Kline{
			Symbol:    strings.ToUpper(k.S),
			Interval:  k.I,
			OpenTime:  time.UnixMilli(k.T),
			CloseTime: time.UnixMilli(k.Tc),
			Open:      parseFloat(k.O),
			High:      parseFloat(k.H),
			Low:       parseFloat(k.L),
			Close:     parseFloat(k.C),
			Volume:    parseFloat(k.V),
		}
		w.broadcast(k.S, k.I, bar)
	}
}

func (w *WSClient) broadcast(symbol, interval string, bar types.Kline) {
	w.mu.Lock()
	defer w.mu.Unlock()
	streamKey := fmt.Sprintf("%s@kline_%s", strings.ToLower(symbol), interval)
	if subs, ok := w.subscribers[streamKey]; ok {
		for ch := range subs {
			select {
			case ch <- bar:
			default:
				// drop if subscriber is slow
			}
		}
	}
}
