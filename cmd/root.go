package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/sarth-shah20/stasis/internal/config" // Import your internal package
)

// Global variable to hold the loaded configuration
var cfg *config.Config

var rootCmd = &cobra.Command{
	Use:   "stasis",
	Short: "Stasis: Local environment management",
	// PersistentPreRunE runs before ANY command (up, down, etc.)
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// We hardcode "stasis.yaml" for now. Later we can make this a flag.
		loadedConfig, err := config.Load("stasis.yaml")
		if err != nil {
			return err
		}
		
		cfg = loadedConfig
		fmt.Printf("Loaded config for project: %s\n", cfg.Name)
		return nil
	},
}


// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Here we will define global flags (like --verbose or --config) later.
}