package topology

import (
	"fmt"
	"strconv"
	"strings"
)

// yamlCommandEscape ensures a string is safely embedded in YAML double-quotes.
// It escapes \, ", and control chars so that malicious input can't break out
// of the YAML string and inject arbitrary YAML structure.
func yamlCommandEscape(s string) string {
	return `"` + string(strconv.AppendQuote(nil, s)) + `"`
}

// ToYAML converts ComposeConfig to YAML string
func (c *ComposeConfig) ToYAML() string {
	var sb strings.Builder

	// Version
	if c.Version != "" {
		sb.WriteString(fmt.Sprintf("version: %q\n\n", c.Version))
	}

	// Services
	if len(c.Services) > 0 {
		sb.WriteString("services:\n")
		// Sort services by name for deterministic output
		names := make([]string, 0, len(c.Services))
		for name := range c.Services {
			names = append(names, name)
		}
		sortStrings(names)

		for _, name := range names {
			svc := c.Services[name]
			sb.WriteString(fmt.Sprintf("  %s:\n", name))
			sb.WriteString(svc.toYAML(4))
		}
		sb.WriteString("\n")
	}

	// Networks
	if len(c.Networks) > 0 {
		sb.WriteString("networks:\n")
		for name, net := range c.Networks {
			sb.WriteString(fmt.Sprintf("  %s:\n", name))
			if net.Driver != "" {
				sb.WriteString(fmt.Sprintf("    driver: %s\n", net.Driver))
			}
		}
		sb.WriteString("\n")
	}

	// Volumes
	if len(c.Volumes) > 0 {
		sb.WriteString("volumes:\n")
		for name, vol := range c.Volumes {
			sb.WriteString(fmt.Sprintf("  %s:\n", name))
			if vol.Driver != "" {
				sb.WriteString(fmt.Sprintf("    driver: %s\n", vol.Driver))
			}
			if vol.External {
				sb.WriteString("    external: true\n")
			}
		}
	}

	return sb.String()
}

// toYAML converts Service to YAML string with given indentation
func (s *Service) toYAML(indent int) string {
	var sb strings.Builder
	pad := strings.Repeat(" ", indent)

	// Image
	if s.Image != "" {
		sb.WriteString(fmt.Sprintf("%simage: %q\n", pad, s.Image))
	}

	// Build
	if s.Build != nil {
		sb.WriteString(fmt.Sprintf("%sbuild:\n", pad))
		if s.Build.Context != "" {
			sb.WriteString(fmt.Sprintf("%s  context: %q\n", pad, s.Build.Context))
		}
		if s.Build.Dockerfile != "" {
			sb.WriteString(fmt.Sprintf("%s  dockerfile: %q\n", pad, s.Build.Dockerfile))
		}
		if len(s.Build.Args) > 0 {
			sb.WriteString(fmt.Sprintf("%s  args:\n", pad))
			for k, v := range s.Build.Args {
				sb.WriteString(fmt.Sprintf("%s    %s: %q\n", pad, k, v))
			}
		}
	}

	// Container name
	if s.ContainerName != "" {
		sb.WriteString(fmt.Sprintf("%scontainer_name: %q\n", pad, s.ContainerName))
	}

	// Restart
	if s.Restart != "" {
		sb.WriteString(fmt.Sprintf("%srestart: %s\n", pad, s.Restart))
	}

	// Ports
	if len(s.Ports) > 0 {
		sb.WriteString(fmt.Sprintf("%sports:\n", pad))
		for _, p := range s.Ports {
			if p.Protocol != "" {
				sb.WriteString(fmt.Sprintf("%s  - \"%d:%d/%s\"\n", pad, p.Host, p.Container, p.Protocol))
			} else {
				sb.WriteString(fmt.Sprintf("%s  - \"%d:%d\"\n", pad, p.Host, p.Container))
			}
		}
	}

	// Expose
	if len(s.Expose) > 0 {
		sb.WriteString(fmt.Sprintf("%sexpose:\n", pad))
		for _, port := range s.Expose {
			sb.WriteString(fmt.Sprintf("%s  - %q\n", pad, strconv.Itoa(port)))
		}
	}

	// Environment
	if len(s.Environment) > 0 {
		sb.WriteString(fmt.Sprintf("%senvironment:\n", pad))
		// Sort for deterministic output
		keys := make([]string, 0, len(s.Environment))
		for k := range s.Environment {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			v := s.Environment[k]
			sb.WriteString(fmt.Sprintf("%s  %s: %q\n", pad, k, v))
		}
	}

	// Volumes
	if len(s.Volumes) > 0 {
		sb.WriteString(fmt.Sprintf("%svolumes:\n", pad))
		for _, v := range s.Volumes {
			sb.WriteString(fmt.Sprintf("%s  - %s\n", pad, v))
		}
	}

	// Networks
	if len(s.Networks) > 0 {
		sb.WriteString(fmt.Sprintf("%snetworks:\n", pad))
		for _, n := range s.Networks {
			sb.WriteString(fmt.Sprintf("%s  - %s\n", pad, n))
		}
	}

	// DependsOn
	if len(s.DependsOn) > 0 {
		sb.WriteString(fmt.Sprintf("%sdepends_on:\n", pad))
		for _, d := range s.DependsOn {
			sb.WriteString(fmt.Sprintf("%s  - %s\n", pad, d))
		}
	}

	// Labels
	if len(s.Labels) > 0 {
		sb.WriteString(fmt.Sprintf("%slabels:\n", pad))
		keys := make([]string, 0, len(s.Labels))
		for k := range s.Labels {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("%s  %s: %q\n", pad, k, s.Labels[k]))
		}
	}

	// Health check
	if s.HealthCheck != nil {
		sb.WriteString(fmt.Sprintf("%shealthcheck:\n", pad))
		if len(s.HealthCheck.Test) > 0 {
			sb.WriteString(fmt.Sprintf("%s  test: [", pad))
			for i, t := range s.HealthCheck.Test {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(fmt.Sprintf("%q", t))
			}
			sb.WriteString("]\n")
		}
		if s.HealthCheck.Interval != "" {
			sb.WriteString(fmt.Sprintf("%s  interval: %s\n", pad, s.HealthCheck.Interval))
		}
		if s.HealthCheck.Timeout != "" {
			sb.WriteString(fmt.Sprintf("%s  timeout: %s\n", pad, s.HealthCheck.Timeout))
		}
		if s.HealthCheck.Retries > 0 {
			sb.WriteString(fmt.Sprintf("%s  retries: %d\n", pad, s.HealthCheck.Retries))
		}
		if s.HealthCheck.Disable {
			sb.WriteString(fmt.Sprintf("%s  disable: true\n", pad))
		}
	}

	// Command
	if s.Command != "" {
		sb.WriteString(fmt.Sprintf("%scommand: %s\n", pad, yamlCommandEscape(s.Command)))
	}

	// Deploy
	if s.Deploy != nil {
		sb.WriteString(fmt.Sprintf("%sdeploy:\n", pad))
		if s.Deploy.Replicas > 0 {
			sb.WriteString(fmt.Sprintf("%s  replicas: %d\n", pad, s.Deploy.Replicas))
		}
		if s.Deploy.Resources != nil {
			sb.WriteString(fmt.Sprintf("%s  resources:\n", pad))
			if s.Deploy.Resources.Limits != nil {
				sb.WriteString(fmt.Sprintf("%s    limits:\n", pad))
				if s.Deploy.Resources.Limits.CPUs != "" {
					sb.WriteString(fmt.Sprintf("%s      cpus: %s\n", pad, s.Deploy.Resources.Limits.CPUs))
				}
				if s.Deploy.Resources.Limits.Memory != "" {
					sb.WriteString(fmt.Sprintf("%s      memory: %s\n", pad, s.Deploy.Resources.Limits.Memory))
				}
			}
		}
	}

	return sb.String()
}

// sortStrings sorts a slice of strings
func sortStrings(s []string) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
