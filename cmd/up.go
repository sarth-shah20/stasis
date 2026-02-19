package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

			// Prepare Volumes
			var volumeBinds []string

			// Get user home directory
			homeDir, _ := os.UserHomeDir()
			projectBase := filepath.Join(homeDir, ".stasis", "volumes", cfg.Name, name)
			//host path: we will use a hidden folder in user's home directory: ~/.stasis/volumes/<project>/<service>
			//container path: YAML will specify
			
			for _, vol := range service.Volumes {
				// Format in YAML: "name:/var/lib/data"
				// We want to map: "~/.stasis/volumes/project/service/name" -> "/var/lib/data"
				
				// Split the volume string "name:path"
				// Note: This is a naive split. Windows paths might break this.
				// For time being, we'll assume Linux/Mac style.
                // Later, we will use a more robust parser.
				parts := strings.Split(vol,":")
				if len(parts) == 2 {
					hostPath := filepath.Join(projectBase, parts[0])
					containerPath := parts[1]
					
					// Ensure host directory exists
					if err := os.MkdirAll(hostPath, 0755); err != nil {
						return fmt.Errorf("failed to create volume dir: %w", err)
					}
					
					// Create the bind string: "/abs/path/on/host:/path/in/container"
					bind := fmt.Sprintf("%s:%s", hostPath, containerPath)
					volumeBinds = append(volumeBinds, bind)
				}
			}

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
			if err := mgr.StartContainer(ctx, name, service.Image, networkName, portMap, service.Environment, volumeBinds); err != nil {
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