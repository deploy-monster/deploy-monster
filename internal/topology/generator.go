package topology

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// generateCompose creates the Docker Compose configuration
func (c *Compiler) generateCompose() (*ComposeConfig, error) {
	compose := &ComposeConfig{
		Version:  "3.9",
		Services: make(map[string]Service),
		Networks: make(map[string]Network),
		Volumes:  make(map[string]VolumeSpec),
	}

	// Create default network
	compose.Networks["default"] = Network{Driver: "bridge"}

	// Generate services for databases first (dependencies)
	for i := range c.topology.Databases {
		db := &c.topology.Databases[i]
		if db.Managed || db.External {
			// Skip managed/external databases - they don't run in compose
			continue
		}
		svc, err := c.generateDatabaseService(db)
		if err != nil {
			return nil, fmt.Errorf("database %s: %w", db.Name, err)
		}
		compose.Services[db.Name] = *svc

		// Create volume for database
		volName := fmt.Sprintf("%s_data", db.Name)
		compose.Volumes[volName] = VolumeSpec{Driver: "local"}
	}

	// Generate services for apps
	for i := range c.topology.Apps {
		app := &c.topology.Apps[i]
		svc, err := c.generateAppService(app)
		if err != nil {
			return nil, fmt.Errorf("app %s: %w", app.Name, err)
		}
		compose.Services[app.Name] = *svc
	}

	// Generate services for workers
	for i := range c.topology.Workers {
		worker := &c.topology.Workers[i]
		svc, err := c.generateWorkerService(worker)
		if err != nil {
			return nil, fmt.Errorf("worker %s: %w", worker.Name, err)
		}
		compose.Services[worker.Name] = *svc
	}

	// Generate volumes
	for _, vol := range c.topology.Volumes {
		if vol.Temporary {
			// tmpfs volumes don't need to be declared
			continue
		}
		compose.Volumes[vol.Name] = VolumeSpec{Driver: "local"}
	}

	// Generate reverse proxy (Caddy) if there are domains
	if len(c.topology.Domains) > 0 {
		svc, err := c.generateProxyService()
		if err != nil {
			return nil, fmt.Errorf("proxy: %w", err)
		}
		compose.Services["proxy"] = *svc

		// Add Caddy volumes for SSL certificates and config
		compose.Volumes["caddy_data"] = VolumeSpec{Driver: "local"}
		compose.Volumes["caddy_config"] = VolumeSpec{Driver: "local"}
	}

	return compose, nil
}

// generateDatabaseService creates a service definition for a database
func (c *Compiler) generateDatabaseService(db *Database) (*Service, error) {
	svc := &Service{
		ContainerName: fmt.Sprintf("%s-%s", c.project, db.Name),
		Restart:       "unless-stopped",
		Networks:      []string{"default"},
		Environment:   make(map[string]string),
		Labels:        make(map[string]string),
	}

	// Set image based on engine
	image, err := c.getDatabaseImage(db)
	if err != nil {
		return nil, err
	}
	svc.Image = image

	// Set credentials
	username := db.Username
	if username == "" {
		username = "root"
	}
	password := db.Password
	if password == "" {
		password = c.generatePassword()
	}
	database := db.Database
	if database == "" {
		database = db.Name
	}

	// Engine-specific configuration
	switch db.Engine {
	case EnginePostgres:
		svc.Environment["POSTGRES_USER"] = username
		svc.Environment["POSTGRES_PASSWORD"] = password
		svc.Environment["POSTGRES_DB"] = database
		svc.HealthCheck = &HealthCheck{
			Test:     []string{"CMD-SHELL", "pg_isready -U " + username},
			Interval: "5s",
			Timeout:  "5s",
			Retries:  5,
		}
	case EngineMySQL:
		svc.Environment["MYSQL_ROOT_PASSWORD"] = password
		svc.Environment["MYSQL_DATABASE"] = database
		if username != "root" {
			svc.Environment["MYSQL_USER"] = username
			svc.Environment["MYSQL_PASSWORD"] = password
		}
		svc.HealthCheck = &HealthCheck{
			Test:     []string{"CMD", "mysqladmin", "ping", "-h", "localhost"},
			Interval: "5s",
			Timeout:  "5s",
			Retries:  5,
		}
	case EngineMariaDB:
		svc.Environment["MARIADB_ROOT_PASSWORD"] = password
		svc.Environment["MARIADB_DATABASE"] = database
		if username != "root" {
			svc.Environment["MARIADB_USER"] = username
			svc.Environment["MARIADB_PASSWORD"] = password
		}
		svc.HealthCheck = &HealthCheck{
			Test:     []string{"CMD", "healthcheck.sh", "--connect"},
			Interval: "5s",
			Timeout:  "5s",
			Retries:  5,
		}
	case EngineMongoDB:
		svc.Environment["MONGO_INITDB_ROOT_USERNAME"] = username
		svc.Environment["MONGO_INITDB_ROOT_PASSWORD"] = password
		svc.HealthCheck = &HealthCheck{
			Test:     []string{"CMD", "mongosh", "--eval", "db.adminCommand('ping')"},
			Interval: "5s",
			Timeout:  "5s",
			Retries:  5,
		}
	case EngineRedis:
		if password != "" {
			svc.Command = fmt.Sprintf("redis-server --requirepass %s", password)
		}
		svc.HealthCheck = &HealthCheck{
			Test:     []string{"CMD", "redis-cli", "ping"},
			Interval: "5s",
			Timeout:  "5s",
			Retries:  5,
		}
	}

	// Data volume
	volName := fmt.Sprintf("%s_data", db.Name)
	dataPath := c.getDatabaseDataPath(db.Engine)
	svc.Volumes = []string{fmt.Sprintf("%s:%s", volName, dataPath)}

	// Labels
	svc.Labels["monster.type"] = "database"
	svc.Labels["monster.engine"] = string(db.Engine)
	svc.Labels["monster.project"] = c.project
	svc.Labels["monster.environment"] = c.env

	// Extra config
	for k, v := range db.ExtraConfig {
		svc.Environment[k] = v
	}

	return svc, nil
}

// generateAppService creates a service definition for an app
func (c *Compiler) generateAppService(app *App) (*Service, error) {
	svc := &Service{
		ContainerName: fmt.Sprintf("%s-%s", c.project, app.Name),
		Restart:       "unless-stopped",
		Networks:      []string{"default"},
		Environment:   make(map[string]string),
		Labels:        make(map[string]string),
	}

	// Image (built from git)
	imageName := c.images[app.ID]
	svc.Image = imageName

	// Build configuration
	svc.Build = &BuildConfig{
		Context:    filepath.Join(".", "apps", app.Name),
		Dockerfile: app.Dockerfile,
	}

	// Port
	if app.Port > 0 {
		svc.Expose = []int{app.Port}
		// Don't publish to host - use proxy for external access
	}

	// Environment variables
	for k, v := range app.EnvVars {
		// Check for references
		if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
			// Resolve reference
			refKey := strings.Trim(v, "${}")
			if val, ok := c.connections[refKey]; ok {
				svc.Environment[k] = val
			} else {
				svc.Environment[k] = v
			}
		} else {
			svc.Environment[k] = v
		}
	}

	// Add connection environment variables from dependencies
	for _, conn := range c.topology.Connections {
		if conn.Type == ConnDependency && conn.SourceID == app.ID {
			// This app depends on something
			if conn.Config.EnvVarName != "" {
				if val, ok := c.connections[conn.Config.EnvVarName]; ok {
					svc.Environment[conn.Config.EnvVarName] = val
				}
			}
		}
	}

	// Volume mounts
	for _, mount := range app.VolumeMounts {
		vol := c.findVolume(mount.VolumeID)
		if vol == nil {
			continue
		}

		var volRef string
		if vol.Temporary {
			// tmpfs mount
			volRef = fmt.Sprintf("tmpfs:%s", mount.MountPath)
		} else {
			volRef = fmt.Sprintf("%s:%s", vol.Name, mount.MountPath)
		}
		if mount.ReadOnly {
			volRef += ":ro"
		}
		svc.Volumes = append(svc.Volumes, volRef)
	}

	// Health check
	if app.HealthCheckPath != "" {
		port := app.HealthCheckPort
		if port == 0 {
			port = app.Port
		}
		svc.HealthCheck = &HealthCheck{
			Test:     []string{"CMD", "curl", "-f", fmt.Sprintf("http://localhost:%d%s", port, app.HealthCheckPath)},
			Interval: "30s",
			Timeout:  "10s",
			Retries:  3,
		}
	}

	// Depends on databases
	for _, conn := range c.topology.Connections {
		if conn.Type == ConnDependency && conn.SourceID == app.ID {
			// Find target database name
			for _, db := range c.topology.Databases {
				if db.ID == conn.TargetID && !db.Managed && !db.External {
					svc.DependsOn = append(svc.DependsOn, db.Name)
				}
			}
		}
	}
	sort.Strings(svc.DependsOn)

	// Labels
	svc.Labels["monster.type"] = "app"
	svc.Labels["monster.project"] = c.project
	svc.Labels["monster.environment"] = c.env
	svc.Labels["monster.port"] = fmt.Sprintf("%d", app.Port)

	// Resource limits
	if app.MemoryMB > 0 || app.CPU > 0 {
		svc.Deploy = &DeployConfig{
			Resources: &Resources{
				Limits: &ResourceLimit{},
			},
		}
		if app.MemoryMB > 0 {
			svc.Deploy.Resources.Limits.Memory = fmt.Sprintf("%dM", app.MemoryMB)
		}
		if app.CPU > 0 {
			svc.Deploy.Resources.Limits.CPUs = fmt.Sprintf("%dm", app.CPU)
		}
	}

	return svc, nil
}

// generateWorkerService creates a service definition for a worker
func (c *Compiler) generateWorkerService(worker *Worker) (*Service, error) {
	svc := &Service{
		ContainerName: fmt.Sprintf("%s-%s", c.project, worker.Name),
		Restart:       "unless-stopped",
		Networks:      []string{"default"},
		Environment:   make(map[string]string),
		Labels:        make(map[string]string),
	}

	// Image (built from git)
	imageName := c.images[worker.ID]
	svc.Image = imageName

	// Build configuration
	svc.Build = &BuildConfig{
		Context:    filepath.Join(".", "workers", worker.Name),
		Dockerfile: worker.Dockerfile,
	}

	// Command
	if worker.Command != "" {
		svc.Command = worker.Command
	}

	// Environment variables
	for k, v := range worker.EnvVars {
		if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
			refKey := strings.Trim(v, "${}")
			if val, ok := c.connections[refKey]; ok {
				svc.Environment[k] = val
			} else {
				svc.Environment[k] = v
			}
		} else {
			svc.Environment[k] = v
		}
	}

	// Volume mounts
	for _, mount := range worker.VolumeMounts {
		vol := c.findVolume(mount.VolumeID)
		if vol == nil {
			continue
		}

		var volRef string
		if vol.Temporary {
			volRef = fmt.Sprintf("tmpfs:%s", mount.MountPath)
		} else {
			volRef = fmt.Sprintf("%s:%s", vol.Name, mount.MountPath)
		}
		if mount.ReadOnly {
			volRef += ":ro"
		}
		svc.Volumes = append(svc.Volumes, volRef)
	}

	// Depends on databases
	for _, conn := range c.topology.Connections {
		if conn.Type == ConnDependency && conn.SourceID == worker.ID {
			for _, db := range c.topology.Databases {
				if db.ID == conn.TargetID && !db.Managed && !db.External {
					svc.DependsOn = append(svc.DependsOn, db.Name)
				}
			}
		}
	}

	// Labels
	svc.Labels["monster.type"] = "worker"
	svc.Labels["monster.project"] = c.project
	svc.Labels["monster.environment"] = c.env

	return svc, nil
}

// generateProxyService creates the reverse proxy (Caddy) service
func (c *Compiler) generateProxyService() (*Service, error) {
	svc := &Service{
		ContainerName: fmt.Sprintf("%s-proxy", c.project),
		Image:         "caddy:2-alpine",
		Restart:       "unless-stopped",
		Networks:      []string{"default"},
		Ports: []PortMapping{
			{Host: 80, Container: 80},
			{Host: 443, Container: 443},
		},
		Volumes: []string{
			"caddy_data:/data",
			"caddy_config:/config",
			"./Caddyfile:/etc/caddy/Caddyfile:ro",
		},
		Labels: map[string]string{
			"monster.type":    "proxy",
			"monster.project": c.project,
		},
	}

	return svc, nil
}

// GenerateCaddyfile generates the Caddy configuration for domains
func (c *Compiler) GenerateCaddyfile() string {
	var sb strings.Builder

	// Global options
	sb.WriteString("{\n")
	sb.WriteString("  email admin@deploy.monster\n")
	sb.WriteString("  acme_ca https://acme-v02.api.letsencrypt.org/directory\n")
	sb.WriteString("}\n\n")

	// Domain blocks
	for _, domain := range c.topology.Domains {
		if domain.TargetAppID == "" {
			continue
		}

		// Find target app
		var app *App
		for i := range c.topology.Apps {
			if c.topology.Apps[i].ID == domain.TargetAppID {
				app = &c.topology.Apps[i]
				break
			}
		}
		if app == nil {
			continue
		}

		// Domain block
		sb.WriteString(fmt.Sprintf("%s {\n", domain.FQDN))

		// Reverse proxy
		port := app.Port
		if port == 0 {
			port = 3000
		}
		sb.WriteString(fmt.Sprintf("  reverse_proxy %s:%d\n", app.Name, port))

		// Encoding
		sb.WriteString("  encode gzip zstd\n")

		// TLS
		if domain.SSLEnabled {
			if domain.SSLMODE == SSLAuto {
				sb.WriteString("  tls internal\n")
			}
		}

		sb.WriteString("}\n\n")
	}

	return sb.String()
}

// GenerateEnvFile generates the .env file content
func (c *Compiler) GenerateEnvFile() string {
	var sb strings.Builder

	sb.WriteString("# Auto-generated environment file\n")
	sb.WriteString(fmt.Sprintf("# Project: %s\n", c.project))
	sb.WriteString(fmt.Sprintf("# Environment: %s\n\n", c.env))

	// Add all resolved connections
	for key, val := range c.connections {
		sb.WriteString(fmt.Sprintf("%s=%s\n", key, val))
	}

	return sb.String()
}

// Helper functions

func (c *Compiler) getDatabaseImage(db *Database) (string, error) {
	version := db.Version
	if version == "" {
		version = c.getDefaultVersion(db.Engine)
	}

	switch db.Engine {
	case EnginePostgres:
		return fmt.Sprintf("postgres:%s-alpine", version), nil
	case EngineMySQL:
		return fmt.Sprintf("mysql:%s", version), nil
	case EngineMariaDB:
		return fmt.Sprintf("mariadb:%s", version), nil
	case EngineMongoDB:
		return fmt.Sprintf("mongo:%s", version), nil
	case EngineRedis:
		return fmt.Sprintf("redis:%s-alpine", version), nil
	default:
		return "", fmt.Errorf("unsupported database engine: %s", db.Engine)
	}
}

func (c *Compiler) getDefaultVersion(engine DatabaseEngine) string {
	switch engine {
	case EnginePostgres:
		return "16"
	case EngineMySQL:
		return "8.0"
	case EngineMariaDB:
		return "11"
	case EngineMongoDB:
		return "7"
	case EngineRedis:
		return "7"
	default:
		return "latest"
	}
}

func (c *Compiler) getDatabaseDataPath(engine DatabaseEngine) string {
	switch engine {
	case EnginePostgres:
		return "/var/lib/postgresql/data"
	case EngineMySQL:
		return "/var/lib/mysql"
	case EngineMariaDB:
		return "/var/lib/mysql"
	case EngineMongoDB:
		return "/data/db"
	case EngineRedis:
		return "/data"
	default:
		return "/data"
	}
}

func (c *Compiler) findVolume(id string) *Volume {
	for i := range c.topology.Volumes {
		if c.topology.Volumes[i].ID == id {
			return &c.topology.Volumes[i]
		}
	}
	return nil
}
