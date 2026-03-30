package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Snapshotter defines the contract for saving and restoring container state.
type Snapshotter interface {
	// Save extracts data from the container and writes it to destDir
	Save(ctx context.Context, containerName, destDir string) error
	
	// Load reads data from srcDir and injects it into the container
	Load(ctx context.Context, containerName, srcDir string) error
	
	// RequiresStop indicates if the container must be stopped before taking the snapshot
	// (e.g., true for generic volume tarballing, false for pg_dump)
	RequiresStop() bool
}

// Manifest holds metadata about an entire environment snapshot.
type Manifest struct {
	ProjectName  string                 `json:"project_name"`
	SnapshotName string                 `json:"snapshot_name"`
	Timestamp    time.Time              `json:"timestamp"`
	Services     map[string]ServiceMeta `json:"services"`
}

// ServiceMeta holds metadata for a specific service within the snapshot.
type ServiceMeta struct {
	Strategy string `json:"strategy"` // e.g., "postgres", "redis", "tar"
	Image    string `json:"image"`    // e.g., "postgres:14"
}

// SaveManifest writes the Manifest struct to a manifest.json file in the target directory i.e Marshalling
func SaveManifest(destDir string, manifest Manifest) error {
	manifestPath := filepath.Join(destDir, "manifest.json")

	// MarshalIndent makes the JSON human-readable with indention
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// 0644 sp that owner can read/write and others can only read
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest.json: %w", err)
	}

	return nil
}

// LoadManifest reads a manifest.json file from the source directory and parses it i.e unmarshalling
func LoadManifest(srcDir string) (*Manifest, error) {
	manifestPath := filepath.Join(srcDir, "manifest.json")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest.json: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest.json: %w", err)
	}

	return &manifest, nil
}