package indicator

import (
	"math"
	"testing"
)

func approx(a, b, eps float64) bool { return math.Abs(a-b) < eps }

func TestSMA(t *testing.T) {
	if got := SMA([]float64{1, 2, 3, 4, 5}, 5); got != 3 {
		t.Fatalf("SMA = %v, want 3", got)
	}
	if got := SMA([]float64{2, 4, 6}, 2); got != 5 {
		t.Fatalf("SMA last2 = %v, want 5", got)
	}
	if got := SMA([]float64{1, 2}, 5); got != 0 {
		t.Fatalf("SMA insufficient = %v, want 0", got)
	}
}

func TestEMA(t *testing.T) {
	// Constant series → EMA equals the constant.
	if got := EMA([]float64{10, 10, 10, 10}, 3); !approx(got, 10, 1e-9) {
		t.Fatalf("EMA constant = %v, want 10", got)
	}
}

func TestRSI(t *testing.T) {
	// Strictly increasing prices → no losses → RSI 100.
	up := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	if got := RSI(up, 14); !approx(got, 100, 1e-9) {
		t.Fatalf("RSI rising = %v, want 100", got)
	}
	// Strictly decreasing prices → no gains → RSI 0.
	down := []float64{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	if got := RSI(down, 14); !approx(got, 0, 1e-9) {
		t.Fatalf("RSI falling = %v, want 0", got)
	}
}

func TestBollinger(t *testing.T) {
	prices := []float64{2, 4, 6, 8, 10}
	mid, upper, lower := Bollinger(prices, 5, 2)
	if mid != 6 {
		t.Fatalf("Bollinger mid = %v, want 6", mid)
	}
	if upper <= mid || lower >= mid {
		t.Fatalf("expected lower < mid < upper, got %v < %v < %v", lower, mid, upper)
	}
}

func TestMACD(t *testing.T) {
	prices := make([]float64, 60)
	for i := range prices {
		prices[i] = float64(i) // steady uptrend
	}
	macd, sig, hist := MACD(prices, 12, 26, 9)
	// In a steady uptrend the MACD line stays above its signal line.
	if macd <= sig {
		t.Fatalf("expected macd > signal in uptrend, got macd=%v sig=%v", macd, sig)
	}
	if !approx(hist, macd-sig, 1e-9) {
		t.Fatalf("hist != macd-sig: %v vs %v", hist, macd-sig)
	}
}

func TestHighestLowest(t *testing.T) {
	v := []float64{3, 1, 4, 1, 5, 9, 2}
	if got := Highest(v, 7); got != 9 {
		t.Fatalf("Highest = %v, want 9", got)
	}
	if got := Lowest(v, 7); got != 1 {
		t.Fatalf("Lowest = %v, want 1", got)
	}
}
