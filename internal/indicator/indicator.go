// Package indicator provides pure technical-analysis functions shared by
// strategy plugins. All functions operate on a slice of prices ordered oldest
// to newest and return zero values when there is insufficient data.
package indicator

import "math"

// SMA returns the simple moving average of the last `period` values.
func SMA(values []float64, period int) float64 {
	if period <= 0 || len(values) < period {
		return 0
	}
	var sum float64
	for _, v := range values[len(values)-period:] {
		sum += v
	}
	return sum / float64(period)
}

// EMASeries returns the exponential moving average computed at every index.
// The series is seeded with the first value.
func EMASeries(values []float64, period int) []float64 {
	if period <= 0 || len(values) == 0 {
		return nil
	}
	k := 2.0 / float64(period+1)
	out := make([]float64, len(values))
	out[0] = values[0]
	for i := 1; i < len(values); i++ {
		out[i] = values[i]*k + out[i-1]*(1-k)
	}
	return out
}

// EMA returns the latest exponential moving average value.
func EMA(values []float64, period int) float64 {
	s := EMASeries(values, period)
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1]
}

// RSI returns the Relative Strength Index over the last `period` price changes,
// in the range [0, 100]. Returns 0 when there is not enough data.
func RSI(prices []float64, period int) float64 {
	if period <= 0 || len(prices) < period+1 {
		return 0
	}
	var gain, loss float64
	for i := len(prices) - period; i < len(prices); i++ {
		change := prices[i] - prices[i-1]
		if change >= 0 {
			gain += change
		} else {
			loss -= change
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	if avgLoss == 0 {
		if avgGain == 0 {
			return 50 // flat market
		}
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

// Bollinger returns the middle (SMA), upper and lower bands for the last
// `period` values using `k` standard deviations.
func Bollinger(prices []float64, period int, k float64) (mid, upper, lower float64) {
	if period <= 0 || len(prices) < period {
		return 0, 0, 0
	}
	mid = SMA(prices, period)
	window := prices[len(prices)-period:]
	var sumSq float64
	for _, p := range window {
		d := p - mid
		sumSq += d * d
	}
	sd := math.Sqrt(sumSq / float64(period))
	return mid, mid + k*sd, mid - k*sd
}

// MACD returns the MACD line, signal line and histogram for the latest bar.
func MACD(prices []float64, fast, slow, signal int) (macd, sig, hist float64) {
	if fast <= 0 || slow <= 0 || signal <= 0 || len(prices) < slow+signal {
		return 0, 0, 0
	}
	fastE := EMASeries(prices, fast)
	slowE := EMASeries(prices, slow)
	macdLine := make([]float64, len(prices))
	for i := range prices {
		macdLine[i] = fastE[i] - slowE[i]
	}
	sigLine := EMASeries(macdLine, signal)
	macd = macdLine[len(macdLine)-1]
	sig = sigLine[len(sigLine)-1]
	return macd, sig, macd - sig
}

// Highest returns the maximum of the last `period` values.
func Highest(values []float64, period int) float64 {
	if period <= 0 || len(values) < period {
		return 0
	}
	window := values[len(values)-period:]
	m := window[0]
	for _, v := range window {
		if v > m {
			m = v
		}
	}
	return m
}

// Lowest returns the minimum of the last `period` values.
func Lowest(values []float64, period int) float64 {
	if period <= 0 || len(values) < period {
		return 0
	}
	window := values[len(values)-period:]
	m := window[0]
	for _, v := range window {
		if v < m {
			m = v
		}
	}
	return m
}
