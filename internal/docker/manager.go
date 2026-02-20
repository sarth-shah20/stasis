package docker

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/docker/docker/api/types/network"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"

	"github.com/docker/docker/api/types/filters"
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

// EnsureNetwork creates a bridge network for the project if it doesn't exist.
func (m *Manager) EnsureNetwork(ctx context.Context, networkName string) error {
	// 1. List networks to see if it already exists
	// We filter by name to avoid fetching everything
	// Note: In a production tool, we would use filters.Args, but iterating is easier for learning.
	networks, err := m.cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	for _, net := range networks {
		if net.Name == networkName {
			fmt.Printf("Network %s already exists\n", networkName)
			return nil
		}
	}

	// 2. Create the network if not found
	fmt.Printf("Creating network: %s...\n", networkName)
	_, err = m.cli.NetworkCreate(ctx, networkName, types.NetworkCreate{
		Driver: "bridge", // Standard local network driver
	})
	if err != nil {
		return fmt.Errorf("failed to create network %s: %w", networkName, err)
	}

	return nil
}

// StartContainer creates and starts a container.
// serviceName: e.g., "postgres"
// imageName: e.g., "postgres:14"
// networkName: e.g., "stasis-myproject"
// portMap: e.g., "5432:5432" (host:container)
func (m *Manager) StartContainer(ctx context.Context, projectName, serviceName, imageName, networkName,
								portMapping string, envVars []string, volumes []string) error {

	containerName := fmt.Sprintf("stasis-%s-%s", projectName, serviceName)

	// 1. Configure Port Mapping (Host -> Container)
	// We need to parse "5432:5432" into Docker's format
	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}

	if portMapping != "" {
		// nat.ParsePortSpec parses "8080:80" into structs
		// It returns a list, but we usually just have one mapping per string
		mappings, err := nat.ParsePortSpec(portMapping)
		if err != nil {
			return fmt.Errorf("invalid port mapping %s: %w", portMapping, err)
		}

		for _, pm := range mappings {
			// The container port (e.g., "80/tcp")
			port := pm.Port
			exposedPorts[port] = struct{}{}

			// The host binding
			portBindings[port] = []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: pm.Binding.HostPort,
				},
			}
		}
	}

	// 2. Define the Container Config (Inside)
	config := &container.Config{
		Image:        imageName,

		Labels: map[string]string{
			"stasis.project": projectName,
			"stasis.service": serviceName,
			"stasis.managed": "true",
		},

		ExposedPorts: exposedPorts,
		Env:          envVars,
	}

	// 3. Define the Host Config (Outside)
	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Binds:        volumes,
	}

	// 4. Define Network Config
	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {},
		},
	}

	// 5. Create the Container
	// This just creates the config, it doesn't run it yet.
	fmt.Printf("Creating container %s...\n", containerName)

	// We need to remove it first if it exists (simple cleanup for now)
	// We ignore errors here because it might not exist.
	_ = m.cli.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true})

	resp, err := m.cli.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// 6. Start the Container
	fmt.Printf("Starting container %s...\n", containerName)
	if err := m.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

// ListContainers returns a list of containers belonging to stasis
func (m *Manager) ListContainers(ctx context.Context, projectName string) ([]types.Container, error) {
	// Create a filter: label="stasis.project=<projectName>"
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("stasis.project=%s", projectName))

	return m.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
}

// StopAndRemoveContainer stops and deletes a container
func (m *Manager) StopAndRemoveContainer(ctx context.Context, projectName string,serviceName string) error {
	containerName := fmt.Sprintf("stasis-%s-%s", projectName, serviceName)
	
	fmt.Printf("Stopping %s...\n", containerName)
	
	// timeout := 10 // Seconds to wait for graceful shutdown
    // In newer SDKs, ContainerStop takes a pointer to int or ContainerStopOptions
    // We will use the default (nil = 10 seconds)
	if err := m.cli.ContainerStop(ctx, containerName, container.StopOptions{}); err != nil {
		// If it's already stopped or doesn't exist, just log and continue
		fmt.Printf("Warning: failed to stop %s (might not be running): %v\n", containerName, err)
	}

	fmt.Printf("Removing %s...\n", containerName)
	if err := m.cli.ContainerRemove(ctx, containerName, container.RemoveOptions{
		RemoveVolumes: false, // Keep the data!
		Force:         true,
	}); err != nil {
		return fmt.Errorf("failed to remove %s: %w", containerName, err)
	}

	return nil
}

// RemoveNetwork deletes the project network
func (m *Manager) RemoveNetwork(ctx context.Context, networkName string) error {
	fmt.Printf("Removing network %s...\n", networkName)
	return m.cli.NetworkRemove(ctx, networkName)
}