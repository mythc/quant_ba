package backtest

import (
	"testing"
	"time"

	"github.com/colinmyth/quant_ba/internal/types"
)

func TestSimulateFill_MarketBuy(t *testing.T) {
	sig := &types.Signal{
		Symbol:    "BTCUSDT",
		Direction: types.DirBuy,
		Size:      0.01,
		Type:      types.OrdMarket,
		Price:     50000,
	}
	bar := types.Kline{Open: 50000, High: 50100, Low: 49900, Close: 50050, Volume: 100}

	order := SimulateFill(sig, bar)
	if order == nil {
		t.Fatal("expected order, got nil")
	}
	if order.Status != types.OrdFilled {
		t.Fatalf("expected filled, got %s", order.Status)
	}
	// VWAP = (50000+50100+49900+50050)/4 = 50012.5
	// With slippageBuy: 50012.5 * 1.0005 = 50037.50625
	expectedFill := (50000.0 + 50100 + 49900 + 50050) / 4 * slippageBuy
	if order.FilledPrice < expectedFill*0.99 || order.FilledPrice > expectedFill*1.01 {
		t.Fatalf("expected fill ~%.2f, got %.2f", expectedFill, order.FilledPrice)
	}
}

func TestSimulateFill_MarketSell(t *testing.T) {
	sig := &types.Signal{
		Symbol:    "BTCUSDT",
		Direction: types.DirSell,
		Size:      0.01,
		Type:      types.OrdMarket,
		Price:     50000,
	}
	bar := types.Kline{Open: 50000, High: 50100, Low: 49900, Close: 50050, Volume: 100}

	order := SimulateFill(sig, bar)
	if order == nil {
		t.Fatal("expected order, got nil")
	}
	if order.Status != types.OrdFilled {
		t.Fatalf("expected filled, got %s", order.Status)
	}
	expectedFill := (50000.0 + 50100 + 49900 + 50050) / 4 * slippageSell
	if order.FilledPrice < expectedFill*0.99 || order.FilledPrice > expectedFill*1.01 {
		t.Fatalf("expected fill ~%.2f, got %.2f", expectedFill, order.FilledPrice)
	}
}

func TestSimulateFill_LimitBuyTouched(t *testing.T) {
	sig := &types.Signal{
		Symbol:    "BTCUSDT",
		Direction: types.DirBuy,
		Size:      0.01,
		Type:      types.OrdLimit,
		Price:     49900,
	}
	// Low = 49900, so limit is touched
	bar := types.Kline{Open: 50000, High: 50100, Low: 49900, Close: 50050, Volume: 100}

	order := SimulateFill(sig, bar)
	if order == nil {
		t.Fatal("expected order, got nil")
	}
	if order.Status != types.OrdFilled {
		t.Fatalf("expected filled, got %s", order.Status)
	}
}

func TestSimulateFill_LimitBuyNotTouched(t *testing.T) {
	sig := &types.Signal{
		Symbol:    "BTCUSDT",
		Direction: types.DirBuy,
		Size:      0.01,
		Type:      types.OrdLimit,
		Price:     49800,
	}
	// Low = 49900, limit < low, not touched
	bar := types.Kline{Open: 50000, High: 50100, Low: 49900, Close: 50050, Volume: 100}

	order := SimulateFill(sig, bar)
	if order != nil {
		t.Fatal("expected nil, limit not touched")
	}
}

func TestComputeStats(t *testing.T) {
	trades := []types.Trade{
		{ID: "1", Symbol: "BTCUSDT", Side: types.DirSell, Size: 0.01, PnL: 100, EntryTime: time.Now(), ExitTime: time.Now()},
		{ID: "2", Symbol: "BTCUSDT", Side: types.DirSell, Size: 0.01, PnL: -50, EntryTime: time.Now(), ExitTime: time.Now()},
		{ID: "3", Symbol: "BTCUSDT", Side: types.DirSell, Size: 0.01, PnL: 80, EntryTime: time.Now(), ExitTime: time.Now()},
		{ID: "4", Symbol: "BTCUSDT", Side: types.DirSell, Size: 0.01, PnL: 20, EntryTime: time.Now(), ExitTime: time.Now()},
	}
	equityCurve := []types.EquityPoint{
		{Time: time.Now(), Equity: 10000},
		{Time: time.Now().Add(time.Hour), Equity: 10100},
		{Time: time.Now().Add(2 * time.Hour), Equity: 10050},
		{Time: time.Now().Add(3 * time.Hour), Equity: 10130},
		{Time: time.Now().Add(4 * time.Hour), Equity: 10150},
	}

	result := ComputeStats(trades, equityCurve, 10000)
	if result.TotalTrades != 4 {
		t.Fatalf("expected 4 trades, got %d", result.TotalTrades)
	}
	if result.WinRate != 0.75 {
		t.Fatalf("expected 0.75 win rate, got %.2f", result.WinRate)
	}
	if result.TotalReturn <= 0 {
		t.Fatalf("expected positive return, got %.4f", result.TotalReturn)
	}
	if result.MaxDrawdown < 0 {
		t.Fatalf("expected no negative max drawdown, got %.4f", result.MaxDrawdown)
	}
	// Max drawdown: peak=10100, low=10050 → (100)/10100 ~= 0.0099
	if result.MaxDrawdown < 0 || result.MaxDrawdown > 0.5 {
		t.Fatalf("expected reasonable max drawdown, got %.4f", result.MaxDrawdown)
	}
	if result.ProfitFactor <= 0 {
		t.Fatalf("expected positive profit factor, got %.2f", result.ProfitFactor)
	}
}

func TestFee(t *testing.T) {
	order := &types.Order{
		Symbol:      "BTCUSDT",
		FilledSize:  0.01,
		FilledPrice: 50000,
	}
	fee := Fee(order)
	expectedFee := 0.01 * 50000 * 0.001 // 0.5
	if fee != expectedFee {
		t.Fatalf("expected fee %.4f, got %.4f", expectedFee, fee)
	}
}
