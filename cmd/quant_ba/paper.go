package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/colinmyth/quant_ba/internal/types"
	"github.com/spf13/cobra"
)

func init() {
	paperCmd.AddCommand(paperStartCmd)
	rootCmd.AddCommand(paperCmd)
}

var paperCmd = &cobra.Command{
	Use:   "paper",
	Short: "Manage paper trading",
}

var paperStartCmd = &cobra.Command{
	Use:   "start <plugin-path>",
	Short: "Start paper trading for a strategy plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()
		defer app.Loader.Close()

		// Initialize paper portfolio with fake balance.
		app.Portfolio.Init(map[string]types.Balance{
			"USDT": {Asset: "USDT", Free: 10000},
		})

		// Load the plugin binary
		meta, err := app.Loader.Load(args[0])
		if err != nil {
			return fmt.Errorf("load plugin: %w", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		go func() {
			<-sigCh
			fmt.Println("\nShutting down...")
			cancel()
		}()

		fmt.Printf("Starting paper trading: %s (%s)\n", meta.Name, meta.ID)
		fmt.Println("Press Ctrl+C to stop")
		if err := app.PaperExec.Run(ctx, meta.ID); err != nil {
			return err
		}
		return nil
	},
}
