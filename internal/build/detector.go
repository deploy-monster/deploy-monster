package build

import (
	"os"
	"path/filepath"
)

// ProjectType identifies the detected project technology.
type ProjectType string

const (
	TypeDockerfile    ProjectType = "dockerfile"
	TypeDockerCompose ProjectType = "docker-compose"
	TypeNextJS        ProjectType = "nextjs"
	TypeVite          ProjectType = "vite"
	TypeNuxt          ProjectType = "nuxt"
	TypeNodeJS        ProjectType = "nodejs"
	TypeGo            ProjectType = "go"
	TypeRust          ProjectType = "rust"
	TypePython        ProjectType = "python"
	TypeRuby          ProjectType = "ruby"
	TypePHP           ProjectType = "php"
	TypeJava          ProjectType = "java"
	TypeDotNet        ProjectType = "dotnet"
	TypeStatic        ProjectType = "static"
	TypeUnknown       ProjectType = "unknown"
)

// DetectResult holds the result of project type detection.
type DetectResult struct {
	Type       ProjectType `json:"type"`
	Confidence int         `json:"confidence"` // 0-100
	Indicators []string    `json:"indicators"` // files that matched
}

// Detect analyzes a directory to determine the project type.
// Detection is ordered by specificity — most specific match wins.
func Detect(dir string) *DetectResult {
	// Check each type in order of specificity
	checks := []struct {
		ptype ProjectType
		fn    func(string) (bool, []string)
	}{
		{TypeDockerfile, hasDockerfile},
		{TypeDockerCompose, hasDockerCompose},
		{TypeNextJS, isNextJS},
		{TypeNuxt, isNuxt},
		{TypeVite, isVite},
		{TypeNodeJS, isNodeJS},
		{TypeGo, isGo},
		{TypeRust, isRust},
		{TypePython, isPython},
		{TypeRuby, isRuby},
		{TypePHP, isPHP},
		{TypeJava, isJava},
		{TypeDotNet, isDotNet},
		{TypeStatic, isStatic},
	}

	for _, check := range checks {
		if ok, indicators := check.fn(dir); ok {
			return &DetectResult{
				Type:       check.ptype,
				Confidence: 90,
				Indicators: indicators,
			}
		}
	}

	return &DetectResult{Type: TypeUnknown, Confidence: 0}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasDockerfile(dir string) (bool, []string) {
	if exists(filepath.Join(dir, "Dockerfile")) {
		return true, []string{"Dockerfile"}
	}
	return false, nil
}

func hasDockerCompose(dir string) (bool, []string) {
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		if exists(filepath.Join(dir, name)) {
			return true, []string{name}
		}
	}
	return false, nil
}

func isNextJS(dir string) (bool, []string) {
	if !exists(filepath.Join(dir, "package.json")) {
		return false, nil
	}
	for _, name := range []string{"next.config.js", "next.config.mjs", "next.config.ts"} {
		if exists(filepath.Join(dir, name)) {
			return true, []string{"package.json", name}
		}
	}
	return false, nil
}

func isNuxt(dir string) (bool, []string) {
	if !exists(filepath.Join(dir, "package.json")) {
		return false, nil
	}
	for _, name := range []string{"nuxt.config.js", "nuxt.config.ts"} {
		if exists(filepath.Join(dir, name)) {
			return true, []string{"package.json", name}
		}
	}
	return false, nil
}

func isVite(dir string) (bool, []string) {
	if !exists(filepath.Join(dir, "package.json")) {
		return false, nil
	}
	for _, name := range []string{"vite.config.js", "vite.config.ts", "vite.config.mjs"} {
		if exists(filepath.Join(dir, name)) {
			return true, []string{"package.json", name}
		}
	}
	return false, nil
}

func isNodeJS(dir string) (bool, []string) {
	if exists(filepath.Join(dir, "package.json")) {
		return true, []string{"package.json"}
	}
	return false, nil
}

func isGo(dir string) (bool, []string) {
	if exists(filepath.Join(dir, "go.mod")) {
		return true, []string{"go.mod"}
	}
	return false, nil
}

func isRust(dir string) (bool, []string) {
	if exists(filepath.Join(dir, "Cargo.toml")) {
		return true, []string{"Cargo.toml"}
	}
	return false, nil
}

func isPython(dir string) (bool, []string) {
	for _, name := range []string{"requirements.txt", "pyproject.toml", "setup.py", "Pipfile"} {
		if exists(filepath.Join(dir, name)) {
			return true, []string{name}
		}
	}
	return false, nil
}

func isRuby(dir string) (bool, []string) {
	if exists(filepath.Join(dir, "Gemfile")) {
		return true, []string{"Gemfile"}
	}
	return false, nil
}

func isPHP(dir string) (bool, []string) {
	if exists(filepath.Join(dir, "composer.json")) {
		return true, []string{"composer.json"}
	}
	return false, nil
}

func isJava(dir string) (bool, []string) {
	for _, name := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
		if exists(filepath.Join(dir, name)) {
			return true, []string{name}
		}
	}
	return false, nil
}

func isDotNet(dir string) (bool, []string) {
	// Check for .csproj or .sln files
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, nil
	}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext == ".csproj" || ext == ".sln" || ext == ".fsproj" {
			return true, []string{e.Name()}
		}
	}
	return false, nil
}

func isStatic(dir string) (bool, []string) {
	if exists(filepath.Join(dir, "index.html")) {
		return true, []string{"index.html"}
	}
	return false, nil
}
