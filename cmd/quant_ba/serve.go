package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start HTTP API server",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("HTTP server not yet implemented.")
		return nil
	},
}
