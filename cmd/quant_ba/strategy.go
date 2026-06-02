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
}

var strategyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List loaded strategies",
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()

		metas := app.Loader.List()
		if len(metas) == 0 {
			fmt.Println("No strategies loaded. Use 'quant_ba strategy load <path>' to load a plugin.")
			return nil
		}
		for _, m := range metas {
			fmt.Printf("  %s  %s  v%s  symbols=%v intervals=%v  (%s)\n",
				m.ID, m.Name, m.Version, m.Symbols, m.Intervals, m.Path)
		}
		return nil
	},
}

var strategyLoadCmd = &cobra.Command{
	Use:   "load <plugin-path>",
	Short: "Load a strategy plugin binary",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()

		meta, err := app.Loader.Load(args[0])
		if err != nil {
			return fmt.Errorf("load strategy: %w", err)
		}
		fmt.Printf("Loaded: %s (%s) v%s\n", meta.Name, meta.ID, meta.Version)
		fmt.Printf("  Symbols: %v\n", meta.Symbols)
		fmt.Printf("  Intervals: %v\n", meta.Intervals)
		return nil
	},
}

var strategyUnloadCmd = &cobra.Command{
	Use:   "unload <strategy-id>",
	Short: "Unload a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()

		if err := app.Loader.Unload(args[0]); err != nil {
			return err
		}
		fmt.Printf("Unloaded: %s\n", args[0])
		return nil
	},
}

var strategyParamsCmd = &cobra.Command{
	Use:   "params <strategy-id>",
	Short: "Show strategy parameters",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Parameters for %s (params query not yet available via RPC)\n", args[0])
		return nil
	},
}
