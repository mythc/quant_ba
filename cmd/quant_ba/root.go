package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgPath string

var rootCmd = &cobra.Command{
	Use:   "quant_ba",
	Short: "Quantitative trading system for Binance",
	Long:  "A multi-strategy quantitative trading platform supporting backtest, paper trading, and live trading on Binance.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "config/default.yaml", "path to config file")
}
