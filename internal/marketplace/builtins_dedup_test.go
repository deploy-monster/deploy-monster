package marketplace

import (
	"sort"
	"testing"
)

// TestBuiltinTemplates_NoDuplicateSlugsWithinSlice asserts no duplicates
// inside the primary builtinTemplates slice — the union of builtins.go,
// builtins_extra.go, builtins_extended.go, and builtins_more.go which
// all append to it via init(). This is the "curated" catalog. Any dupe
// inside this slice is a bug: builtinTemplates is iterated as a slice by
// LoadBuiltins, so a repeated slug just wastes a registry.Add call and
// makes maintenance confusing.
func TestBuiltinTemplates_NoDuplicateSlugsWithinSlice(t *testing.T) {
	assertNoInternalDupes(t, "builtinTemplates", builtinTemplates)
}

// TestMoreTemplates100_NoDuplicateSlugsWithinSlice — same invariant for
// the bulk 100-list.
func TestMoreTemplates100_NoDuplicateSlugsWithinSlice(t *testing.T) {
	assertNoInternalDupes(t, "moreTemplates100", moreTemplates100)
}

// TestModuleInit_CuratedBuiltinsWinOverBulkList pins the dedup semantics
// introduced alongside this test. The `builtinTemplates` slice is the
// curated catalog (richer schemas, newer images, icons) and the
// `moreTemplates100` slice is the bulk-imported 100-list. Before the
// fix, module.Init unconditionally Add()-ed every 100-list entry,
// silently clobbering the 25 curated entries whose slugs overlapped.
// This test reproduces the merge that Module.Init performs and asserts:
//   - The curated entry wins for every overlapping slug.
//   - The final registry size is exactly len(builtin) + net-new.
func TestModuleInit_CuratedBuiltinsWinOverBulkList(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()
	builtinSize := r.Count()

	// Simulate the "skip-if-present" merge Module.Init does.
	added := 0
	skipped := 0
	for _, t2 := range GetMoreTemplates100() {
		if r.Get(t2.Slug) != nil {
			skipped++
			continue
		}
		r.Add(t2)
		added++
	}

	if added+skipped != len(moreTemplates100) {
		t.Fatalf("accounting mismatch: added=%d skipped=%d 100-list-size=%d",
			added, skipped, len(moreTemplates100))
	}

	if r.Count() != builtinSize+added {
		t.Fatalf("final registry size %d != builtin(%d) + added(%d)",
			r.Count(), builtinSize, added)
	}

	// Spot check: strapi overlaps, and the curated entry specifies
	// Version "5" while the bulk list specifies "4". Confirm the
	// richer (curated) version survived the merge.
	strapi := r.Get("strapi")
	if strapi == nil {
		t.Fatal("strapi template missing from merged registry")
	}
	if strapi.Version != "5" {
		t.Errorf("strapi.Version after merge = %q, want %q (the curated builtins_extra.go version must win over the bulk 100-list)",
			strapi.Version, "5")
	}
	if strapi.Icon == "" {
		t.Errorf("strapi.Icon empty after merge — curated Icon should have survived")
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
	for _, t2 := range GetMoreTemplates100() {
		if r.Get(t2.Slug) != nil {
			continue
		}
		r.Add(t2)
	}

	if r.Count() != expected {
		t.Errorf("final template count = %d, want %d — adjust the constant only if you intentionally added or removed a template",
			r.Count(), expected)
	}
}

func assertNoInternalDupes(t *testing.T, label string, ts []*Template) {
	t.Helper()
	seen := make(map[string]int)
	for _, tmpl := range ts {
		seen[tmpl.Slug]++
	}
	var dupes []string
	for slug, n := range seen {
		if n > 1 {
			dupes = append(dupes, slug)
		}
	}
	if len(dupes) > 0 {
		sort.Strings(dupes)
		t.Errorf("%s contains %d duplicate slug(s): %v", label, len(dupes), dupes)
	}
}
