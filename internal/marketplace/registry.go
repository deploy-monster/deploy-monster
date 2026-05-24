package marketplace

import (
	"regexp"
	"strings"
	"sync"
)

// Template represents a marketplace application template.
type Template struct {
	Slug         string         `json:"slug"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Category     string         `json:"category"`
	Icon         string         `json:"icon,omitempty"`
	Tags         []string       `json:"tags"`
	Author       string         `json:"author"`
	Version      string         `json:"version"`
	ComposeYAML  string         `json:"compose_yaml"`
	ConfigSchema map[string]any `json:"config_schema,omitempty"` // JSON Schema for user config
	MinResources ResourceReq    `json:"min_resources"`
	Featured     bool           `json:"featured"`
	Verified     bool           `json:"verified"`
}

// ResourceReq defines minimum resource requirements.
type ResourceReq struct {
	CPUMB    int `json:"cpu_mb"`
	MemoryMB int `json:"memory_mb"`
	DiskMB   int `json:"disk_mb"`
}

// TemplateRegistry holds all marketplace templates.
type TemplateRegistry struct {
	mu        sync.RWMutex
	templates map[string]*Template
}

// NewTemplateRegistry creates an empty template registry.
func NewTemplateRegistry() *TemplateRegistry {
	return &TemplateRegistry{templates: make(map[string]*Template)}
}

// Add registers a template.
func (r *TemplateRegistry) Add(t *Template) {
	if t == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates[t.Slug] = sanitizeTemplate(t)
}

// Get returns a template by slug.
func (r *TemplateRegistry) Get(slug string) *Template {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.templates[slug]
}

// Count returns total template count.
func (r *TemplateRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.templates)
}

// List returns all templates, optionally filtered by category.
func (r *TemplateRegistry) List(category string) []*Template {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Template
	for _, t := range r.templates {
		if category == "" || t.Category == category {
			result = append(result, t)
		}
	}
	return result
}

// Search performs full-text search over template names, descriptions, and tags.
func (r *TemplateRegistry) Search(query string) []*Template {
	r.mu.RLock()
	defer r.mu.RUnlock()

	q := strings.ToLower(query)
	var result []*Template
	for _, t := range r.templates {
		if strings.Contains(strings.ToLower(t.Name), q) ||
			strings.Contains(strings.ToLower(t.Description), q) ||
			containsTag(t.Tags, q) {
			result = append(result, t)
		}
	}
	return result
}

// Categories returns all unique categories.
func (r *TemplateRegistry) Categories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	for _, t := range r.templates {
		seen[t.Category] = true
	}
	cats := make([]string, 0, len(seen))
	for c := range seen {
		cats = append(cats, c)
	}
	return cats
}

func containsTag(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

var (
	composeDefaultExpr         = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*):-([^}]*)\}`)
	composeURLPasswordExpr     = regexp.MustCompile(`(://[^:\s/@]+:)([^@\s/"']+)(@)`)
	composeQueryPasswordExpr   = regexp.MustCompile(`([?&](?:p|pass|password|pwd)=)([^&\s"']+)`)
	composePasswordPlaceholder = "${DB_PASSWORD}"
)

func sanitizeTemplate(t *Template) *Template {
	if t == nil {
		return nil
	}
	cp := *t
	if len(t.Tags) > 0 {
		cp.Tags = append([]string(nil), t.Tags...)
	}
	cp.ComposeYAML = composeDefaultExpr.ReplaceAllStringFunc(t.ComposeYAML, func(match string) string {
		parts := composeDefaultExpr.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		if isSensitiveTemplateEnvKey(parts[1]) && isWeakTemplateSecretDefault(parts[2]) {
			return "${" + parts[1] + "}"
		}
		return match
	})
	cp.ComposeYAML = sanitizeSensitiveScalarDefaults(cp.ComposeYAML)
	cp.ComposeYAML = composeURLPasswordExpr.ReplaceAllStringFunc(cp.ComposeYAML, func(match string) string {
		parts := composeURLPasswordExpr.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		if isWeakTemplateSecretDefault(parts[2]) {
			return parts[1] + composePasswordPlaceholder + parts[3]
		}
		return match
	})
	cp.ComposeYAML = composeQueryPasswordExpr.ReplaceAllStringFunc(cp.ComposeYAML, func(match string) string {
		parts := composeQueryPasswordExpr.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		if isWeakTemplateSecretDefault(parts[2]) {
			return parts[1] + composePasswordPlaceholder
		}
		return match
	})
	return &cp
}

func sanitizeSensitiveScalarDefaults(composeYAML string) string {
	lines := strings.SplitAfter(composeYAML, "\n")
	for i, line := range lines {
		key, _, ok := weakSensitiveScalarDefault(line)
		if !ok {
			continue
		}
		body, newline := splitLineEnding(line)
		colon := strings.Index(body, ":")
		if colon < 0 {
			continue
		}
		afterColon := body[colon+1:]
		leadingValueSpace := afterColon[:len(afterColon)-len(strings.TrimLeft(afterColon, " \t"))]
		lines[i] = body[:colon+1] + leadingValueSpace + sensitiveTemplatePlaceholder(key) + newline
	}
	return strings.Join(lines, "")
}

func weakSensitiveScalarDefault(line string) (key string, value string, ok bool) {
	body, _ := splitLineEnding(line)
	trimmed := strings.TrimLeft(body, " \t")
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	colon := strings.Index(trimmed, ":")
	if colon <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(trimmed[:colon])
	if !isTemplateEnvKey(key) || !isSensitiveTemplateEnvKey(key) {
		return "", "", false
	}
	valuePart := strings.TrimSpace(trimmed[colon+1:])
	if valuePart == "" || strings.Contains(valuePart, "${") {
		return "", "", false
	}
	if comment := strings.Index(valuePart, "#"); comment >= 0 {
		valuePart = strings.TrimSpace(valuePart[:comment])
	}
	value = strings.Trim(valuePart, `"'`)
	if !isWeakTemplateSecretDefault(value) {
		return "", "", false
	}
	return key, value, true
}

func splitLineEnding(line string) (body string, newline string) {
	switch {
	case strings.HasSuffix(line, "\r\n"):
		return strings.TrimSuffix(line, "\r\n"), "\r\n"
	case strings.HasSuffix(line, "\n"):
		return strings.TrimSuffix(line, "\n"), "\n"
	default:
		return line, ""
	}
}

func isTemplateEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if i == 0 {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '_' {
				return false
			}
			continue
		}
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func isSensitiveTemplateEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"PASSWORD", "PASS", "PWD", "SECRET", "TOKEN", "SALT"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return upper == "KEY" || strings.HasSuffix(upper, "_KEY")
}

func sensitiveTemplatePlaceholder(key string) string {
	upper := strings.ToUpper(key)
	switch {
	case strings.Contains(upper, "ROOT_PASSWORD"):
		return "${DB_ROOT_PASSWORD}"
	case strings.Contains(upper, "DB") ||
		strings.Contains(upper, "DATABASE") ||
		strings.Contains(upper, "POSTGRES") ||
		strings.Contains(upper, "MYSQL") ||
		strings.Contains(upper, "MARIADB"):
		return composePasswordPlaceholder
	case strings.Contains(upper, "ADMIN_PASSWORD"):
		return "${ADMIN_PASSWORD}"
	case strings.Contains(upper, "JWT"):
		return "${JWT_SECRET}"
	case strings.Contains(upper, "SECRET_KEY"):
		return "${SECRET_KEY}"
	default:
		return "${" + key + "}"
	}
}

func isWeakTemplateSecretDefault(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return false
	}
	switch v {
	case "activepieces", "admin", "authentik", "authelia", "baserow", "bookstack",
		"change-this", "change-this-key", "change-this-secret", "change-this-too",
		"changeme", "ghostfolio", "huginn", "immich", "keycloak", "masterkey123",
		"matomo", "medusa", "minioadmin", "mmuser", "nocodb", "outline", "paperless",
		"penpot", "photoprism", "please-change-me", "projectsend", "rootpass",
		"seafile", "sylius", "trigger", "umami", "wiki", "zulip":
		return true
	default:
		return false
	}
}
