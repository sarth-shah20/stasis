package cmd

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/sarth-shah20/stasis/internal/docker"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the development environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Initialize Docker Manager
		mgr, err := docker.NewManager()
		if err != nil {
			return fmt.Errorf("could not connect to docker: %w", err)
		}
		
		// 2. Create a Context
		// Contexts are used to handle timeouts and cancellations.
		// Background() means "no timeout, run until I say stop".
		ctx := context.Background()

		fmt.Println("Stasis starting...")

		// 3. Loop through services and pull images
		for name, service := range cfg.Services {
			fmt.Printf("Preparing service: %s\n", name)
			
			if err := mgr.EnsureImage(ctx, service.Image); err != nil {
				return fmt.Errorf("failed to setup service %s: %w", name, err)
			}
		}
		
		fmt.Println("All images pulled successfully!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}