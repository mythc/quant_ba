package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(strategyCmd)
	strategyCmd.AddCommand(strategyListCmd)
	strategyCmd.AddCommand(strategyLoadCmd)
	strategyCmd.AddCommand(strategyUnloadCmd)
	strategyCmd.AddCommand(strategyParamsCmd)
}

var strategyCmd = &cobra.Command{
	Use:   "strategy",
	Short: "Manage trading strategies",
	Long:  "List, load, unload, and configure strategy plugins.",
}

var strategyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available strategies",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("No strategies loaded yet. Place .so plugin binaries in plugins/")
		return nil
	},
}

var strategyLoadCmd = &cobra.Command{
	Use:   "load <name>",
	Short: "Load a strategy plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Loading strategy: %s (not yet implemented)\n", args[0])
		return nil
	},
}

var strategyUnloadCmd = &cobra.Command{
	Use:   "unload <name>",
	Short: "Unload a strategy plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Unloading strategy: %s (not yet implemented)\n", args[0])
		return nil
	},
}

var strategyParamsCmd = &cobra.Command{
	Use:   "params <name>",
	Short: "Show or update strategy parameters",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Strategy params for: %s (not yet implemented)\n", args[0])
		return nil
	},
}
