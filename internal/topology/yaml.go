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
		fmt.Fprintf(&sb, "version: %q\n\n", c.Version)
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
			fmt.Fprintf(&sb, "  %s:\n", name)
			sb.WriteString(svc.toYAML(4))
		}
		sb.WriteString("\n")
	}

	// Networks
	if len(c.Networks) > 0 {
		sb.WriteString("networks:\n")
		for name, net := range c.Networks {
			fmt.Fprintf(&sb, "  %s:\n", name)
			if net.Driver != "" {
				fmt.Fprintf(&sb, "    driver: %s\n", net.Driver)
			}
		}
		sb.WriteString("\n")
	}

	// Volumes
	if len(c.Volumes) > 0 {
		sb.WriteString("volumes:\n")
		for name, vol := range c.Volumes {
			fmt.Fprintf(&sb, "  %s:\n", name)
			if vol.Driver != "" {
				fmt.Fprintf(&sb, "    driver: %s\n", vol.Driver)
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
		fmt.Fprintf(&sb, "%simage: %q\n", pad, s.Image)
	}

	// Build
	if s.Build != nil {
		fmt.Fprintf(&sb, "%sbuild:\n", pad)
		if s.Build.Context != "" {
			fmt.Fprintf(&sb, "%s  context: %q\n", pad, s.Build.Context)
		}
		if s.Build.Dockerfile != "" {
			fmt.Fprintf(&sb, "%s  dockerfile: %q\n", pad, s.Build.Dockerfile)
		}
		if len(s.Build.Args) > 0 {
			fmt.Fprintf(&sb, "%s  args:\n", pad)
			for k, v := range s.Build.Args {
				fmt.Fprintf(&sb, "%s    %s: %q\n", pad, k, v)
			}
		}
	}

	// Container name
	if s.ContainerName != "" {
		fmt.Fprintf(&sb, "%scontainer_name: %q\n", pad, s.ContainerName)
	}

	// Restart
	if s.Restart != "" {
		fmt.Fprintf(&sb, "%srestart: %s\n", pad, s.Restart)
	}

	// Ports
	if len(s.Ports) > 0 {
		fmt.Fprintf(&sb, "%sports:\n", pad)
		for _, p := range s.Ports {
			if p.Protocol != "" {
				fmt.Fprintf(&sb, "%s  - \"%d:%d/%s\"\n", pad, p.Host, p.Container, p.Protocol)
			} else {
				fmt.Fprintf(&sb, "%s  - \"%d:%d\"\n", pad, p.Host, p.Container)
			}
		}
	}

	// Expose
	if len(s.Expose) > 0 {
		fmt.Fprintf(&sb, "%sexpose:\n", pad)
		for _, port := range s.Expose {
			fmt.Fprintf(&sb, "%s  - %q\n", pad, strconv.Itoa(port))
		}
	}

	// Environment
	if len(s.Environment) > 0 {
		fmt.Fprintf(&sb, "%senvironment:\n", pad)
		// Sort for deterministic output
		keys := make([]string, 0, len(s.Environment))
		for k := range s.Environment {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			v := s.Environment[k]
			fmt.Fprintf(&sb, "%s  %s: %q\n", pad, k, v)
		}
	}

	// Volumes
	if len(s.Volumes) > 0 {
		fmt.Fprintf(&sb, "%svolumes:\n", pad)
		for _, v := range s.Volumes {
			fmt.Fprintf(&sb, "%s  - %s\n", pad, v)
		}
	}

	// Networks
	if len(s.Networks) > 0 {
		fmt.Fprintf(&sb, "%snetworks:\n", pad)
		for _, n := range s.Networks {
			fmt.Fprintf(&sb, "%s  - %s\n", pad, n)
		}
	}

	// DependsOn
	if len(s.DependsOn) > 0 {
		fmt.Fprintf(&sb, "%sdepends_on:\n", pad)
		for _, d := range s.DependsOn {
			fmt.Fprintf(&sb, "%s  - %s\n", pad, d)
		}
	}

	// Labels
	if len(s.Labels) > 0 {
		fmt.Fprintf(&sb, "%slabels:\n", pad)
		keys := make([]string, 0, len(s.Labels))
		for k := range s.Labels {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			fmt.Fprintf(&sb, "%s  %s: %q\n", pad, k, s.Labels[k])
		}
	}

	// Health check
	if s.HealthCheck != nil {
		fmt.Fprintf(&sb, "%shealthcheck:\n", pad)
		if len(s.HealthCheck.Test) > 0 {
			fmt.Fprintf(&sb, "%s  test: [", pad)
			for i, t := range s.HealthCheck.Test {
				if i > 0 {
					sb.WriteString(", ")
				}
				fmt.Fprintf(&sb, "%q", t)
			}
			sb.WriteString("]\n")
		}
		if s.HealthCheck.Interval != "" {
			fmt.Fprintf(&sb, "%s  interval: %s\n", pad, s.HealthCheck.Interval)
		}
		if s.HealthCheck.Timeout != "" {
			fmt.Fprintf(&sb, "%s  timeout: %s\n", pad, s.HealthCheck.Timeout)
		}
		if s.HealthCheck.Retries > 0 {
			fmt.Fprintf(&sb, "%s  retries: %d\n", pad, s.HealthCheck.Retries)
		}
		if s.HealthCheck.Disable {
			fmt.Fprintf(&sb, "%s  disable: true\n", pad)
		}
	}

	// Command
	if s.Command != "" {
		fmt.Fprintf(&sb, "%scommand: %s\n", pad, yamlCommandEscape(s.Command))
	}

	// Deploy
	if s.Deploy != nil {
		fmt.Fprintf(&sb, "%sdeploy:\n", pad)
		if s.Deploy.Replicas > 0 {
			fmt.Fprintf(&sb, "%s  replicas: %d\n", pad, s.Deploy.Replicas)
		}
		if s.Deploy.Resources != nil {
			fmt.Fprintf(&sb, "%s  resources:\n", pad)
			if s.Deploy.Resources.Limits != nil {
				fmt.Fprintf(&sb, "%s    limits:\n", pad)
				if s.Deploy.Resources.Limits.CPUs != "" {
					fmt.Fprintf(&sb, "%s      cpus: %s\n", pad, s.Deploy.Resources.Limits.CPUs)
				}
				if s.Deploy.Resources.Limits.Memory != "" {
					fmt.Fprintf(&sb, "%s      memory: %s\n", pad, s.Deploy.Resources.Limits.Memory)
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
