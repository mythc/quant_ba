package executor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/colinmyth/quant_ba/internal/order"
	"github.com/colinmyth/quant_ba/internal/portfolio"
	"github.com/colinmyth/quant_ba/internal/store"
	"github.com/colinmyth/quant_ba/internal/types"
)

func TestReferencePrice(t *testing.T) {
	bars := []types.Kline{
		{Close: 49000},
		{Close: 50000},
		{Close: 51000},
	}

	tests := []struct {
		name string
		sig  *types.Signal
		bars []types.Kline
		want float64
	}{
		{
			name: "market order uses latest close",
			sig:  &types.Signal{Type: types.OrdMarket, Price: 0},
			bars: bars,
			want: 51000,
		},
		{
			name: "limit price wins over close",
			sig:  &types.Signal{Type: types.OrdLimit, Price: 48000},
			bars: bars,
			want: 48000,
		},
		{
			name: "no bars and no price yields zero",
			sig:  &types.Signal{Type: types.OrdMarket, Price: 0},
			bars: nil,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := referencePrice(tt.sig, tt.bars); got != tt.want {
				t.Fatalf("referencePrice = %.2f, want %.2f", got, tt.want)
			}
		})
	}
}

func TestResolveSize(t *testing.T) {
	positions := map[string]*types.Position{
		"BTCUSDT": {Symbol: "BTCUSDT", Size: 0.03},
	}

	tests := []struct {
		name string
		sig  *types.Signal
		want float64
	}{
		{
			name: "explicit positive size is kept",
			sig:  &types.Signal{Symbol: "BTCUSDT", Size: 0.01},
			want: 0.01,
		},
		{
			name: "zero size closes the whole position",
			sig:  &types.Signal{Symbol: "BTCUSDT", Size: 0},
			want: 0.03,
		},
		{
			name: "zero size with no position resolves to zero",
			sig:  &types.Signal{Symbol: "ETHUSDT", Size: 0},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSize(tt.sig, positions); got != tt.want {
				t.Fatalf("resolveSize = %.8f, want %.8f", got, tt.want)
			}
		})
	}
}

// TestMarketCloseAccounting reproduces a death-cross style close signal that
// carries Size 0. Before the resolveSize fix this placed a no-op 0-size order
// and the position was never closed. Now the whole position is sold and cash
// is credited back.
func TestMarketCloseAccounting(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	portf := portfolio.New(st, false)
	portf.Init(map[string]types.Balance{
		"USDT": {Asset: "USDT", Free: 9500},
		"BTC":  {Asset: "BTC", Free: 0.01},
	})
	portf.OnOrderFilled(types.Order{
		Symbol:      "BTCUSDT",
		Side:        types.DirBuy,
		Type:        types.OrdMarket,
		Size:        0.01,
		FilledSize:  0.01,
		FilledPrice: 50000,
		Status:      types.OrdFilled,
	})

	om := order.NewPaperOrderManager()

	// Close signal: sell with Size 0 ("close the whole position").
	sig := &types.Signal{
		Symbol:    "BTCUSDT",
		Direction: types.DirSell,
		Size:      0,
		Type:      types.OrdMarket,
	}
	bars := []types.Kline{{Close: 51000}}

	port := portf.GetPortfolio()
	sig.Size = resolveSize(sig, port.Positions)
	if sig.Size != 0.01 {
		t.Fatalf("expected resolved size 0.01, got %.8f", sig.Size)
	}
	sig.Price = referencePrice(sig, bars)

	ord, err := om.Place(context.Background(), sig)
	if err != nil {
		t.Fatalf("place order: %v", err)
	}
	portf.OnOrderFilled(*ord)

	p := portf.GetPortfolio()
	if pos := p.Positions["BTCUSDT"]; pos != nil {
		t.Fatalf("expected position closed, got size %.8f", pos.Size)
	}
}

// TestMarketBuyAccounting reproduces the paper-trading flow for a market buy
// signal that carries no price. Before the referencePrice fix the order filled
// at price 0, so no USDT was deducted and the entry price was 0. This guards
// the full strategy -> order -> portfolio accounting chain.
func TestMarketBuyAccounting(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	portf := portfolio.New(st, false)
	portf.Init(map[string]types.Balance{"USDT": {Asset: "USDT", Free: 10000}})

	om := order.NewPaperOrderManager()

	// Market buy emitted by a strategy: no price, just a size.
	sig := &types.Signal{
		Symbol:    "BTCUSDT",
		Direction: types.DirBuy,
		Size:      0.01,
		Type:      types.OrdMarket,
	}
	bars := []types.Kline{{Close: 50000}}
	sig.Price = referencePrice(sig, bars)

	ord, err := om.Place(context.Background(), sig)
	if err != nil {
		t.Fatalf("place order: %v", err)
	}
	if ord.Status != types.OrdFilled {
		t.Fatalf("expected filled order, got %s", ord.Status)
	}
	// Fill price includes buy-side slippage; a fee is charged.
	if ord.FilledPrice <= 50000 {
		t.Fatalf("expected fill price above reference (slippage), got %.2f", ord.FilledPrice)
	}
	if ord.Fee <= 0 {
		t.Fatalf("expected a positive fee, got %.4f", ord.Fee)
	}

	portf.OnOrderFilled(*ord)

	p := portf.GetPortfolio()
	usdt := p.Balances["USDT"]
	wantUSDT := 10000 - ord.FilledSize*ord.FilledPrice - ord.Fee
	if !approxEqual(usdt.Free, wantUSDT) {
		t.Fatalf("expected USDT free %.6f after buy, got %.6f", wantUSDT, usdt.Free)
	}
	pos := p.Positions["BTCUSDT"]
	if pos == nil {
		t.Fatal("expected open BTCUSDT position")
	}
	if !approxEqual(pos.EntryPrice, ord.FilledPrice) {
		t.Fatalf("expected entry price %.6f, got %.6f", ord.FilledPrice, pos.EntryPrice)
	}
}

func approxEqual(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-6
}
