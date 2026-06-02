package backtest

import (
	"math"

	"github.com/colinmyth/quant_ba/internal/types"
)

type BacktestResult struct {
	TotalReturn  float64            `json:"total_return"`
	SharpeRatio  float64            `json:"sharpe_ratio"`
	MaxDrawdown  float64            `json:"max_drawdown"`
	WinRate      float64            `json:"win_rate"`
	ProfitFactor float64            `json:"profit_factor"`
	TotalTrades  int                `json:"total_trades"`
	TradeLog     []types.Trade      `json:"trade_log"`
	EquityCurve  []types.EquityPoint `json:"equity_curve"`
}

func ComputeStats(trades []types.Trade, equityCurve []types.EquityPoint, startCapital float64) *BacktestResult {
	r := &BacktestResult{
		TotalTrades: len(trades),
		TradeLog:    trades,
		EquityCurve: equityCurve,
	}

	if len(trades) == 0 || startCapital == 0 {
		return r
	}

	// Final equity
	var finalEquity float64
	if len(equityCurve) > 0 {
		finalEquity = equityCurve[len(equityCurve)-1].Equity
	}

	r.TotalReturn = (finalEquity - startCapital) / startCapital

	// Win rate, profit factor
	var wins int
	var grossProfit, grossLoss float64
	for _, t := range trades {
		if t.PnL > 0 {
			wins++
			grossProfit += t.PnL
		} else {
			grossLoss += -t.PnL
		}
	}
	r.WinRate = float64(wins) / float64(len(trades))
	if grossLoss > 0 {
		r.ProfitFactor = grossProfit / grossLoss
	}

	// Max drawdown
	peak := startCapital
	maxDD := 0.0
	for _, p := range equityCurve {
		if p.Equity > peak {
			peak = p.Equity
		}
		dd := (peak - p.Equity) / peak
		if dd > maxDD {
			maxDD = dd
		}
	}
	r.MaxDrawdown = maxDD

	// Sharpe ratio (annualized for daily bars)
	if len(equityCurve) > 1 {
		returns := make([]float64, len(equityCurve)-1)
		for i := 1; i < len(equityCurve); i++ {
			if equityCurve[i-1].Equity > 0 {
				returns[i-1] = (equityCurve[i].Equity - equityCurve[i-1].Equity) / equityCurve[i-1].Equity
			}
		}
		mean := avg(returns)
		std := stdDev(returns, mean)
		if std > 0 {
			r.SharpeRatio = mean / std * math.Sqrt(252) // annualized (daily bars)
		}
	}

	return r
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func stdDev(vals []float64, mean float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	sumSq := 0.0
	for _, v := range vals {
		sumSq += (v - mean) * (v - mean)
	}
	return math.Sqrt(sumSq / float64(len(vals)-1))
}
