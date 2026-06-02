package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(paperCmd)
}

var paperCmd = &cobra.Command{
	Use:   "paper",
	Short: "Manage paper trading",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Paper trading not yet implemented.")
		return nil
	},
}
