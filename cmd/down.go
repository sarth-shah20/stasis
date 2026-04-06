package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sarth-shah20/stasis/internal/docker"
	"github.com/sarth-shah20/stasis/internal/provider"
)

var cloudDown bool

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop and remove services",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if cloudDown {
			return runCloudDown(ctx)
		}
		return runLocalDown(ctx)
	},
}

// runLocalDown stops and removes Docker containers (original behavior, unchanged).
func runLocalDown(ctx context.Context) error {
	mgr, err := docker.NewManager()
	if err != nil {
		return err
	}

	// 1. Stop services
	for name := range cfg.Services {
		if err := mgr.StopAndRemoveContainer(ctx, cfg.Name, name); err != nil {
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
}

// runCloudDown deprovisions AWS resources for services with cloud config.
func runCloudDown(ctx context.Context) error {
	p, err := provider.NewAWSProvider(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Tearing down cloud resources... ☁️")

	for name, service := range cfg.Services {
		if service.Cloud.Type == "" {
			continue
		}

		fmt.Printf("\n--- Deprovisioning %s ---\n", name)
		if err := p.Deprovision(ctx, cfg.Name, name); err != nil {
			fmt.Printf("Error deprovisioning %s: %v\n", name, err)
		}
	}

	fmt.Println("\nCloud resources are being deleted. ☁️🗑")
	fmt.Println("Note: RDS and ElastiCache deletion may take a few minutes to complete.")
	return nil
}

func init() {
	downCmd.Flags().BoolVar(&cloudDown, "cloud", false, "Deprovision cloud (AWS) resources instead of local Docker containers")
	rootCmd.AddCommand(downCmd)
}