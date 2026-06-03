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
	Use:   "start <plugin-path>",
	Short: "Start live trading for a strategy plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := NewApp(cmd.Flag("config").Value.String())
		if err != nil {
			return err
		}
		defer app.Store.Close()
		defer app.Loader.Close()

		// Load plugin
		meta, err := app.Loader.Load(args[0])
		if err != nil {
			return fmt.Errorf("load plugin: %w", err)
		}

		fmt.Printf("Starting live trading: %s (%s)\n", meta.Name, meta.ID)
		fmt.Println("Live trading requires valid API keys in config/default.yaml")
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
