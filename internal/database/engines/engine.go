package engines

import (
	"context"
	"fmt"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Engine is the interface for provisioning a specific database type.
type Engine interface {
	// Name returns the engine identifier (postgres, mysql, redis, etc.)
	Name() string
	// Versions returns supported versions.
	Versions() []string
	// DefaultPort returns the default port for this engine.
	DefaultPort() int
	// Image returns the Docker image for a given version.
	Image(version string) string
	// Env returns required environment variables for initial setup.
	Env(credentials Credentials) []string
	// HealthCmd returns the health check command for this engine.
	HealthCmd() []string
	// ConnectionString builds a connection string for clients.
	ConnectionString(host string, port int, creds Credentials) string
}

// Credentials holds database access credentials.
type Credentials struct {
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// ProvisionOpts holds options for creating a managed database.
type ProvisionOpts struct {
	TenantID string
	Name     string
	Engine   string
	Version  string
	ServerID string
}

// Provision creates a new managed database container.
func Provision(ctx context.Context, runtime core.ContainerRuntime, engine Engine, opts ProvisionOpts) (string, Credentials, error) {
	if runtime == nil {
		return "", Credentials{}, fmt.Errorf("container runtime not available")
	}

	creds := Credentials{
		Database: opts.Name,
		User:     opts.Name,
		Password: core.GeneratePassword(24),
	}

	version := opts.Version
	if version == "" && len(engine.Versions()) > 0 {
		version = engine.Versions()[0] // Latest
	}

	containerName := fmt.Sprintf("monster-db-%s-%s", opts.Engine, core.GenerateID()[:8])
	labels := map[string]string{
		"monster.enable":      "true",
		"monster.managed":     "database",
		"monster.db.engine":   opts.Engine,
		"monster.db.name":     opts.Name,
		"monster.tenant":      opts.TenantID,
	}

	containerID, err := runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         engine.Image(version),
		Env:           engine.Env(creds),
		Labels:        labels,
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		return "", Credentials{}, fmt.Errorf("provision %s: %w", opts.Engine, err)
	}

	return containerID, creds, nil
}

// Registry holds all available database engines.
var Registry = map[string]Engine{
	"postgres": &Postgres{},
	"mysql":    &MySQL{},
	"mariadb":  &MariaDB{},
	"redis":    &Redis{},
	"mongodb":  &MongoDB{},
}

// Get returns an engine by name.
func Get(name string) (Engine, bool) {
	e, ok := Registry[name]
	return e, ok
}
