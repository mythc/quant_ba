package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/colinmyth/quant_ba/internal/backtest"
	"github.com/spf13/cobra"
)

var (
	btSymbols  string
	btInterval string
	btStart    string
	btEnd      string
	btCapital  float64
	btOut      string
)

func init() {
	rootCmd.AddCommand(backtestCmd)
	backtestCmd.Flags().StringVar(&btSymbols, "symbols", "BTCUSDT", "Comma-separated symbols")
	backtestCmd.Flags().StringVar(&btInterval, "interval", "1h", "Kline interval")
	backtestCmd.Flags().StringVar(&btStart, "start", "2025-01-01", "Start date (YYYY-MM-DD)")
	backtestCmd.Flags().StringVar(&btEnd, "end", "2025-12-31", "End date (YYYY-MM-DD)")
	backtestCmd.Flags().Float64Var(&btCapital, "capital", 10000, "Starting capital in USDT")
	backtestCmd.Flags().StringVar(&btOut, "out", "results/backtest.json", "Output file path")
}

var backtestCmd = &cobra.Command{
	Use:   "backtest run <strategy-id>",
	Short: "Run backtest for a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()

		ls, err := app.Loader.Get(args[0])
		if err != nil {
			return fmt.Errorf("strategy not loaded: %w", err)
		}

		start, err := time.Parse("2006-01-02", btStart)
		if err != nil {
			return fmt.Errorf("invalid start date: %w", err)
		}
		end, err := time.Parse("2006-01-02", btEnd)
		if err != nil {
			return fmt.Errorf("invalid end date: %w", err)
		}
		symbols := strings.Split(btSymbols, ",")

		engine := backtest.NewEngine(app.Cache, app.Risk, app.Portfolio, app.Store)
		result, err := engine.Run(context.Background(), ls, symbols, btInterval, start, end, btCapital)
		if err != nil {
			return err
		}

		if err := engine.SaveResult(result, btOut); err != nil {
			return fmt.Errorf("save result: %w", err)
		}

		fmt.Printf("Backtest complete: %d trades\n", result.TotalTrades)
		fmt.Printf("  Total Return: %.2f%%\n", result.TotalReturn*100)
		fmt.Printf("  Sharpe Ratio: %.3f\n", result.SharpeRatio)
		fmt.Printf("  Max Drawdown: %.2f%%\n", result.MaxDrawdown*100)
		fmt.Printf("  Win Rate:     %.2f%%\n", result.WinRate*100)
		if result.ProfitFactor > 0 {
			fmt.Printf("  Profit Factor: %.2f\n", result.ProfitFactor)
		}
		fmt.Printf("  Results saved to: %s\n", btOut)
		return nil
	},
}
