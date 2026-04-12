package marketplace

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationError is the error type returned when a template fails structural
// validation. It carries both the template slug and the list of issues so
// callers can surface them verbatim in logs and UIs.
type ValidationError struct {
	Slug   string
	Issues []string
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 0 {
		return fmt.Sprintf("template %q: validation failed", e.Slug)
	}
	return fmt.Sprintf("template %q: %s", e.Slug, strings.Join(e.Issues, "; "))
}

// composeDoc is the minimal subset of a docker-compose document that we
// care about for marketplace validation. We deliberately do NOT model every
// compose field — that's the deployer's job. All we need is enough to
// confirm the template is structurally sound and that the services are
// internally consistent.
type composeDoc struct {
	Version  string                    `yaml:"version,omitempty"`
	Services map[string]*composeSvc    `yaml:"services"`
	Volumes  map[string]*composeVolume `yaml:"volumes,omitempty"`
	Networks map[string]*composeNetwk  `yaml:"networks,omitempty"`
}

type composeSvc struct {
	Image       string   `yaml:"image"`
	Build       any      `yaml:"build,omitempty"`
	Ports       []string `yaml:"ports,omitempty"`
	Volumes     []string `yaml:"volumes,omitempty"`
	Networks    any      `yaml:"networks,omitempty"`
	Environment any      `yaml:"environment,omitempty"`
	DependsOn   any      `yaml:"depends_on,omitempty"`
	Command     any      `yaml:"command,omitempty"`
	Entrypoint  any      `yaml:"entrypoint,omitempty"`
	Restart     string   `yaml:"restart,omitempty"`
	User        string   `yaml:"user,omitempty"`
	WorkingDir  string   `yaml:"working_dir,omitempty"`
	Healthcheck any      `yaml:"healthcheck,omitempty"`
}

type composeVolume struct {
	Driver   string            `yaml:"driver,omitempty"`
	External any               `yaml:"external,omitempty"`
	Name     string            `yaml:"name,omitempty"`
	Labels   map[string]string `yaml:"labels,omitempty"`
}

type composeNetwk struct {
	Driver   string `yaml:"driver,omitempty"`
	External any    `yaml:"external,omitempty"`
}

// ValidateTemplate runs structural and semantic checks against a single
// template. It returns nil on success, or a *ValidationError listing every
// issue found (the validator does NOT short-circuit on the first failure —
// it collects all issues so authors see the full picture in one pass).
//
// The checks are intentionally conservative: compose YAML is a big spec
// and docker-compose itself accepts many things we'd rather reject inside a
// marketplace context (unset images, services referencing undeclared
// volumes, missing bind paths). This validator focuses on errors that would
// cause a real deployment to fail or behave surprisingly.
func ValidateTemplate(t *Template) error {
	if t == nil {
		return errors.New("template is nil")
	}

	var issues []string
	addf := func(format string, args ...any) {
		issues = append(issues, fmt.Sprintf(format, args...))
	}

	// ---- required metadata ----
	if strings.TrimSpace(t.Slug) == "" {
		addf("slug is empty")
	}
	if strings.TrimSpace(t.Name) == "" {
		addf("name is empty")
	}
	if strings.TrimSpace(t.Description) == "" {
		addf("description is empty")
	}
	if strings.TrimSpace(t.Category) == "" {
		addf("category is empty")
	}
	if strings.TrimSpace(t.Author) == "" {
		addf("author is empty")
	}
	if strings.TrimSpace(t.Version) == "" {
		addf("version is empty")
	}
	if strings.TrimSpace(t.ComposeYAML) == "" {
		addf("compose_yaml is empty")
	}

	// ---- min resources sanity ----
	if t.MinResources.MemoryMB < 0 {
		addf("min_resources.memory_mb is negative: %d", t.MinResources.MemoryMB)
	}
	if t.MinResources.DiskMB < 0 {
		addf("min_resources.disk_mb is negative: %d", t.MinResources.DiskMB)
	}
	if t.MinResources.CPUMB < 0 {
		addf("min_resources.cpu_mb is negative: %d", t.MinResources.CPUMB)
	}

	// If compose is empty we can't do structural checks — short circuit
	// after metadata so the author sees both classes of issue in one pass.
	if strings.TrimSpace(t.ComposeYAML) == "" {
		return &ValidationError{Slug: t.Slug, Issues: issues}
	}

	// ---- compose document ----
	var doc composeDoc
	if err := yaml.Unmarshal([]byte(t.ComposeYAML), &doc); err != nil {
		addf("compose_yaml is not valid YAML: %v", err)
		return &ValidationError{Slug: t.Slug, Issues: issues}
	}
	if len(doc.Services) == 0 {
		addf("compose_yaml declares no services")
		return &ValidationError{Slug: t.Slug, Issues: issues}
	}

	// Track declared volume/network names so we can cross-check service
	// references below. A service may use a named volume only if that name
	// is declared at the top level (bind mounts starting with / or . are
	// always allowed).
	declaredVolumes := make(map[string]bool, len(doc.Volumes))
	for name := range doc.Volumes {
		declaredVolumes[name] = true
	}

	// ---- per-service checks ----
	for svcName, svc := range doc.Services {
		if svc == nil {
			addf("service %q: empty definition", svcName)
			continue
		}
		// Every service must name an image or have a build block. The
		// marketplace is image-first — a build block is legal but we warn
		// so it's obvious the template needs source bind-mounted in.
		if strings.TrimSpace(svc.Image) == "" && svc.Build == nil {
			addf("service %q: missing image (and no build block)", svcName)
		}

		// Volume references must be either bind mounts or declared top-level.
		for _, v := range svc.Volumes {
			if v == "" {
				addf("service %q: empty volume entry", svcName)
				continue
			}
			// Bind mount: "/host/path:/container/path" or "./rel:/container".
			// "${VAR}:/path" is also a legal bind mount — the variable expands
			// at deploy time, so we can't know the final prefix here, but we
			// can detect the pattern and let the deployer validate the result.
			if strings.HasPrefix(v, "/") || strings.HasPrefix(v, ".") || strings.HasPrefix(v, "~") || strings.HasPrefix(v, "${") || strings.HasPrefix(v, "$(") {
				continue
			}
			// Named volume: "volname:/container/path" — extract volname.
			name := v
			if i := strings.Index(v, ":"); i >= 0 {
				name = v[:i]
			}
			if name == "" {
				addf("service %q: malformed volume entry %q", svcName, v)
				continue
			}
			if !declaredVolumes[name] {
				addf("service %q: volume %q is not declared at the top level", svcName, name)
			}
		}

		// Port entries must look like "host:container" or a bare number.
		// We reject ports with letters (except the protocol suffix /tcp,
		// /udp) because those rarely parse the way authors expect.
		for _, p := range svc.Ports {
			if strings.TrimSpace(p) == "" {
				addf("service %q: empty port entry", svcName)
				continue
			}
			if !looksLikePort(p) {
				addf("service %q: port %q does not look like host:container", svcName, p)
			}
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Slug: t.Slug, Issues: issues}
	}
	return nil
}

// looksLikePort checks whether a compose port entry is structurally sane.
// Accepts: "80", "80:80", "127.0.0.1:8080:80", "8080:80/tcp", "8080-8090:80-90".
// Rejects entries with unexpected letters outside the /proto suffix — those
// are almost always typos.
func looksLikePort(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	// Strip optional /proto suffix first.
	if i := strings.LastIndex(p, "/"); i >= 0 {
		proto := p[i+1:]
		if proto != "tcp" && proto != "udp" && proto != "sctp" {
			return false
		}
		p = p[:i]
	}
	// Now every character must be a digit, a colon, a dot, or a dash
	// (ranges like 8080-8090 are legal).
	for _, c := range p {
		switch {
		case c >= '0' && c <= '9':
		case c == ':' || c == '.' || c == '-':
		default:
			return false
		}
	}
	return true
}

// ValidationResult pairs a template slug with the validation outcome. Used
// by Registry.ValidateAll so a caller gets a per-slug breakdown in a single
// call instead of walking the registry manually.
type ValidationResult struct {
	Slug   string
	Err    error // nil means valid; *ValidationError otherwise
	Issues []string
}

// ValidateAll runs ValidateTemplate against every template in the registry
// and returns one ValidationResult per slug (in lexicographic order, so
// test output stays deterministic). The slice is never nil even when the
// registry is empty.
func (r *TemplateRegistry) ValidateAll() []ValidationResult {
	r.mu.RLock()
	all := make([]*Template, 0, len(r.templates))
	for _, t := range r.templates {
		all = append(all, t)
	}
	r.mu.RUnlock()

	// Deterministic order so test diffs are readable.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j-1].Slug > all[j].Slug; j-- {
			all[j-1], all[j] = all[j], all[j-1]
		}
	}

	out := make([]ValidationResult, 0, len(all))
	for _, t := range all {
		res := ValidationResult{Slug: t.Slug}
		if err := ValidateTemplate(t); err != nil {
			res.Err = err
			var ve *ValidationError
			if errors.As(err, &ve) {
				res.Issues = append(res.Issues, ve.Issues...)
			} else {
				res.Issues = append(res.Issues, err.Error())
			}
		}
		out = append(out, res)
	}
	return out
}
