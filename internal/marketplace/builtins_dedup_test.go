package marketplace

import (
	"testing"
)

// TestBuiltinTemplates_NoDuplicateSlugsInRegistry asserts that after
// loading all builtins into the registry, there are no duplicate slugs.
// The builtinTemplates slice may contain duplicates (curated entries
// from the original files + bulk entries from the 100-list), but
// LoadBuiltins uses registry.Add which is map-based, so only the first
// entry for each slug survives. This test pins that behavior.
func TestBuiltinTemplates_NoDuplicateSlugsInRegistry(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	// The registry is a map — duplicates are impossible by construction.
	// Verify the count matches unique slugs in builtinTemplates.
	seen := make(map[string]bool)
	for _, tmpl := range builtinTemplates {
		seen[tmpl.Slug] = true
	}

	if r.Count() != len(seen) {
		t.Errorf("registry count %d != unique slugs %d in builtinTemplates",
			r.Count(), len(seen))
	}
}

// TestModuleInit_FinalTemplateCount pins the expected final registry
// size at startup so any accidental drop or double-counted add shows up
// in CI. Update the constant when templates are added or removed
// deliberately.
func TestModuleInit_FinalTemplateCount(t *testing.T) {
	const expected = 91

	r := NewTemplateRegistry()
	r.LoadBuiltins()

	if r.Count() != expected {
		t.Errorf("final template count = %d, want %d — adjust the constant only if you intentionally added or removed a template",
			r.Count(), expected)
	}
}

// TestBuiltinTemplates_CuratedEntriesPreserved verifies that templates
// which existed in the curated sets (with richer ConfigSchema, icons,
// resource requirements) still have their enriched fields after the
// consolidation.
func TestBuiltinTemplates_CuratedEntriesPreserved(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	// WordPress should have a ConfigSchema (curated entry)
	wp := r.Get("wordpress")
	if wp == nil {
		t.Fatal("wordpress template missing")
	}
	if wp.ConfigSchema == nil {
		t.Error("wordpress should have ConfigSchema from curated entry")
	}
	if wp.Icon == "" {
		t.Error("wordpress should have Icon from curated entry")
	}

	// Strapi should be version 5 (curated, not the bulk 100-list version 4)
	strapi := r.Get("strapi")
	if strapi == nil {
		t.Fatal("strapi template missing")
	}
	if strapi.Version != "5" {
		t.Errorf("strapi.Version = %q, want %q (curated version must win)",
			strapi.Version, "5")
	}
}
