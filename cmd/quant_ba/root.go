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

var configCmd = &cobra.Command{
	Use:   "config show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgFlag := cmd.Flag("config").Value.String()
		app, err := NewApp(cfgFlag)
		if err != nil {
			return err
		}
		defer app.Store.Close()

		fmt.Printf("Exchange: %s (%s)\n", app.Config.Exchange.Name, app.Config.Exchange.BaseURL)
		fmt.Printf("Testnet: %v\n", app.Config.Exchange.Testnet)
		fmt.Printf("Store: %s\n", app.Config.Store.Path)
		fmt.Printf("Server port: %d\n", app.Config.Server.Port)
		fmt.Printf("Risk - max position: %.0f%%\n", app.Config.Risk.Basic.MaxPositionPct*100)
		fmt.Printf("Risk - daily loss limit: %.0f%%\n", app.Config.Risk.CircuitBreaker.DailyLossPct*100)
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "config/default.yaml", "path to config file")
	rootCmd.AddCommand(configCmd)
}
