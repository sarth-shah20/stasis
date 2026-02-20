package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/sarth-shah20/stasis/internal/docker"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "List running services",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := docker.NewManager()
		if err != nil {
			return err
		}

		containers, err := mgr.ListContainers(context.Background(), cfg.Name)
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
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}