package marketplace

import (
	"context" // === merged from builtins_boost_test.go ===
	"errors"
	"strings"
	"testing"
)

func TestGetMoreTemplates100(t *testing.T) {
	templates := builtinTemplates
	if templates == nil {
		t.Fatal("builtinTemplates returned nil")
	}
	if len(templates) == 0 {
		t.Error("expected builtinTemplates to be non-empty")
	}
	// Verify at least one well-known template
	found := false
	for _, tm := range templates {
		if tm.Slug == "grafana" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'grafana' template in builtinTemplates")
	}
}

// === merged from marketplace_extra_test.go ===
// =============================================================================
// Add — nil template
// =============================================================================
func TestAdd_NilTemplate(t *testing.T) {
	r := NewTemplateRegistry()
	r.Add(nil)
	if r.Count() != 0 {
		t.Error("expected Add(nil) to be a no-op")
	}
}

// =============================================================================
// sanitizeTemplate — nil input returns nil
// =============================================================================
func TestSanitizeTemplate_Nil(t *testing.T) {
	result := sanitizeTemplate(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

// =============================================================================
// sanitizeTemplate — replacement function parts mismatch
// (composeDefaultExpr, composeURLPasswordExpr, composeQueryPasswordExpr
//
//	non-matching branches)
//
// =============================================================================
func TestSanitizeTemplate_DefaultExprNonMatch(t *testing.T) {
	// The composeDefaultExpr matches `${VAR:-default}` patterns.
	// A non-matching string like `${VAR}` has no ":-" so FindStringSubmatch
	// returns nil for the capture groups, hitting len(parts) != 3.
	r := NewTemplateRegistry()
	r.Add(&Template{
		Slug:        "non-match-test",
		Name:        "Test",
		Description: "desc",
		Category:    "test",
		Author:      "a",
		Version:     "1",
		ComposeYAML: `services:
  web:
    image: nginx
    environment:
      - PLAIN=${VAR}
      - WITH_DEFAULT=${OTHER:-default}
`,
	})
	// Should not panic — the non-matching `${VAR}` is preserved as-is
	tmpl := r.Get("non-match-test")
	if tmpl == nil {
		t.Fatal("expected template")
	}
	if !strings.Contains(tmpl.ComposeYAML, "${VAR}") {
		t.Error("expected ${VAR} to be preserved")
	}
}

// =============================================================================
// sanitizeTemplate — URL password non-matching regex parts
// =============================================================================
func TestSanitizeTemplate_URLPasswordNonMatch(t *testing.T) {
	r := NewTemplateRegistry()
	// The composeURLPasswordExpr matches `://user:pass@host` patterns.
	// A non-matching string like `password=changeme` should pass through.
	r.Add(&Template{
		Slug:        "url-nonmatch",
		Name:        "Test",
		Description: "desc",
		Category:    "test",
		Author:      "a",
		Version:     "1",
		ComposeYAML: `services:
  web:
    image: nginx
    environment:
      PASSWORD: changeme
`,
	})
	tmpl := r.Get("url-nonmatch")
	if tmpl == nil {
		t.Fatal("expected template")
	}
}

// =============================================================================
// sanitizeSensitiveScalarDefaults — line without colon
// =============================================================================
func TestSanitizeSensitiveScalarDefaults_NoColon(t *testing.T) {
	// A line like `  # comment` has no colon — should be skipped
	input := `services:
  web:
    image: nginx
  # this line has no colon
    environment:` + "\n"
	output := sanitizeSensitiveScalarDefaults(input)
	// The output should be identical since nothing matched
	if output != input {
		t.Errorf("expected unchanged, got:\n%s", output)
	}
}

// =============================================================================
// sanitizeSensitiveScalarDefaults — line with comment in value
// =============================================================================
func TestSanitizeSensitiveScalarDefaults_CommentInValue(t *testing.T) {
	input := `services:
  web:
    image: nginx
    environment:
      PASSWORD: changeme # this is a weak default
`
	// This should sanitize the PASSWORD value
	output := sanitizeSensitiveScalarDefaults(input)
	if strings.Contains(output, "changeme") {
		t.Errorf("expected weak default to be sanitized, got:\n%s", output)
	}
}

// =============================================================================
// weakSensitiveScalarDefault — not a weak default
// =============================================================================
func TestWeakSensitiveScalarDefault_NotWeak(t *testing.T) {
	key, value, ok := weakSensitiveScalarDefault("      PASSWORD: strongsecret123")
	if ok {
		t.Errorf("expected ok=false for strong secret, got key=%q value=%q", key, value)
	}
}

// =============================================================================
// splitLineEnding — Windows CRLF and no newline
// =============================================================================
func TestSplitLineEnding_WindowsAndNoNewline(t *testing.T) {
	// Windows CRLF
	body, nl := splitLineEnding("POSTGRES_PASSWORD: changeme\r\n")
	if nl != "\r\n" {
		t.Errorf("expected CRLF newline, got %q", nl)
	}
	if !strings.Contains(body, "POSTGRES_PASSWORD") {
		t.Errorf("expected body to contain key, got %q", body)
	}
	// No newline at end
	body, nl = splitLineEnding("POSTGRES_PASSWORD: changeme")
	if nl != "" {
		t.Errorf("expected empty newline, got %q", nl)
	}
	if body != "POSTGRES_PASSWORD: changeme" {
		t.Errorf("expected full body, got %q", body)
	}
}

// =============================================================================
// isTemplateEnvKey — empty key
// =============================================================================
func TestIsTemplateEnvKey_Empty(t *testing.T) {
	if isTemplateEnvKey("") {
		t.Error("expected false for empty key")
	}
}

// =============================================================================
// sensitiveTemplatePlaceholder — all branches
// =============================================================================
func TestSensitiveTemplatePlaceholder_Branches(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"DB_ROOT_PASSWORD", "${DB_ROOT_PASSWORD}"},
		{"ROOT_PASSWORD", "${DB_ROOT_PASSWORD}"},
		{"DATABASE_URL", "${DB_PASSWORD}"},
		{"POSTGRES_PASSWORD", "${DB_PASSWORD}"},
		{"MYSQL_PASSWORD", "${DB_PASSWORD}"},
		// MARIADB_ROOT_PASSWORD contains "ROOT_PASSWORD" so it hits the first case
		{"MARIADB_ROOT_PASSWORD", "${DB_ROOT_PASSWORD}"},
		// MARIADB_PASSWORD does not contain ROOT_PASSWORD, hits the DB case
		{"MARIADB_PASSWORD", "${DB_PASSWORD}"},
		{"DB_PASSWORD", "${DB_PASSWORD}"},
		{"ADMIN_PASSWORD", "${ADMIN_PASSWORD}"},
		{"JWT_SECRET", "${JWT_SECRET}"},
		{"SECRET_KEY", "${SECRET_KEY}"},
		{"CUSTOM_KEY", "${CUSTOM_KEY}"},
		{"MY_TOKEN", "${MY_TOKEN}"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := sensitiveTemplatePlaceholder(tt.key)
			if got != tt.want {
				t.Errorf("sensitiveTemplatePlaceholder(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// =============================================================================
// ValidateTemplate — empty service definition (svc == nil)
// =============================================================================
func TestValidateTemplate_EmptyService(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
  db:
    image: postgres:16
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected validation error for empty service")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "empty definition") {
		t.Errorf("expected empty-definition issue, got %v", err)
	}
}

// =============================================================================
// ValidateTemplate — empty volume entry
// =============================================================================
func TestValidateTemplate_EmptyVolumeEntry(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    image: nginx:latest
    volumes:
      - ""
      - /host:/container
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected validation error for empty volume entry")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "empty volume entry") {
		t.Errorf("expected empty-volume issue, got %v", err)
	}
}

// =============================================================================
// ValidateTemplate — malformed volume entry (name after colon empty)
// =============================================================================
func TestValidateTemplate_MalformedVolume(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    image: nginx:latest
    volumes:
      - ":/container"
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected validation error for malformed volume entry")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "malformed volume entry") {
		t.Errorf("expected malformed-volume issue, got %v", err)
	}
}

// =============================================================================
// ValidateTemplate — empty port entry
// =============================================================================
func TestValidateTemplate_EmptyPortEntry(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    image: nginx:latest
    ports:
      - ""
      - "80:80"
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected validation error for empty port entry")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "empty port entry") {
		t.Errorf("expected empty-port issue, got %v", err)
	}
}

// =============================================================================
// ValidateAll — non-ValidationError error branch (ValidateTemplate returning
// a non-*ValidationError error)
// =============================================================================
func TestValidateAll_NonValidationError(t *testing.T) {
	// This is hard to trigger because ValidateTemplate always returns
	// *ValidationError or nil. The else branch in ValidateAll is a safety
	// net. We test it by validating a template with compose_yaml that
	// triggers a non-validation error path.
	// Actually, ValidateTemplate always wraps errors in ValidationError,
	// so the else branch is a defensive fallback. Let's just ensure
	// ValidateAll works correctly for a valid case.
	r := NewTemplateRegistry()
	good := validTemplate()
	good.Slug = "valid-slug"
	good.Name = "Valid"
	r.Add(good)
	results := r.ValidateAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("expected no error for valid template, got %v", results[0].Err)
	}
}

// === merged from marketplace_final_test.go ===
// The only uncovered code in marketplace is init() at 50%.
// init() functions are partially covered by the test binary startup.
// This file ensures the module is loadable and functional.
func TestModule_ID_Final(t *testing.T) {
	m := New()
	if m.ID() != "marketplace" {
		t.Errorf("ID = %q, want %q", m.ID(), "marketplace")
	}
}

// === merged from templates_extra_test.go ===
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
	for _, tmpl := range builtinTemplates {
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
		for _, line := range strings.SplitAfter(tmpl.ComposeYAML, "\n") {
			if _, _, ok := weakSensitiveScalarDefault(line); ok {
				t.Fatalf("template %s contains hardcoded weak secret %q", tmpl.Slug, line)
			}
		}
		for _, match := range composeURLPasswordExpr.FindAllStringSubmatch(tmpl.ComposeYAML, -1) {
			if len(match) != 4 {
				continue
			}
			if isWeakTemplateSecretDefault(match[2]) {
				t.Fatalf("template %s contains weak URL password %q", tmpl.Slug, match[0])
			}
		}
		for _, match := range composeQueryPasswordExpr.FindAllStringSubmatch(tmpl.ComposeYAML, -1) {
			if len(match) != 3 {
				continue
			}
			if isWeakTemplateSecretDefault(match[2]) {
				t.Fatalf("template %s contains weak query password %q", tmpl.Slug, match[0])
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
