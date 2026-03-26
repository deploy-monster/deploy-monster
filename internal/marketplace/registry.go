package marketplace

import (
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates[t.Slug] = t
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
