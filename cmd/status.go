package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/sarth-shah20/stasis/internal/docker"
	"github.com/sarth-shah20/stasis/internal/provider"
)

var cloudStatus bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "List running services",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if cloudStatus {
			return runCloudStatus(ctx)
		}
		return runLocalStatus(ctx)
	},
}

// runLocalStatus shows Docker container status (original behavior, unchanged).
func runLocalStatus(ctx context.Context) error {
	mgr, err := docker.NewManager()
	if err != nil {
		return err
	}

	containers, err := mgr.ListContainers(ctx, cfg.Name)
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		fmt.Println("No stasis services found.")
		return nil
	}

	// Use tabwriter to print pretty columns
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tIMAGE\tSTATUS\tPORTS")

	for _, c := range containers {
		// c.Names[0] is usually "/stasis-postgres", strip the slash
		name := c.Names[0][1:]

		// Format ports (simplified)
		ports := ""
		for _, p := range c.Ports {
			if p.PublicPort != 0 {
				ports += fmt.Sprintf("%d->%d/tcp ", p.PublicPort, p.PrivatePort)
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, c.Image, c.Status, ports)
	}
	w.Flush()

	return nil
}

// runCloudStatus shows AWS resource status for services with cloud config.
func runCloudStatus(ctx context.Context) error {
	p, err := provider.NewAWSProvider(ctx)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tTYPE\tSTATUS")

	found := false
	for name, service := range cfg.Services {
		if service.Cloud.Type == "" {
			continue
		}

		status, err := p.Status(ctx, cfg.Name, name)
		if err != nil {
			status = fmt.Sprintf("error: %v", err)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", name, service.Cloud.Type, status)
		found = true
	}

	w.Flush()

	if !found {
		fmt.Println("No services with cloud configuration found in stasis.yaml.")
	}

	return nil
}

func init() {
	statusCmd.Flags().BoolVar(&cloudStatus, "cloud", false, "Show cloud (AWS) resource status instead of local Docker containers")
	rootCmd.AddCommand(statusCmd)
}