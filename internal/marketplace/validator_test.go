package marketplace

import (
	"errors"
	"strings"
	"testing"
)

// helper: make a minimally-valid template, then let callers mutate it.
func validTemplate() *Template {
	return &Template{
		Slug:        "demo",
		Name:        "Demo",
		Description: "A demo template",
		Category:    "test",
		Author:      "tester",
		Version:     "1.0.0",
		MinResources: ResourceReq{
			MemoryMB: 128,
			DiskMB:   256,
			CPUMB:    100,
		},
		ComposeYAML: `services:
  web:
    image: nginx:latest
    ports:
      - "8080:80"
`,
	}
}

func TestValidateTemplate_NilReturnsError(t *testing.T) {
	if err := ValidateTemplate(nil); err == nil {
		t.Fatal("expected error for nil template, got nil")
	}
}

func TestValidateTemplate_HappyPath(t *testing.T) {
	if err := ValidateTemplate(validTemplate()); err != nil {
		t.Fatalf("expected valid template, got error: %v", err)
	}
}

func TestValidateTemplate_MissingMetadata(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Template)
		want   string
	}{
		{"slug", func(t *Template) { t.Slug = "" }, "slug is empty"},
		{"name", func(t *Template) { t.Name = "" }, "name is empty"},
		{"description", func(t *Template) { t.Description = "" }, "description is empty"},
		{"category", func(t *Template) { t.Category = "" }, "category is empty"},
		{"author", func(t *Template) { t.Author = "" }, "author is empty"},
		{"version", func(t *Template) { t.Version = "" }, "version is empty"},
		{"compose_yaml", func(t *Template) { t.ComposeYAML = "" }, "compose_yaml is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := validTemplate()
			tc.mutate(tmpl)
			err := ValidateTemplate(tmpl)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *ValidationError, got %T", err)
			}
			if !containsIssue(ve.Issues, tc.want) {
				t.Errorf("expected issue containing %q, got %v", tc.want, ve.Issues)
			}
		})
	}
}

func TestValidateTemplate_NegativeResources(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Template)
		want   string
	}{
		{"memory", func(t *Template) { t.MinResources.MemoryMB = -1 }, "memory_mb is negative"},
		{"disk", func(t *Template) { t.MinResources.DiskMB = -1 }, "disk_mb is negative"},
		{"cpu", func(t *Template) { t.MinResources.CPUMB = -1 }, "cpu_mb is negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := validTemplate()
			tc.mutate(tmpl)
			err := ValidateTemplate(tmpl)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var ve *ValidationError
			if !errors.As(err, &ve) || !containsIssue(ve.Issues, tc.want) {
				t.Errorf("expected issue containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestValidateTemplate_MalformedYAML(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = "services:\n  web:\n    image: nginx\n  - broken"
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "not valid YAML") {
		t.Errorf("expected YAML parse issue, got %v", err)
	}
}

func TestValidateTemplate_NoServices(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = "version: \"3\"\n"
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected error for missing services")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "no services") {
		t.Errorf("expected no-services issue, got %v", err)
	}
}

func TestValidateTemplate_ServiceMissingImage(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    ports:
      - "80:80"
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected error")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "missing image") {
		t.Errorf("expected missing-image issue, got %v", err)
	}
}

func TestValidateTemplate_BuildBlockSatisfiesImageRequirement(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    build: .
`
	if err := ValidateTemplate(tmpl); err != nil {
		t.Errorf("build block should satisfy image requirement, got %v", err)
	}
}

func TestValidateTemplate_UndeclaredNamedVolume(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    image: nginx:latest
    volumes:
      - data:/var/data
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected error for undeclared volume")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "not declared at the top level") {
		t.Errorf("expected undeclared-volume issue, got %v", err)
	}
}

func TestValidateTemplate_DeclaredNamedVolumeOK(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    image: nginx:latest
    volumes:
      - data:/var/data
volumes:
  data:
`
	if err := ValidateTemplate(tmpl); err != nil {
		t.Errorf("declared volume should pass, got %v", err)
	}
}

func TestValidateTemplate_BindMountPrefixesAllowed(t *testing.T) {
	prefixes := []string{
		"/host/data:/var/data",
		"./rel:/var/data",
		"~/home:/var/data",
		"${DATA_PATH}:/var/data",
		"$(pwd)/data:/var/data",
	}
	for _, vol := range prefixes {
		t.Run(vol, func(t *testing.T) {
			tmpl := validTemplate()
			tmpl.ComposeYAML = `services:
  web:
    image: nginx:latest
    volumes:
      - "` + vol + `"
`
			if err := ValidateTemplate(tmpl); err != nil {
				t.Errorf("bind mount %q should be accepted, got %v", vol, err)
			}
		})
	}
}

func TestValidateTemplate_BadPort(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    image: nginx:latest
    ports:
      - "abc:def"
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected error for bad port")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "does not look like host:container") {
		t.Errorf("expected bad-port issue, got %v", err)
	}
}

func TestLooksLikePort(t *testing.T) {
	good := []string{
		"80",
		"80:80",
		"127.0.0.1:8080:80",
		"8080:80/tcp",
		"8080-8090:80-90",
		"53:53/udp",
	}
	for _, p := range good {
		if !looksLikePort(p) {
			t.Errorf("expected %q to be accepted", p)
		}
	}
	bad := []string{
		"",
		"abc",
		"80:abc",
		"80:80/foo",
		"80 :80",
	}
	for _, p := range bad {
		if looksLikePort(p) {
			t.Errorf("expected %q to be rejected", p)
		}
	}
}

func TestValidationError_Message(t *testing.T) {
	e := &ValidationError{Slug: "demo", Issues: []string{"a", "b"}}
	msg := e.Error()
	if !strings.Contains(msg, "demo") || !strings.Contains(msg, "a") || !strings.Contains(msg, "b") {
		t.Errorf("error message missing pieces: %q", msg)
	}
	// empty-issues branch
	empty := &ValidationError{Slug: "x"}
	if !strings.Contains(empty.Error(), "x") {
		t.Errorf("empty-issues error missing slug: %q", empty.Error())
	}
}

func TestRegistry_ValidateAll(t *testing.T) {
	r := NewTemplateRegistry()
	good := validTemplate()
	good.Slug = "good"
	bad := validTemplate()
	bad.Slug = "bad"
	bad.Name = ""
	r.Add(good)
	r.Add(bad)

	results := r.ValidateAll()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// lexicographic order: bad before good
	if results[0].Slug != "bad" || results[1].Slug != "good" {
		t.Errorf("expected lex order [bad, good], got [%s, %s]", results[0].Slug, results[1].Slug)
	}
	if results[0].Err == nil {
		t.Error("expected bad template to have error")
	}
	if len(results[0].Issues) == 0 {
		t.Error("expected bad template to have issues listed")
	}
	if results[1].Err != nil {
		t.Errorf("expected good template to pass, got %v", results[1].Err)
	}
}

func TestRegistry_ValidateAll_Empty(t *testing.T) {
	r := NewTemplateRegistry()
	results := r.ValidateAll()
	if results == nil {
		t.Error("expected non-nil slice for empty registry")
	}
	if len(results) != 0 {
		t.Errorf("expected empty result, got %d", len(results))
	}
}

func TestRegistry_ValidateAll_AllBuiltinsPass(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()
	for _, tt := range GetMoreTemplates100() {
		r.Add(tt)
	}
	for _, res := range r.ValidateAll() {
		if res.Err != nil {
			t.Errorf("builtin %s failed validation: %v", res.Slug, res.Issues)
		}
	}
}

func containsIssue(issues []string, needle string) bool {
	for _, s := range issues {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
