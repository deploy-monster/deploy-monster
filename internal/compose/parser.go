package compose

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"gopkg.in/yaml.v3"
)

// ComposeFile represents a parsed docker-compose.yml.
type ComposeFile struct {
	Version  string                    `yaml:"version,omitempty" json:"version,omitempty"`
	Services map[string]*ServiceConfig `yaml:"services" json:"services"`
	Networks map[string]*NetworkConfig `yaml:"networks,omitempty" json:"networks,omitempty"`
	Volumes  map[string]*VolumeConfig  `yaml:"volumes,omitempty" json:"volumes,omitempty"`
}

// ServiceConfig represents a single service in the compose file.
type ServiceConfig struct {
	Image           string            `yaml:"image,omitempty" json:"image,omitempty"`
	Build           *BuildConfig      `yaml:"build,omitempty" json:"build,omitempty"`
	Command         any               `yaml:"command,omitempty" json:"command,omitempty"`
	Entrypoint      any               `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Environment     any               `yaml:"environment,omitempty" json:"environment,omitempty"` // map or list
	EnvFile         any               `yaml:"env_file,omitempty" json:"env_file,omitempty"`
	Ports           []string          `yaml:"ports,omitempty" json:"ports,omitempty"`
	Volumes         []string          `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	DependsOn       any               `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Networks        any               `yaml:"networks,omitempty" json:"networks,omitempty"`
	Labels          map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Restart         string            `yaml:"restart,omitempty" json:"restart,omitempty"`
	HealthCheck     *HealthCheck      `yaml:"healthcheck,omitempty" json:"healthcheck,omitempty"`
	Deploy          *DeployConfig     `yaml:"deploy,omitempty" json:"deploy,omitempty"`
	CapAdd          []string          `yaml:"cap_add,omitempty" json:"cap_add,omitempty"`
	CapDrop         []string          `yaml:"cap_drop,omitempty" json:"cap_drop,omitempty"`
	Privileged      bool              `yaml:"privileged,omitempty" json:"privileged,omitempty"`
	User            string            `yaml:"user,omitempty" json:"user,omitempty"`
	WorkingDir      string            `yaml:"working_dir,omitempty" json:"working_dir,omitempty"`
	Hostname        string            `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	ExtraHosts      []string          `yaml:"extra_hosts,omitempty" json:"extra_hosts,omitempty"`
	Logging         *LoggingConfig    `yaml:"logging,omitempty" json:"logging,omitempty"`
	StopGracePeriod string            `yaml:"stop_grace_period,omitempty" json:"stop_grace_period,omitempty"`

	// Resolved fields (populated after parsing)
	ResolvedEnv map[string]string `yaml:"-" json:"resolved_env,omitempty"`
}

// BuildConfig for services with build: directive.
type BuildConfig struct {
	Context    string            `yaml:"context,omitempty" json:"context,omitempty"`
	Dockerfile string            `yaml:"dockerfile,omitempty" json:"dockerfile,omitempty"`
	Args       map[string]string `yaml:"args,omitempty" json:"args,omitempty"`
	Target     string            `yaml:"target,omitempty" json:"target,omitempty"`
}

// NetworkConfig for custom networks.
type NetworkConfig struct {
	Driver   string            `yaml:"driver,omitempty" json:"driver,omitempty"`
	External bool              `yaml:"external,omitempty" json:"external,omitempty"`
	Labels   map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// VolumeConfig for named volumes.
type VolumeConfig struct {
	Driver   string            `yaml:"driver,omitempty" json:"driver,omitempty"`
	External bool              `yaml:"external,omitempty" json:"external,omitempty"`
	Labels   map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// HealthCheck configuration.
type HealthCheck struct {
	Test        any    `yaml:"test,omitempty" json:"test,omitempty"`
	Interval    string `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout     string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Retries     int    `yaml:"retries,omitempty" json:"retries,omitempty"`
	StartPeriod string `yaml:"start_period,omitempty" json:"start_period,omitempty"`
}

// DeployConfig for swarm deploy settings.
type DeployConfig struct {
	Replicas  int               `yaml:"replicas,omitempty" json:"replicas,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Resources *ResourceConfig   `yaml:"resources,omitempty" json:"resources,omitempty"`
}

// ResourceConfig for deploy resource limits.
type ResourceConfig struct {
	Limits       *ResourceSpec `yaml:"limits,omitempty" json:"limits,omitempty"`
	Reservations *ResourceSpec `yaml:"reservations,omitempty" json:"reservations,omitempty"`
}

// ResourceSpec defines CPU/memory limits.
type ResourceSpec struct {
	CPUs   string `yaml:"cpus,omitempty" json:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty" json:"memory,omitempty"`
}

// LoggingConfig for service logging.
type LoggingConfig struct {
	Driver  string            `yaml:"driver,omitempty" json:"driver,omitempty"`
	Options map[string]string `yaml:"options,omitempty" json:"options,omitempty"`
}

// Parse reads and parses a docker-compose YAML string.
func Parse(data []byte) (*ComposeFile, error) {
	var cf ComposeFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse compose yaml: %w", err)
	}

	if len(cf.Services) == 0 {
		return nil, fmt.Errorf("no services defined")
	}

	// Resolve environment variables for each service
	for name, svc := range cf.Services {
		if svc == nil {
			cf.Services[name] = &ServiceConfig{}
			svc = cf.Services[name]
		}
		svc.ResolvedEnv = resolveEnv(svc.Environment)
		_ = name
	}

	return &cf, nil
}

// resolveEnv normalizes environment from either map or list format.
func resolveEnv(env any) map[string]string {
	result := make(map[string]string)
	if env == nil {
		return result
	}

	switch v := env.(type) {
	case map[string]any:
		for key, val := range v {
			result[key] = fmt.Sprintf("%v", val)
		}
	case []any:
		for _, item := range v {
			s := fmt.Sprintf("%v", item)
			parts := strings.SplitN(s, "=", 2)
			if len(parts) == 2 {
				result[parts[0]] = parts[1]
			} else {
				result[parts[0]] = ""
			}
		}
	}

	return result
}

// Interpolate replaces ${VAR} and ${VAR:-default} in the compose YAML.
// When a sensitive variable is missing and has no default, or has a known
// weak placeholder default, a crypto-random password is generated.
// The same generated value is reused for every occurrence of the same
// variable name so that e.g. DB_PASSWORD stays consistent across services.
func Interpolate(data []byte, vars map[string]string) []byte {
	generated := make(map[string]string) // track auto-generated secrets per key
	re := regexp.MustCompile(`\$\{([^}]+)\}`)
	result := re.ReplaceAllFunc(data, func(match []byte) []byte {
		expr := string(match[2 : len(match)-1]) // strip ${ and }

		// Handle ${VAR:-default}
		if idx := strings.Index(expr, ":-"); idx >= 0 {
			key := expr[:idx]
			defaultVal := expr[idx+2:]
			if val, ok := vars[key]; ok && val != "" {
				return []byte(val)
			}
			if isSensitiveEnvKey(key) && isWeakSecretDefault(defaultVal) {
				return []byte(generatedSecret(generated, key))
			}
			return []byte(defaultVal)
		}

		// Handle ${VAR}
		if val, ok := vars[expr]; ok {
			return []byte(val)
		}
		if isSensitiveEnvKey(expr) {
			return []byte(generatedSecret(generated, expr))
		}

		return match // Leave as-is if not found
	})
	return result
}

func generatedSecret(generated map[string]string, key string) string {
	if pw, ok := generated[key]; ok {
		return pw
	}
	pw := core.GeneratePassword(24)
	generated[key] = pw
	return pw
}

func isSensitiveEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"PASSWORD", "PASS", "PWD", "SECRET", "TOKEN", "KEY", "SALT"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func isWeakSecretDefault(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return false
	}
	switch v {
	case "admin", "authentik", "baserow", "bookstack", "change-this", "change-this-key",
		"change-this-secret", "change-this-too", "changeme", "huginn", "immich",
		"keycloak", "masterkey123", "matomo", "minioadmin", "paperless",
		"please-change-me", "rootpass", "seafile", "wiki":
		return true
	default:
		return false
	}
}

// DependencyOrder returns service names in startup order (respecting depends_on).
func (cf *ComposeFile) DependencyOrder() []string {
	visited := make(map[string]bool)
	var order []string

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true

		svc := cf.Services[name]
		if svc != nil {
			deps := parseDependsOn(svc.DependsOn)
			for _, dep := range deps {
				visit(dep)
			}
		}

		order = append(order, name)
	}

	for name := range cf.Services {
		visit(name)
	}

	return order
}

// parseDependsOn normalizes depends_on from list or map format.
func parseDependsOn(dep any) []string {
	if dep == nil {
		return nil
	}

	switch v := dep.(type) {
	case []any:
		var deps []string
		for _, item := range v {
			deps = append(deps, fmt.Sprintf("%v", item))
		}
		return deps
	case map[string]any:
		var deps []string
		for key := range v {
			deps = append(deps, key)
		}
		return deps
	}
	return nil
}
