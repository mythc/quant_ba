package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(liveCmd)
	liveCmd.AddCommand(liveStartCmd)
	liveCmd.AddCommand(liveStopCmd)
	liveCmd.AddCommand(liveStatusCmd)
}

var liveCmd = &cobra.Command{
	Use:   "live",
	Short: "Manage live trading",
}

var liveStartCmd = &cobra.Command{
	Use:   "start <strategy>",
	Short: "Start live trading for a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Starting live trading: %s (not yet implemented)\n", args[0])
		return nil
	},
}

var liveStopCmd = &cobra.Command{
	Use:   "stop <strategy>",
	Short: "Stop live trading for a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Stopping live trading: %s (not yet implemented)\n", args[0])
		return nil
	},
}

var liveStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show running strategy status",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("No strategies running.")
		return nil
	},
}
