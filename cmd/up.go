package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the development environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		// The config is already loaded by PersistentPreRunE in root.go
		
		fmt.Println("Starting services...")
		for name, service := range cfg.Services {
			fmt.Printf("Found service: %s (Image: %s)\n", name, service.Image)
		}
		
		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}