package portfolio

import (
	"testing"

	"github.com/colinmyth/quant_ba/internal/types"
)

func TestOnOrderFilled_Buy(t *testing.T) {
	svc := &Service{
		balances: map[string]types.Balance{
			"USDT": {Asset: "USDT", Free: 10000},
		},
		positions: make(map[string]*types.Position),
	}

	order := types.Order{
		Symbol:      "BTCUSDT",
		Side:        types.DirBuy,
		Type:        types.OrdMarket,
		Size:        0.01,
		FilledSize:  0.01,
		FilledPrice: 50000,
		Status:      types.OrdFilled,
	}

	svc.OnOrderFilled(order)

	// Check position created
	pos := svc.positions["BTCUSDT"]
	if pos == nil {
		t.Fatal("expected position to be created")
	}
	if pos.Size != 0.01 {
		t.Fatalf("expected size 0.01, got %.8f", pos.Size)
	}
	if pos.EntryPrice != 50000 {
		t.Fatalf("expected entry 50000, got %.2f", pos.EntryPrice)
	}

	// Check balance deducted
	usdt := svc.balances["USDT"]
	expectedFree := 10000.0 - 500.0 // 0.01 * 50000
	if usdt.Free != expectedFree {
		t.Fatalf("expected USDT free %.2f, got %.2f", expectedFree, usdt.Free)
	}
}

func TestOnOrderFilled_Sell(t *testing.T) {
	svc := &Service{
		balances: map[string]types.Balance{
			"USDT": {Asset: "USDT", Free: 9500},
			"BTC":  {Asset: "BTC", Free: 0.01},
		},
		positions: map[string]*types.Position{
			"BTCUSDT": {
				Symbol:       "BTCUSDT",
				Side:         types.DirBuy,
				Size:         0.01,
				EntryPrice:   50000,
				CurrentPrice: 51000,
			},
		},
	}

	order := types.Order{
		Symbol:      "BTCUSDT",
		Side:        types.DirSell,
		Type:        types.OrdMarket,
		Size:        0.01,
		FilledSize:  0.01,
		FilledPrice: 51000,
		Status:      types.OrdFilled,
	}

	svc.OnOrderFilled(order)

	// Position should be closed (size ≤ 0)
	pos := svc.positions["BTCUSDT"]
	if pos != nil {
		t.Fatal("expected position to be closed")
	}

	// USDT balance should increase by 510
	usdt := svc.balances["USDT"]
	expectedFree := 9500.0 + 510.0
	if usdt.Free != expectedFree {
		t.Fatalf("expected USDT free %.2f, got %.2f", expectedFree, usdt.Free)
	}
}

func TestEquity(t *testing.T) {
	svc := &Service{
		balances: map[string]types.Balance{
			"USDT": {Asset: "USDT", Free: 9000, Locked: 0},
		},
		positions: map[string]*types.Position{
			"BTCUSDT": {
				Symbol:       "BTCUSDT",
				Size:         0.01,
				CurrentPrice: 50000,
				EntryPrice:   49000,
			},
		},
	}

	equity := svc.Equity()
	expected := 9000.0 + 0.01*50000.0 // 9500
	if equity != expected {
		t.Fatalf("expected equity %.2f, got %.2f", expected, equity)
	}
}
