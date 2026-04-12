package topology

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

// Compiler converts a Topology into Docker Compose configuration
type Compiler struct {
	topology *Topology
	project  string
	env      string

	// Resolved values
	images      map[string]string // appID -> image name
	connections map[string]string // env var name -> value
}

// NewCompiler creates a new topology compiler
func NewCompiler(t *Topology, project, env string) *Compiler {
	return &Compiler{
		topology:    t,
		project:     project,
		env:         env,
		images:      make(map[string]string),
		connections: make(map[string]string),
	}
}

// Compile generates a complete Docker Compose configuration
func (c *Compiler) Compile() (*ComposeConfig, error) {
	// Phase 1: Validate
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Phase 2: Resolve references
	if err := c.resolveReferences(); err != nil {
		return nil, fmt.Errorf("reference resolution failed: %w", err)
	}

	// Phase 3: Generate compose
	compose, err := c.generateCompose()
	if err != nil {
		return nil, fmt.Errorf("compose generation failed: %w", err)
	}

	return compose, nil
}

// validate performs topology validation
func (c *Compiler) validate() error {
	// Check for duplicate names
	names := make(map[string]bool)
	for _, app := range c.topology.Apps {
		if names[app.Name] {
			return fmt.Errorf("duplicate app name: %s", app.Name)
		}
		names[app.Name] = true
	}
	for _, db := range c.topology.Databases {
		if names[db.Name] {
			return fmt.Errorf("duplicate database name: %s", db.Name)
		}
		names[db.Name] = true
	}
	for _, vol := range c.topology.Volumes {
		if names[vol.Name] {
			return fmt.Errorf("duplicate volume name: %s", vol.Name)
		}
		names[vol.Name] = true
	}

	// Check port conflicts
	ports := make(map[int]string)
	for _, app := range c.topology.Apps {
		if app.Port > 0 {
			if existing, ok := ports[app.Port]; ok {
				return fmt.Errorf("port conflict: %d used by both %s and %s", app.Port, existing, app.Name)
			}
			ports[app.Port] = app.Name
		}
	}

	// Check domain targets
	for _, domain := range c.topology.Domains {
		if domain.TargetAppID != "" {
			if !c.appExists(domain.TargetAppID) {
				return fmt.Errorf("domain %s targets non-existent app: %s", domain.FQDN, domain.TargetAppID)
			}
		}
	}

	// Check volume mounts
	for _, app := range c.topology.Apps {
		for _, mount := range app.VolumeMounts {
			if !c.volumeExists(mount.VolumeID) {
				return fmt.Errorf("app %s mounts non-existent volume: %s", app.Name, mount.VolumeID)
			}
		}
	}

	// Check connections
	for _, conn := range c.topology.Connections {
		if !c.componentExists(conn.SourceID) {
			return fmt.Errorf("connection %s has invalid source: %s", conn.ID, conn.SourceID)
		}
		if !c.componentExists(conn.TargetID) {
			return fmt.Errorf("connection %s has invalid target: %s", conn.ID, conn.TargetID)
		}
	}

	return nil
}

// resolveReferences resolves all references (secrets, images, connections)
func (c *Compiler) resolveReferences() error {
	// Resolve database connections
	for _, conn := range c.topology.Connections {
		if conn.Type == ConnDependency {
			// Find the database
			var db *Database
			for i := range c.topology.Databases {
				if c.topology.Databases[i].ID == conn.TargetID {
					db = &c.topology.Databases[i]
					break
				}
			}
			if db == nil {
				continue
			}

			// Generate connection string
			connStr := c.generateDatabaseURL(db)
			envVar := conn.Config.EnvVarName
			if envVar == "" {
				envVar = fmt.Sprintf("%s_URL", strings.ToUpper(db.Name))
			}
			c.connections[envVar] = connStr
		}
	}

	// Resolve image names for apps
	for _, app := range c.topology.Apps {
		imageName := fmt.Sprintf("%s/%s-%s:latest", c.project, app.Name, c.env)
		c.images[app.ID] = imageName
	}

	// Resolve image names for workers
	for _, worker := range c.topology.Workers {
		imageName := fmt.Sprintf("%s/%s-%s:latest", c.project, worker.Name, c.env)
		c.images[worker.ID] = imageName
	}

	return nil
}

// generateDatabaseURL creates a connection URL for a database
func (c *Compiler) generateDatabaseURL(db *Database) string {
	if db.ConnURL != "" {
		return db.ConnURL
	}

	host := db.Name
	port := c.getDatabasePort(db.Engine)
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

	switch db.Engine {
	case EnginePostgres:
		return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s", username, password, host, port, database)
	case EngineMySQL:
		return fmt.Sprintf("mysql://%s:%s@%s:%d/%s", username, password, host, port, database)
	case EngineMariaDB:
		return fmt.Sprintf("mariadb://%s:%s@%s:%d/%s", username, password, host, port, database)
	case EngineMongoDB:
		return fmt.Sprintf("mongodb://%s:%s@%s:%d/%s", username, password, host, port, database)
	case EngineRedis:
		if password != "" {
			return fmt.Sprintf("redis://:%s@%s:%d/%s", password, host, port, database)
		}
		return fmt.Sprintf("redis://%s:%d/%s", host, port, database)
	default:
		return fmt.Sprintf("%s://%s:%d/%s", db.Engine, host, port, database)
	}
}

// getDatabasePort returns the default port for a database engine
func (c *Compiler) getDatabasePort(engine DatabaseEngine) int {
	switch engine {
	case EnginePostgres:
		return 5432
	case EngineMySQL:
		return 3306
	case EngineMariaDB:
		return 3306
	case EngineMongoDB:
		return 27017
	case EngineRedis:
		return 6379
	default:
		return 5432
	}
}

// generatePassword generates a random password
func (c *Compiler) generatePassword() string {
	return fmt.Sprintf("pwd_%s", randomString(12))
}

// ComposeConfig represents the generated Docker Compose configuration
type ComposeConfig struct {
	Version  string                `yaml:"version,omitempty"`
	Services map[string]Service    `yaml:"services"`
	Networks map[string]Network    `yaml:"networks,omitempty"`
	Volumes  map[string]VolumeSpec `yaml:"volumes,omitempty"`
	Configs  map[string]Config     `yaml:"configs,omitempty"`
	Secrets  map[string]Secret     `yaml:"secrets,omitempty"`
}

// Service represents a Docker Compose service
type Service struct {
	Build         *BuildConfig      `yaml:"build,omitempty"`
	Image         string            `yaml:"image,omitempty"`
	ContainerName string            `yaml:"container_name,omitempty"`
	Restart       string            `yaml:"restart,omitempty"`
	Ports         []PortMapping     `yaml:"ports,omitempty"`
	Expose        []int             `yaml:"expose,omitempty"`
	Environment   map[string]string `yaml:"environment,omitempty"`
	EnvFile       []string          `yaml:"env_file,omitempty"`
	Volumes       []string          `yaml:"volumes,omitempty"`
	Networks      []string          `yaml:"networks,omitempty"`
	DependsOn     []string          `yaml:"depends_on,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
	HealthCheck   *HealthCheck      `yaml:"health_check,omitempty"`
	Command       string            `yaml:"command,omitempty"`
	Entrypoint    string            `yaml:"entrypoint,omitempty"`
	WorkingDir    string            `yaml:"working_dir,omitempty"`
	User          string            `yaml:"user,omitempty"`
	Deploy        *DeployConfig     `yaml:"deploy,omitempty"`
	Replicas      *int              `yaml:"replicas,omitempty"` // For swarm mode
}

// BuildConfig represents Docker build configuration
type BuildConfig struct {
	Context    string            `yaml:"context,omitempty"`
	Dockerfile string            `yaml:"dockerfile,omitempty"`
	Args       map[string]string `yaml:"args,omitempty"`
	Target     string            `yaml:"target,omitempty"`
	CacheFrom  []string          `yaml:"cache_from,omitempty"`
}

// PortMapping represents a port mapping
type PortMapping struct {
	Host      int
	Container int
	Protocol  string
}

// Network represents a Docker network
type Network struct {
	Driver string `yaml:"driver,omitempty"`
}

// VolumeSpec represents a Docker volume
type VolumeSpec struct {
	Driver     string            `yaml:"driver,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
	External   bool              `yaml:"external,omitempty"`
}

// Config represents a Docker config
type Config struct {
	File     string `yaml:"file,omitempty"`
	External bool   `yaml:"external,omitempty"`
}

// Secret represents a Docker secret
type Secret struct {
	File     string `yaml:"file,omitempty"`
	External bool   `yaml:"external,omitempty"`
}

// HealthCheck represents a health check configuration
type HealthCheck struct {
	Test     []string `yaml:"test,omitempty"`
	Interval string   `yaml:"interval,omitempty"`
	Timeout  string   `yaml:"timeout,omitempty"`
	Retries  int      `yaml:"retries,omitempty"`
	Disable  bool     `yaml:"disable,omitempty"`
}

// DeployConfig represents deployment configuration for swarm
type DeployConfig struct {
	Replicas      int            `yaml:"replicas,omitempty"`
	Resources     *Resources     `yaml:"resources,omitempty"`
	RestartPolicy *RestartPolicy `yaml:"restart_policy,omitempty"`
	Placement     *Placement     `yaml:"placement,omitempty"`
}

// Resources represents resource limits
type Resources struct {
	Limits       *ResourceLimit `yaml:"limits,omitempty"`
	Reservations *ResourceLimit `yaml:"reservations,omitempty"`
}

// ResourceLimit represents resource constraints
type ResourceLimit struct {
	CPUs   string `yaml:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}

// RestartPolicy represents restart configuration
type RestartPolicy struct {
	Condition   string `yaml:"condition,omitempty"`
	Delay       string `yaml:"delay,omitempty"`
	MaxAttempts int    `yaml:"max_attempts,omitempty"`
	Window      string `yaml:"window,omitempty"`
}

// Placement represents placement constraints
type Placement struct {
	Constraints []string `yaml:"constraints,omitempty"`
	Preferences []string `yaml:"preferences,omitempty"`
}

// Helper functions

func (c *Compiler) appExists(id string) bool {
	for _, app := range c.topology.Apps {
		if app.ID == id {
			return true
		}
	}
	return false
}

func (c *Compiler) volumeExists(id string) bool {
	for _, vol := range c.topology.Volumes {
		if vol.ID == id {
			return true
		}
	}
	return false
}

func (c *Compiler) componentExists(id string) bool {
	if c.appExists(id) {
		return true
	}
	for _, db := range c.topology.Databases {
		if db.ID == id {
			return true
		}
	}
	for _, vol := range c.topology.Volumes {
		if vol.ID == id {
			return true
		}
	}
	for _, domain := range c.topology.Domains {
		if domain.ID == id {
			return true
		}
	}
	for _, worker := range c.topology.Workers {
		if worker.ID == id {
			return true
		}
	}
	return false
}

// randomString generates a cryptographically secure random string of given length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	max := big.NewInt(int64(len(charset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// Fallback to a deterministic but consistent char on entropy failure
			b[i] = charset[i%len(charset)]
			continue
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// Sort helpers for deterministic output

type byName []App

func (a byName) Len() int           { return len(a) }
func (a byName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byName) Less(i, j int) bool { return a[i].Name < a[j].Name }

// Ensure sort package is imported
var _ = sort.Sort
