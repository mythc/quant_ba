package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(backtestCmd)
}

var backtestCmd = &cobra.Command{
	Use:   "backtest",
	Short: "Run backtest for a strategy",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Backtest not yet implemented.")
		return nil
	},
}
