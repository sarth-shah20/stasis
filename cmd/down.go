package cmd

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/sarth-shah20/stasis/internal/docker"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop and remove services",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := docker.NewManager()
		if err != nil {
			return err
		}
		ctx := context.Background()

		// 1. Stop services
		for name := range cfg.Services {
			if err := mgr.StopAndRemoveContainer(ctx, cfg.Name,name); err != nil {
				fmt.Printf("Error cleaning up %s: %v\n", name, err)
			}
		}

		// 2. Remove Network
		networkName := fmt.Sprintf("stasis-%s", cfg.Name)
		if err := mgr.RemoveNetwork(ctx, networkName); err != nil {
			fmt.Printf("Error removing network: %v\n", err)
		}

		fmt.Println("Environment stopped.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}