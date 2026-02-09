package docker

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

)

// Manager handles all interactions with the Docker Daemon
type Manager struct {
	cli *client.Client
}

// NewManager creates a new Docker client connected to the local daemon
func NewManager() (*Manager, error) {
	// FromEnv looks for standard env vars like DOCKER_HOST,
	// or defaults to the unix socket /var/run/docker.sock
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &Manager{cli: cli}, nil
}

// EnsureImage checks if an image exists locally, and pulls it if it doesn't.
// For now, we will force a pull to keep it simple.
func (m *Manager) EnsureImage(ctx context.Context, imageName string) error {
	fmt.Printf("Pulling image: %s...\n", imageName)

	// ImagePull requests the daemon to download the image.
	// It returns a ReadCloser that streams the download progress (JSON).
	reader, err := m.cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer reader.Close()

	// We must read the output until EOF, otherwise the pull might be cancelled
	// or the connection closed prematurely.
	// io.Copy(os.Stdout, reader) pipes the Docker JSON progress to your terminal.
	// In a polished app, we would parse this JSON to show a progress bar.
	_, err = io.Copy(os.Stdout, reader)
	if err != nil {
		return fmt.Errorf("error reading pull output: %w", err)
	}

	return nil
}