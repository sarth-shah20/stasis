package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"strings"
	"github.com/spf13/cobra"
	"github.com/sarth-shah20/stasis/internal/provider"
)

var cloudUp bool

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the development environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if cloudUp {
			return runCloudUp(ctx)
		}
		return runLocalUp(ctx)
	},
}

// runLocalUp uses LocalProvider to start Docker containers (original behavior).
func runLocalUp(ctx context.Context) error {
	p, err := provider.NewLocalProvider()
	if err != nil {
		return err
	}

	fmt.Println("Stasis starting...")

	results := make(map[string]provider.ConnectionInfo)

	var provisionErrors []string

	for name, service := range cfg.Services {
		if service.Cloud.Type == "" {
        	continue
    	}
		info, err := p.Provision(ctx, cfg.Name, name, service)
		if err != nil {
			fmt.Printf("  ❌ Failed to provision %s: %v\n", name, err)
			provisionErrors = append(provisionErrors, name)
			continue
		}
		results[name] = info
	}
	if len(provisionErrors) > 0 {
    	fmt.Printf("\nWarning: failed to provision: %s\n", strings.Join(provisionErrors, ", "))
	}

	fmt.Println("\nEnvironment is UP! 🚀")
	printConnectionTable(results)
	return nil
}

// runCloudUp uses AWSProvider to provision managed cloud services.
func runCloudUp(ctx context.Context) error {
	p, err := provider.NewAWSProvider(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Stasis starting (cloud mode)... ☁️")

	results := make(map[string]provider.ConnectionInfo)

	var provisionErrors []string

	for name, service := range cfg.Services {
		// Only provision services that have a cloud config
		if service.Cloud.Type == "" {
			fmt.Printf("\n--- Skipping %s (no cloud config) ---\n", name)
			continue
		}

		info, err := p.Provision(ctx, cfg.Name, name, service)
		if err != nil {
			fmt.Printf("  ❌ Failed to provision %s: %v\n", name, err)
        	provisionErrors = append(provisionErrors, name)
        	continue
		}
		results[name] = info
	}

	if len(provisionErrors) > 0 {
    fmt.Printf("\nWarning: failed to provision: %s\n", strings.Join(provisionErrors, ", "))
}

	fmt.Println("\nCloud environment provisioning initiated! ☁️🚀")
	printConnectionTable(results)
	fmt.Println("\nNote: Some resources (RDS, ElastiCache) may take a few minutes to become available.")
	fmt.Println("Check status with: stasis status --cloud")
	return nil
}

// printConnectionTable prints a summary table of provisioned service endpoints.
func printConnectionTable(results map[string]provider.ConnectionInfo) {
	if len(results) == 0 {
		return
	}

	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tENDPOINT\tPORT")

	for name, info := range results {
		port := ""
		if info.Port != 0 {
			port = fmt.Sprintf("%d", info.Port)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", name, info.Endpoint, port)
	}
	w.Flush()
}

func init() {
	upCmd.Flags().BoolVar(&cloudUp, "cloud", false, "Provision cloud (AWS) resources instead of local Docker containers")
	rootCmd.AddCommand(upCmd)
}