package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/colinmyth/quant_ba/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start HTTP API server",
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()

		// Override config — always enable server in serve mode.
		port := app.Config.Server.Port
		if port == 0 {
			port = 8080
		}

		server := api.NewServer(port, app.Loader, app.Portfolio)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		go func() {
			<-sigCh
			fmt.Println("\nShutting down...")
			server.Stop(context.Background())
		}()

		fmt.Printf("HTTP server listening on :%d\n", port)
		fmt.Println("Endpoints:")
		fmt.Println("  GET /health")
		fmt.Println("  GET /api/strategies")
		fmt.Println("  GET /api/portfolio")

		if err := server.Start(); err != nil {
			return fmt.Errorf("server: %w", err)
		}
		return nil
	},
}
