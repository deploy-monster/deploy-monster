package marketplace

import (
	"context"
	"testing"
)

// TestAllBuiltinTemplates_RequiredFields verifies every built-in template has
// the mandatory fields populated: Name, Description, Category, and ComposeYAML
// (which implies an image is specified inside the compose definition).
func TestAllBuiltinTemplates_RequiredFields(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	if r.Count() == 0 {
		t.Fatal("no builtin templates loaded")
	}

	for _, tmpl := range r.List("") {
		t.Run(tmpl.Slug, func(t *testing.T) {
			if tmpl.Slug == "" {
				t.Error("slug must not be empty")
			}
			if tmpl.Name == "" {
				t.Errorf("template %q: name must not be empty", tmpl.Slug)
			}
			if tmpl.Description == "" {
				t.Errorf("template %q: description must not be empty", tmpl.Slug)
			}
			if tmpl.Category == "" {
				t.Errorf("template %q: category must not be empty", tmpl.Slug)
			}
			if tmpl.ComposeYAML == "" {
				t.Errorf("template %q: compose YAML must not be empty", tmpl.Slug)
			}
			if tmpl.Author == "" {
				t.Errorf("template %q: author must not be empty", tmpl.Slug)
			}
			if tmpl.Version == "" {
				t.Errorf("template %q: version must not be empty", tmpl.Slug)
			}
		})
	}
}

func TestBuiltinTemplates_NoWeakSecretFallbacks(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()
	for _, tmpl := range GetMoreTemplates100() {
		r.Add(tmpl)
	}
	for _, tmpl := range r.List("") {
		for _, match := range composeDefaultExpr.FindAllStringSubmatch(tmpl.ComposeYAML, -1) {
			if len(match) != 3 {
				continue
			}
			if isSensitiveTemplateEnvKey(match[1]) && isWeakTemplateSecretDefault(match[2]) {
				t.Fatalf("template %s contains weak secret fallback %q", tmpl.Slug, match[0])
			}
		}
	}
}

// TestAllBuiltinTemplates_ComposeContainsImage ensures every compose YAML
// contains an "image:" directive, meaning it references a container image.
func TestAllBuiltinTemplates_ComposeContainsImage(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	for _, tmpl := range r.List("") {
		t.Run(tmpl.Slug, func(t *testing.T) {
			if !containsSubstring(tmpl.ComposeYAML, "image:") {
				t.Errorf("template %q: compose YAML should contain an image directive", tmpl.Slug)
			}
		})
	}
}

// TestAllBuiltinTemplates_UniqueSlugs verifies no two templates share the same slug.
func TestAllBuiltinTemplates_UniqueSlugs(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	seen := make(map[string]bool)
	for _, tmpl := range r.List("") {
		if seen[tmpl.Slug] {
			t.Errorf("duplicate slug: %q", tmpl.Slug)
		}
		seen[tmpl.Slug] = true
	}
}

// TestSearchByCategory_FiltersByExactCategory verifies filtering templates
// by category returns only matching templates.
func TestSearchByCategory_FiltersByExactCategory(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	categories := r.Categories()
	if len(categories) < 2 {
		t.Fatalf("expected at least 2 categories, got %d", len(categories))
	}

	for _, cat := range categories {
		results := r.List(cat)
		if len(results) == 0 {
			t.Errorf("category %q returned no results", cat)
			continue
		}
		for _, tmpl := range results {
			if tmpl.Category != cat {
				t.Errorf("List(%q) returned template %q with category %q", cat, tmpl.Slug, tmpl.Category)
			}
		}
	}
}

// TestSearchByCategory_NonexistentReturnsEmpty ensures a non-existent category
// returns an empty list.
func TestSearchByCategory_NonexistentReturnsEmpty(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	results := r.List("nonexistent-category-xyz")
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent category, got %d", len(results))
	}
}

// TestSearchByName_CaseInsensitive verifies that Search is case-insensitive
// for template names.
func TestSearchByName_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"lowercase", "wordpress"},
		{"uppercase", "WORDPRESS"},
		{"mixed case", "WordPress"},
		{"partial", "word"},
	}

	r := NewTemplateRegistry()
	r.LoadBuiltins()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			results := r.Search(tc.query)
			found := false
			for _, tmpl := range results {
				if tmpl.Slug == "wordpress" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Search(%q) should find wordpress template", tc.query)
			}
		})
	}
}

// TestSearch_ByDescription verifies searching by content in the description.
func TestSearch_ByDescription(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	// "content management" should match WordPress
	results := r.Search("content management")
	found := false
	for _, tmpl := range results {
		if tmpl.Slug == "wordpress" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Search('content management') should find wordpress template")
	}
}

// TestSearch_ByTag verifies searching by tag values.
func TestSearch_ByTag(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	results := r.Search("blog")
	if len(results) == 0 {
		t.Error("Search('blog') should return at least one result")
	}

	// WordPress and Ghost both have "blog" tag
	foundWP := false
	foundGhost := false
	for _, tmpl := range results {
		if tmpl.Slug == "wordpress" {
			foundWP = true
		}
		if tmpl.Slug == "ghost" {
			foundGhost = true
		}
	}
	if !foundWP {
		t.Error("Search('blog') should find wordpress")
	}
	if !foundGhost {
		t.Error("Search('blog') should find ghost")
	}
}

// TestSearch_NoResults verifies that a nonsensical query returns nothing.
func TestSearch_NoResults(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	results := r.Search("xyznonexistent123456")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestSearch_EmptyQuery verifies that an empty query returns all templates.
func TestSearch_EmptyQuery(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	results := r.Search("")
	all := r.List("")
	if len(results) != len(all) {
		t.Errorf("empty search should return all %d templates, got %d", len(all), len(results))
	}
}

// TestTemplateDeployConfig_MinResources validates that templates with
// resource requirements have sensible values.
func TestTemplateDeployConfig_MinResources(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	for _, tmpl := range r.List("") {
		t.Run(tmpl.Slug, func(t *testing.T) {
			if tmpl.MinResources.MemoryMB < 0 {
				t.Errorf("template %q: memory_mb should not be negative, got %d", tmpl.Slug, tmpl.MinResources.MemoryMB)
			}
			if tmpl.MinResources.DiskMB < 0 {
				t.Errorf("template %q: disk_mb should not be negative, got %d", tmpl.Slug, tmpl.MinResources.DiskMB)
			}
			if tmpl.MinResources.CPUMB < 0 {
				t.Errorf("template %q: cpu_mb should not be negative, got %d", tmpl.Slug, tmpl.MinResources.CPUMB)
			}
		})
	}
}

// TestTemplateDeployConfig_VerifiedTemplatesHaveAuthor ensures that all
// verified templates have an author set.
func TestTemplateDeployConfig_VerifiedTemplatesHaveAuthor(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	for _, tmpl := range r.List("") {
		if tmpl.Verified && tmpl.Author == "" {
			t.Errorf("verified template %q should have an author", tmpl.Slug)
		}
	}
}

// TestModule_Lifecycle verifies the Module metadata methods.
func TestModule_Lifecycle(t *testing.T) {
	m := New()

	if m.ID() != "marketplace" {
		t.Errorf("expected ID 'marketplace', got %q", m.ID())
	}
	if m.Name() != "Marketplace" {
		t.Errorf("expected Name 'Marketplace', got %q", m.Name())
	}
	if m.Version() != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %q", m.Version())
	}
	// Health should return HealthOK (0) for an uninitialized marketplace module
	if got := m.Health(); got != 0 {
		t.Errorf("expected Health() == 0 (HealthOK), got %d", got)
	}

	deps := m.Dependencies()
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(deps))
	}
	if deps[0] != "core.db" {
		t.Errorf("expected first dependency 'core.db', got %q", deps[0])
	}
	if deps[1] != "deploy" {
		t.Errorf("expected second dependency 'deploy', got %q", deps[1])
	}

	routes := m.Routes()
	if routes != nil {
		t.Errorf("expected nil routes, got %v", routes)
	}

	events := m.Events()
	if events != nil {
		t.Errorf("expected nil events, got %v", events)
	}
}

// TestModule_StopIsIdempotent verifies that Stop can be called on an
// uninitialized module without error.
func TestModule_StopIsIdempotent(t *testing.T) {
	m := New()
	if err := m.Stop(context.TODO()); err != nil {
		t.Errorf("Stop on uninitialized module should not error, got: %v", err)
	}
}

// TestRegistry_Count verifies Count reflects the number of added templates.
func TestRegistry_Count(t *testing.T) {
	r := NewTemplateRegistry()

	if r.Count() != 0 {
		t.Errorf("new registry should have 0 templates, got %d", r.Count())
	}

	r.Add(&Template{Slug: "a", Name: "A", Category: "cat"})
	r.Add(&Template{Slug: "b", Name: "B", Category: "cat"})

	if r.Count() != 2 {
		t.Errorf("expected 2 templates, got %d", r.Count())
	}
}

// TestRegistry_AddOverwritesSameSlug verifies adding a template with the
// same slug overwrites the previous one.
func TestRegistry_AddOverwritesSameSlug(t *testing.T) {
	r := NewTemplateRegistry()

	r.Add(&Template{Slug: "test", Name: "Original", Category: "cat"})
	r.Add(&Template{Slug: "test", Name: "Updated", Category: "cat"})

	if r.Count() != 1 {
		t.Errorf("overwriting slug should not increase count, got %d", r.Count())
	}

	tmpl := r.Get("test")
	if tmpl.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %q", tmpl.Name)
	}
}

// TestRegistry_GetNonexistent verifies Get returns nil for missing slugs.
func TestRegistry_GetNonexistent(t *testing.T) {
	r := NewTemplateRegistry()
	if r.Get("missing") != nil {
		t.Error("Get should return nil for nonexistent slug")
	}
}

// TestBuiltins_FeaturedTemplatesExist ensures at least some templates are
// marked as featured.
func TestBuiltins_FeaturedTemplatesExist(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	featuredCount := 0
	for _, tmpl := range r.List("") {
		if tmpl.Featured {
			featuredCount++
		}
	}
	if featuredCount == 0 {
		t.Error("expected at least one featured template")
	}
}

// TestBuiltins_TemplateCount ensures we have the expected minimum number of
// built-in templates (20+).
func TestBuiltins_TemplateCount(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	count := r.Count()
	if count < 20 {
		t.Errorf("expected at least 20 builtin templates, got %d", count)
	}
}

// TestCategories_ReturnsUniqueValues verifies Categories returns unique
// category strings with no duplicates.
func TestCategories_ReturnsUniqueValues(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	cats := r.Categories()
	seen := make(map[string]bool)
	for _, c := range cats {
		if seen[c] {
			t.Errorf("duplicate category: %q", c)
		}
		seen[c] = true
	}
}

// containsSubstring is a test helper that checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
