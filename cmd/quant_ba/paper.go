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
	rootCmd.AddCommand(paperCmd)
}

var paperCmd = &cobra.Command{
	Use:   "paper start <strategy-id>",
	Short: "Start paper trading for a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()

		// Initialize paper portfolio with fake balance.
		app.Portfolio.Init(map[string]types.Balance{
			"USDT": {Asset: "USDT", Free: 10000},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		go func() {
			<-sigCh
			fmt.Println("\nShutting down...")
			cancel()
		}()

		fmt.Printf("Starting paper trading: %s\n", args[0])
		fmt.Println("Press Ctrl+C to stop")
		if err := app.PaperExec.Run(ctx, args[0]); err != nil {
			return err
		}
		return nil
	},
}
