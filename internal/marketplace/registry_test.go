package marketplace

import "testing"

func TestTemplateRegistry_AddAndGet(t *testing.T) {
	r := NewTemplateRegistry()
	r.Add(&Template{Slug: "wordpress", Name: "WordPress", Category: "cms"})

	tmpl := r.Get("wordpress")
	if tmpl == nil {
		t.Fatal("expected template")
	}
	if tmpl.Name != "WordPress" {
		t.Errorf("expected WordPress, got %s", tmpl.Name)
	}

	if r.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent slug")
	}
}

func TestTemplateRegistry_Search(t *testing.T) {
	r := NewTemplateRegistry()
	r.Add(&Template{Slug: "wordpress", Name: "WordPress", Description: "CMS platform", Tags: []string{"blog", "cms"}})
	r.Add(&Template{Slug: "ghost", Name: "Ghost", Description: "Publishing platform", Tags: []string{"blog"}})
	r.Add(&Template{Slug: "redis", Name: "Redis", Description: "In-memory store", Tags: []string{"cache"}})

	results := r.Search("blog")
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'blog', got %d", len(results))
	}

	results = r.Search("redis")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'redis', got %d", len(results))
	}

	results = r.Search("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestTemplateRegistry_List(t *testing.T) {
	r := NewTemplateRegistry()
	r.Add(&Template{Slug: "wp", Name: "WP", Category: "cms"})
	r.Add(&Template{Slug: "ghost", Name: "Ghost", Category: "cms"})
	r.Add(&Template{Slug: "redis", Name: "Redis", Category: "database"})

	all := r.List("")
	if len(all) != 3 {
		t.Errorf("expected 3 total, got %d", len(all))
	}

	cms := r.List("cms")
	if len(cms) != 2 {
		t.Errorf("expected 2 cms, got %d", len(cms))
	}
}

func TestTemplateRegistry_Categories(t *testing.T) {
	r := NewTemplateRegistry()
	r.Add(&Template{Slug: "a", Category: "cms"})
	r.Add(&Template{Slug: "b", Category: "database"})
	r.Add(&Template{Slug: "c", Category: "cms"})

	cats := r.Categories()
	if len(cats) != 2 {
		t.Errorf("expected 2 categories, got %d", len(cats))
	}
}

func TestBuiltins_Loaded(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	if r.Count() < 10 {
		t.Errorf("expected at least 10 builtin templates, got %d", r.Count())
	}

	wp := r.Get("wordpress")
	if wp == nil {
		t.Fatal("expected wordpress template")
	}
	if wp.ComposeYAML == "" {
		t.Error("wordpress should have compose YAML")
	}
}
