package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/sarth-shah20/stasis/internal/docker"
	"github.com/sarth-shah20/stasis/internal/snapshot"
	"github.com/sarth-shah20/stasis/internal/utils"
	"github.com/sarth-shah20/stasis/internal/remote"
)

// Helper to extract env vars like "POSTGRES_DB=devdb" -> "devdb"
func getEnvValue(envs[]string, key string) string {
	prefix := key + "="
	for _, e := range envs {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage environment snapshots",
}

var saveCmd = &cobra.Command{
	Use:   "save [snapshot-name]",
	Short: "Save the current state of the environment",
	Args:  cobra.ExactArgs(1), // Forces the user to provide exactly 1 argument
	RunE: func(cmd *cobra.Command, args []string) error {
		snapshotName := args[0]
		ctx := context.Background()

		mgr, err := docker.NewManager()
		if err != nil {
			return err
		}

		destDir, err := utils.GetSnapshotDir(cfg.Name, snapshotName)
		if err != nil {
			return err
		}

		// Create the snapshot directory
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("failed to create snapshot directory: %w", err)
		}

		manifest := snapshot.Manifest{
			ProjectName:  cfg.Name,
			SnapshotName: snapshotName,
			Timestamp:    time.Now(),
			Services:     make(map[string]snapshot.ServiceMeta),
		}

		fmt.Printf("Saving snapshot '%s'...\n", snapshotName)

		for name, service := range cfg.Services {
			containerName := fmt.Sprintf("stasis-%s-%s", cfg.Name, name)

			// Auto-detect Postgres
			if strings.Contains(strings.ToLower(service.Image), "postgres") {
				fmt.Printf("  -> Snapshotting Postgres database (%s)...\n", name)
				
				dbName := getEnvValue(service.Environment, "POSTGRES_DB")
				if dbName == "" {
					dbName = "postgres" // default
				}

				pSnap := snapshot.NewPostgresSnapshotter(mgr, dbName)
				
				// We pass destDir. The snapshotter will create dump.sql inside it.
				if err := pSnap.Save(ctx, containerName, destDir); err != nil {
					return fmt.Errorf("failed to save %s: %w", name, err)
				}

				manifest.Services[name] = snapshot.ServiceMeta{
					Strategy: "postgres",
					Image:    service.Image,
				}
			} else {
				fmt.Printf("  -> Skipping %s (no smart snapshotter configured yet)\n", name)
			}
		}

		if err := snapshot.SaveManifest(destDir, manifest); err != nil {
			return err
		}

		fmt.Println("Snapshot saved successfully! 📸")
		return nil
	},
}

var loadCmd = &cobra.Command{
	Use:   "load [snapshot-name]",
	Short: "Restore the environment to a saved snapshot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		snapshotName := args[0]
		ctx := context.Background()

		mgr, err := docker.NewManager()
		if err != nil {
			return err
		}

		srcDir, err := utils.GetSnapshotDir(cfg.Name, snapshotName)
		if err != nil {
			return err
		}

		manifest, err := snapshot.LoadManifest(srcDir)
		if err != nil {
			return fmt.Errorf("failed to load snapshot manifest (does it exist?): %w", err)
		}

		fmt.Printf("Loading snapshot '%s' (taken at %s)...\n", snapshotName, manifest.Timestamp.Format(time.RFC822))

		for name, meta := range manifest.Services {
			containerName := fmt.Sprintf("stasis-%s-%s", cfg.Name, name)

			if meta.Strategy == "postgres" {
				fmt.Printf("  -> Restoring Postgres database (%s)...\n", name)
				
				// We need the DB name from the current config to restore properly
				dbName := getEnvValue(cfg.Services[name].Environment, "POSTGRES_DB")
				if dbName == "" {
					dbName = "postgres"
				}

				pSnap := snapshot.NewPostgresSnapshotter(mgr, dbName)
				if err := pSnap.Load(ctx, containerName, srcDir); err != nil {
					return fmt.Errorf("failed to load %s: %w", name, err)
				}
			}
		}

		fmt.Println("Snapshot loaded successfully! ⏪")
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available snapshots for this project",
	RunE: func(cmd *cobra.Command, args[]string) error {
		// We need the base snapshot directory for the project
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		projectSnapshotsDir := filepath.Join(home, ".stasis", "snapshots", cfg.Name)

		// Read the directory contents
		entries, err := os.ReadDir(projectSnapshotsDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No snapshots found for this project.")
				return nil
			}
			return fmt.Errorf("failed to read snapshots directory: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "SNAPSHOT NAME\tCREATED AT\tSERVICES")

		validSnapshots := 0
		for _, entry := range entries {
			if !entry.IsDir() {
				continue // Skip random files, we only care about snapshot directories
			}

			snapName := entry.Name()
			srcDir := filepath.Join(projectSnapshotsDir, snapName)
			
			// Try to load the manifest
			manifest, err := snapshot.LoadManifest(srcDir)
			if err != nil {
				// If a folder doesn't have a valid manifest, we skip it silently
				continue
			}

			// Extract service names into a comma-separated string
			var svcNames[]string
			for svcName := range manifest.Services {
				svcNames = append(svcNames, svcName)
			}
			joinedServices := strings.Join(svcNames, ", ")

			// Format the timestamp nicely
			timeStr := manifest.Timestamp.Format(time.RFC822)

			fmt.Fprintf(w, "%s\t%s\t%s\n", snapName, timeStr, joinedServices)
			validSnapshots++
		}
		
		w.Flush()

		if validSnapshots == 0 {
			fmt.Println("No valid snapshots found for this project.")
		}

		return nil
	},
}


var pushCmd = &cobra.Command{
	Use:   "push [snapshot-name]",
	Short: "Push a local snapshot to the remote S3 bucket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		snapshotName := args[0]
		ctx := context.Background()

		if cfg.Remote.S3.Bucket == "" {
			return fmt.Errorf("remote S3 bucket is not configured in stasis.yaml")
		}

		localDir, err := utils.GetSnapshotDir(cfg.Name, snapshotName)
		if err != nil {
			return err
		}

		// Verify the snapshot exists locally before pushing
		if _, err := os.Stat(localDir); os.IsNotExist(err) {
			return fmt.Errorf("local snapshot '%s' does not exist. Run 'stasis snapshot save' first", snapshotName)
		}

		fmt.Printf("Connecting to S3 bucket '%s'...\n", cfg.Remote.S3.Bucket)
		s3Client, err := remote.NewS3Client(ctx, cfg.Remote.S3.Region, cfg.Remote.S3.Bucket)
		if err != nil {
			return err
		}

		fmt.Printf("Pushing snapshot '%s' to S3...\n", snapshotName)
		if err := s3Client.Push(ctx, cfg.Name, snapshotName, localDir); err != nil {
			return err
		}

		fmt.Println("Snapshot pushed successfully! ☁️")
		return nil
	},
}

var pullCmd = &cobra.Command{
	Use:   "pull [snapshot-name]",
	Short: "Pull a remote snapshot from the S3 bucket to your local machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		snapshotName := args[0]
		ctx := context.Background()

		if cfg.Remote.S3.Bucket == "" {
			return fmt.Errorf("remote S3 bucket is not configured in stasis.yaml")
		}

		localDir, err := utils.GetSnapshotDir(cfg.Name, snapshotName)
		if err != nil {
			return err
		}

		fmt.Printf("Connecting to S3 bucket '%s'...\n", cfg.Remote.S3.Bucket)
		s3Client, err := remote.NewS3Client(ctx, cfg.Remote.S3.Region, cfg.Remote.S3.Bucket)
		if err != nil {
			return err
		}

		fmt.Printf("Pulling snapshot '%s' from S3...\n", snapshotName)
		if err := s3Client.Pull(ctx, cfg.Name, snapshotName, localDir); err != nil {
			return err
		}

		fmt.Println("Snapshot pulled successfully! 📥")
		fmt.Printf("You can now run: stasis snapshot load %s\n", snapshotName)
		return nil
	},
}

func init() {
	// Wire the subcommands to the parent snapshot command
	snapshotCmd.AddCommand(saveCmd)
	snapshotCmd.AddCommand(loadCmd)
	snapshotCmd.AddCommand(listCmd)
	snapshotCmd.AddCommand(pushCmd)
	snapshotCmd.AddCommand(pullCmd)

	// Wire the parent snapshot command to the root command
	rootCmd.AddCommand(snapshotCmd)
}
