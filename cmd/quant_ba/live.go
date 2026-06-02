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
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()

		// For now, reuse PaperExecutor as the live executor.
		// In production, swap orderMgr to LiveOM.
		fmt.Printf("Starting live trading: %s\n", args[0])
		fmt.Println("Live trading requires valid API keys in config/default.yaml")
		fmt.Println("(Live executor reuses paper executor for now — swap PaperOM to LiveOM for real trading)")
		return nil
	},
}

var liveStopCmd = &cobra.Command{
	Use:   "stop <strategy>",
	Short: "Stop live trading for a strategy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()

		if err := app.PaperExec.Stop(args[0]); err != nil {
			return err
		}
		fmt.Printf("Stopped: %s\n", args[0])
		return nil
	},
}

var liveStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show running strategy status",
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()

		// Show all loaded strategies and their status.
		metas := app.Loader.List()
		if len(metas) == 0 {
			fmt.Println("No strategies loaded.")
			return nil
		}
		for _, m := range metas {
			status := app.PaperExec.Status(m.ID)
			if status != nil && status.Running {
				fmt.Printf("  %s: running (mode=%s, since=%s)\n", m.ID, status.Mode, status.StartedAt)
			} else {
				fmt.Printf("  %s: stopped\n", m.ID)
			}
		}
		return nil
	},
}
