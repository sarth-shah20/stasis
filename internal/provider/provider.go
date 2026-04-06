package provider

import (
	"context"

	"github.com/sarth-shah20/stasis/internal/config"
)

// ConnectionInfo holds details about a provisioned service endpoint
type ConnectionInfo struct {
	Host     string // hostname or IP
	Port     int    // primary service port
	Endpoint string // full connection string if applicable (e.g., RDS endpoint)
}

// Provider is the abstraction over local (Docker) and cloud (AWS) environments.
// Both providers implement the same lifecycle: Provision, Deprovision, Status.
type Provider interface {
	// Provision creates or starts the resource for this service.
	// Returns connection info the user can use to reach the service.
	Provision(ctx context.Context, projectName string, serviceName string, service config.Service) (ConnectionInfo, error)

	// Deprovision stops and removes the resource for this service.
	Deprovision(ctx context.Context, projectName string, serviceName string) error

	// Status returns a human-readable status string for the service
	// (e.g., "running", "available", "creating", "not found").
	Status(ctx context.Context, projectName string, serviceName string) (string, error)
}
