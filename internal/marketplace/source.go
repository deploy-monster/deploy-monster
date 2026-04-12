package marketplace

import (
	"context"
	"fmt"
)

// TemplateSource is an upstream feed of templates. The marketplace polls
// these sources to keep its registry fresh without requiring a rebuild of
// the binary — production deployments can pull a curated list from a
// central registry, a git-backed HTTP endpoint, or any other JSON feed.
//
// Implementations must be safe for concurrent use and should return
// quickly on ctx cancellation.
type TemplateSource interface {
	// Name returns a human-readable identifier used in logs and errors.
	Name() string
	// Fetch returns the full set of templates this source knows about.
	// A source returning zero templates is treated as an error — callers
	// never replace a populated registry with an empty one by accident.
	Fetch(ctx context.Context) ([]*Template, error)
}

// UpdateResult summarizes the outcome of a registry update. Counts are
// reported so operators can see at a glance what changed without diffing
// template lists by hand.
type UpdateResult struct {
	Source   string
	Added    int
	Updated  int
	Rejected int // templates that failed validation and were skipped
	Total    int // total templates considered from the source
	Errors   []string
}

// Update refreshes the registry from a single source. The update is
// atomic from a reader's perspective: validated templates are staged
// into a scratch map and then swapped in under the write lock. If the
// source returns zero templates the registry is left untouched — we
// never silently empty ourselves because of a transient upstream hiccup.
//
// Templates that fail validation are skipped individually (their slugs
// appear in UpdateResult.Errors). Existing templates with the same slug
// are overwritten; templates not present in the source are left alone,
// so builtins continue to serve even when the upstream has a narrower
// view of the world.
func (r *TemplateRegistry) Update(ctx context.Context, source TemplateSource) (*UpdateResult, error) {
	if source == nil {
		return nil, fmt.Errorf("template source is nil")
	}

	fetched, err := source.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	if len(fetched) == 0 {
		return nil, fmt.Errorf("template source %q: returned no templates", source.Name())
	}

	result := &UpdateResult{
		Source: source.Name(),
		Total:  len(fetched),
	}

	// Validate everything before touching the registry so a single bad
	// template doesn't leave the registry half-updated.
	staged := make(map[string]*Template, len(fetched))
	for _, t := range fetched {
		if t == nil {
			result.Rejected++
			result.Errors = append(result.Errors, "nil template entry")
			continue
		}
		if err := ValidateTemplate(t); err != nil {
			result.Rejected++
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		staged[t.Slug] = t
	}

	if len(staged) == 0 {
		return result, fmt.Errorf("template source %q: all %d templates failed validation", source.Name(), len(fetched))
	}

	// Atomic swap: count added vs updated under the write lock so we
	// report on the state that actually gets committed.
	r.mu.Lock()
	for slug, t := range staged {
		if _, exists := r.templates[slug]; exists {
			result.Updated++
		} else {
			result.Added++
		}
		r.templates[slug] = t
	}
	r.mu.Unlock()

	return result, nil
}
