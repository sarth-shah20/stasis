package utils

import (
	"os"
	"path/filepath"
)

// returns: ~/.stasis/volumes/<projectName>
func GetProjectVolumeDir(projectName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".stasis", "volumes", projectName), nil
}

// returns: ~/.stasis/snapshots/<projectName>/<snapshotName>
func GetSnapshotDir(projectName, snapshotName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".stasis", "snapshots", projectName, snapshotName), nil
}
