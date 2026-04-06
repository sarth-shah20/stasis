package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sarth-shah20/stasis/internal/config"
	"github.com/sarth-shah20/stasis/internal/docker"
	"github.com/sarth-shah20/stasis/internal/utils"
)

// LocalProvider implements Provider using the local Docker daemon.
// It wraps the existing docker.Manager to satisfy the Provider interface,
// keeping all original Docker logic intact.
type LocalProvider struct {
	mgr *docker.Manager
}

// NewLocalProvider creates a LocalProvider backed by a Docker connection.
func NewLocalProvider() (*LocalProvider, error) {
	mgr, err := docker.NewManager()
	if err != nil {
		return nil, fmt.Errorf("could not connect to docker: %w", err)
	}
	return &LocalProvider{mgr: mgr}, nil
}

// Provision pulls the image, prepares volumes, and starts the container.
// This mirrors the logic previously inline in cmd/up.go.
func (l *LocalProvider) Provision(ctx context.Context, projectName string, serviceName string, service config.Service) (ConnectionInfo, error) {
	fmt.Printf("\n--- Setting up %s ---\n", serviceName)

	// --- Network ---
	networkName := fmt.Sprintf("stasis-%s", projectName)
	if err := l.mgr.EnsureNetwork(ctx, networkName); err != nil {
		return ConnectionInfo{}, err
	}

	// --- Volumes ---
	var volumeBinds []string

	baseDir, _ := utils.GetProjectVolumeDir(projectName)
	projectBase := filepath.Join(baseDir, serviceName)

	for _, vol := range service.Volumes {
		parts := strings.Split(vol, ":")
		if len(parts) == 2 {
			hostPath := filepath.Join(projectBase, parts[0])
			containerPath := parts[1]

			if err := os.MkdirAll(hostPath, 0755); err != nil {
				return ConnectionInfo{}, fmt.Errorf("failed to create volume dir: %w", err)
			}

			bind := fmt.Sprintf("%s:%s", hostPath, containerPath)
			volumeBinds = append(volumeBinds, bind)
		}
	}

	// --- Image ---
	if err := l.mgr.EnsureImage(ctx, service.Image); err != nil {
		return ConnectionInfo{}, fmt.Errorf("failed to pull image: %w", err)
	}

	// --- Port mapping ---
	portMap := ""
	if len(service.Ports) > 0 {
		portMap = service.Ports[0]
	}

	// --- Start container ---
	if err := l.mgr.StartContainer(ctx, projectName, serviceName, service.Image, networkName, portMap, service.Environment, volumeBinds); err != nil {
		return ConnectionInfo{}, fmt.Errorf("failed to start %s: %w", serviceName, err)
	}

	// Build connection info from port mapping
	info := ConnectionInfo{Host: "localhost"}
	if portMap != "" {
		parts := strings.Split(portMap, ":")
		if len(parts) == 2 {
			if p, err := strconv.Atoi(parts[0]); err == nil {
				info.Port = p
			}
		}
	}
	info.Endpoint = fmt.Sprintf("localhost:%d", info.Port)

	return info, nil
}

// Deprovision stops and removes the container for the given service.
func (l *LocalProvider) Deprovision(ctx context.Context, projectName string, serviceName string) error {
	return l.mgr.StopAndRemoveContainer(ctx, projectName, serviceName)
}

// Status returns the Docker container status for the given service.
func (l *LocalProvider) Status(ctx context.Context, projectName string, serviceName string) (string, error) {
	containers, err := l.mgr.ListContainers(ctx, projectName)
	if err != nil {
		return "", err
	}

	target := fmt.Sprintf("stasis-%s-%s", projectName, serviceName)
	for _, c := range containers {
		// Container names from Docker have a leading "/"
		name := c.Names[0]
		if strings.TrimPrefix(name, "/") == target {
			return c.Status, nil
		}
	}

	return "not found", nil
}
