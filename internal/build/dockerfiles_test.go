package build

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Each Dockerfile template returns valid, non-empty content
// ---------------------------------------------------------------------------

func TestDockerfileTemplates_NonEmpty(t *testing.T) {
	allTypes := []ProjectType{
		TypeNodeJS, TypeNextJS, TypeVite, TypeNuxt,
		TypeGo, TypeRust, TypePython, TypePHP,
		TypeJava, TypeDotNet, TypeRuby, TypeStatic,
	}

	for _, pt := range allTypes {
		t.Run(string(pt), func(t *testing.T) {
			tmpl := GetDockerfileTemplate(pt)
			if tmpl == "" {
				t.Fatalf("template for %s is empty", pt)
			}
			if len(tmpl) < 20 {
				t.Errorf("template for %s seems too short: %d bytes", pt, len(tmpl))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Each template starts with FROM (valid Dockerfile)
// ---------------------------------------------------------------------------

func TestDockerfileTemplates_StartsWithFROM(t *testing.T) {
	allTypes := []ProjectType{
		TypeNodeJS, TypeNextJS, TypeVite, TypeNuxt,
		TypeGo, TypeRust, TypePython, TypePHP,
		TypeJava, TypeDotNet, TypeRuby, TypeStatic,
	}

	for _, pt := range allTypes {
		t.Run(string(pt), func(t *testing.T) {
			tmpl := GetDockerfileTemplate(pt)
			if !strings.HasPrefix(tmpl, "FROM ") {
				t.Errorf("template for %s should start with 'FROM', starts with: %q",
					pt, tmpl[:min(30, len(tmpl))])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Template for each project type contains expected base images
// ---------------------------------------------------------------------------

func TestDockerfileTemplates_BaseImages(t *testing.T) {
	tests := []struct {
		ptype      ProjectType
		baseImages []string // At least one of these should appear
	}{
		{TypeNodeJS, []string{"node:22-alpine"}},
		{TypeNextJS, []string{"node:22-alpine"}},
		{TypeVite, []string{"node:22-alpine", "nginx:alpine"}},
		{TypeNuxt, []string{"node:22-alpine"}},
		{TypeGo, []string{"golang:1.24-alpine", "alpine:3.20"}},
		{TypeRust, []string{"rust:1-alpine", "alpine:3.20"}},
		{TypePython, []string{"python:3.13-slim"}},
		{TypePHP, []string{"composer:2", "php:8.4-fpm-alpine"}},
		{TypeJava, []string{"eclipse-temurin:21-jdk-alpine", "eclipse-temurin:21-jre-alpine"}},
		{TypeDotNet, []string{"mcr.microsoft.com/dotnet/sdk:9.0-alpine", "mcr.microsoft.com/dotnet/aspnet:9.0-alpine"}},
		{TypeRuby, []string{"ruby:3.3-alpine"}},
		{TypeStatic, []string{"nginx:alpine"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.ptype), func(t *testing.T) {
			tmpl := GetDockerfileTemplate(tt.ptype)
			for _, img := range tt.baseImages {
				if !strings.Contains(tmpl, img) {
					t.Errorf("template for %s should contain base image %q", tt.ptype, img)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Multi-stage build templates have both builder and runtime stages
// ---------------------------------------------------------------------------

func TestDockerfileTemplates_MultiStage(t *testing.T) {
	multiStageTypes := []ProjectType{
		TypeNodeJS, TypeNextJS, TypeVite, TypeNuxt,
		TypeGo, TypeRust, TypePython, TypePHP,
		TypeJava, TypeDotNet, TypeRuby,
	}

	for _, pt := range multiStageTypes {
		t.Run(string(pt), func(t *testing.T) {
			tmpl := GetDockerfileTemplate(pt)
			fromCount := strings.Count(tmpl, "FROM ")
			if fromCount < 2 {
				t.Errorf("template for %s should be multi-stage (expected 2+ FROM), got %d",
					pt, fromCount)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Static template is single-stage
// ---------------------------------------------------------------------------

func TestDockerfileTemplate_Static_SingleStage(t *testing.T) {
	tmpl := GetDockerfileTemplate(TypeStatic)
	fromCount := strings.Count(tmpl, "FROM ")
	if fromCount != 1 {
		t.Errorf("static template should be single-stage, got %d FROM stages", fromCount)
	}
}

// ---------------------------------------------------------------------------
// Templates contain EXPOSE directive
// ---------------------------------------------------------------------------

func TestDockerfileTemplates_Expose(t *testing.T) {
	tests := []struct {
		ptype ProjectType
		port  string
	}{
		{TypeNodeJS, "3000"},
		{TypeNextJS, "3000"},
		{TypeVite, "80"},
		{TypeNuxt, "3000"},
		{TypeGo, "8080"},
		{TypeRust, "8080"},
		{TypePython, "8000"},
		{TypePHP, "80"},
		{TypeJava, "8080"},
		{TypeDotNet, "8080"},
		{TypeRuby, "3000"},
		{TypeStatic, "80"},
	}

	for _, tt := range tests {
		t.Run(string(tt.ptype), func(t *testing.T) {
			tmpl := GetDockerfileTemplate(tt.ptype)
			expose := "EXPOSE " + tt.port
			if !strings.Contains(tmpl, expose) {
				t.Errorf("template for %s should contain %q", tt.ptype, expose)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Templates contain WORKDIR
// ---------------------------------------------------------------------------

func TestDockerfileTemplates_Workdir(t *testing.T) {
	typesWithWorkdir := []ProjectType{
		TypeNodeJS, TypeNextJS, TypeVite, TypeNuxt,
		TypeGo, TypeRust, TypePython, TypePHP,
		TypeJava, TypeDotNet, TypeRuby,
	}

	for _, pt := range typesWithWorkdir {
		t.Run(string(pt), func(t *testing.T) {
			tmpl := GetDockerfileTemplate(pt)
			if !strings.Contains(tmpl, "WORKDIR ") {
				t.Errorf("template for %s should contain WORKDIR", pt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Templates contain CMD or ENTRYPOINT
// ---------------------------------------------------------------------------

func TestDockerfileTemplates_HasEntrypoint(t *testing.T) {
	allTypes := []ProjectType{
		TypeNodeJS, TypeNextJS, TypeNuxt,
		TypeGo, TypeRust, TypePython, TypePHP,
		TypeJava, TypeDotNet, TypeRuby,
	}

	for _, pt := range allTypes {
		t.Run(string(pt), func(t *testing.T) {
			tmpl := GetDockerfileTemplate(pt)
			hasCMD := strings.Contains(tmpl, "CMD ")
			hasEntrypoint := strings.Contains(tmpl, "ENTRYPOINT ")
			if !hasCMD && !hasEntrypoint {
				t.Errorf("template for %s should have CMD or ENTRYPOINT", pt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Templates contain COPY instruction
// ---------------------------------------------------------------------------

func TestDockerfileTemplates_HasCopy(t *testing.T) {
	allTypes := []ProjectType{
		TypeNodeJS, TypeNextJS, TypeVite, TypeNuxt,
		TypeGo, TypeRust, TypePython, TypePHP,
		TypeJava, TypeDotNet, TypeRuby, TypeStatic,
	}

	for _, pt := range allTypes {
		t.Run(string(pt), func(t *testing.T) {
			tmpl := GetDockerfileTemplate(pt)
			if !strings.Contains(tmpl, "COPY ") {
				t.Errorf("template for %s should contain COPY", pt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// No template for unknown/dockerfile types
// ---------------------------------------------------------------------------

func TestDockerfileTemplates_NoTemplateForSpecialTypes(t *testing.T) {
	special := []ProjectType{TypeUnknown, TypeDockerfile, TypeDockerCompose}
	for _, pt := range special {
		t.Run(string(pt), func(t *testing.T) {
			tmpl := GetDockerfileTemplate(pt)
			if tmpl != "" {
				t.Errorf("expected no template for %s, but got %d bytes", pt, len(tmpl))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Go template uses CGO_ENABLED=0 for static binary
// ---------------------------------------------------------------------------

func TestDockerfileTemplate_Go_StaticBinary(t *testing.T) {
	tmpl := GetDockerfileTemplate(TypeGo)
	if !strings.Contains(tmpl, "CGO_ENABLED=0") {
		t.Error("Go template should use CGO_ENABLED=0 for static binary")
	}
	if !strings.Contains(tmpl, `-ldflags="-s -w"`) {
		t.Error("Go template should use -ldflags for smaller binary")
	}
}

// ---------------------------------------------------------------------------
// Python template uses uvicorn
// ---------------------------------------------------------------------------

func TestDockerfileTemplate_Python_Uvicorn(t *testing.T) {
	tmpl := GetDockerfileTemplate(TypePython)
	if !strings.Contains(tmpl, "uvicorn") {
		t.Error("Python template should use uvicorn")
	}
}

// ---------------------------------------------------------------------------
// Java template handles both Maven and Gradle
// ---------------------------------------------------------------------------

func TestDockerfileTemplate_Java_BuildTools(t *testing.T) {
	tmpl := GetDockerfileTemplate(TypeJava)
	if !strings.Contains(tmpl, "mvnw") {
		t.Error("Java template should handle Maven wrapper")
	}
	if !strings.Contains(tmpl, "gradlew") {
		t.Error("Java template should handle Gradle wrapper")
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
