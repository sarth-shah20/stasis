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

		// 1. Create Network
		networkName := fmt.Sprintf("stasis-%s", cfg.Name)
		if err := mgr.EnsureNetwork(ctx, networkName); err != nil {
			return err
		}

		// 2. Loop through services
		for name, service := range cfg.Services {
			fmt.Printf("\n--- Setting up %s ---\n", name)
			
			// Pull Image
			if err := mgr.EnsureImage(ctx, service.Image); err != nil {
				return fmt.Errorf("failed to pull image: %w", err)
			}

			// Get the first port mapping (simplified for now)
			portMap := ""
			if len(service.Ports) > 0 {
				portMap = service.Ports[0]
			}

			// Start Container
			if err := mgr.StartContainer(ctx, name, service.Image, networkName, portMap); err != nil {
				return fmt.Errorf("failed to start %s: %w", name, err)
			}
		}
		
		fmt.Println("\nEnvironment is UP! 🚀")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}