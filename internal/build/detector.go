package build

// P3-11: 14 near-identical detector functions consolidated via fileExists
// helper and table-driven detection structure.

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

// detectorEntry describes how to detect a single project type.
type detectorEntry struct {
	ptype     ProjectType
	prereq    string   // optional; file that must exist before checking filenames
	filenames []string // files to try; first match wins
	isExt     bool     // if true, filenames are extensions to match against directory entries
}

// detectionTable is ordered by specificity — most specific match wins.
var detectionTable = []detectorEntry{
	{TypeDockerfile, "", []string{"Dockerfile"}, false},
	{TypeDockerCompose, "", []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}, false},
	{TypeNextJS, "package.json", []string{"next.config.js", "next.config.mjs", "next.config.ts"}, false},
	{TypeNuxt, "package.json", []string{"nuxt.config.js", "nuxt.config.ts"}, false},
	{TypeVite, "package.json", []string{"vite.config.js", "vite.config.ts", "vite.config.mjs"}, false},
	{TypeNodeJS, "", []string{"package.json"}, false},
	{TypeGo, "", []string{"go.mod"}, false},
	{TypeRust, "", []string{"Cargo.toml"}, false},
	{TypePython, "", []string{"requirements.txt", "pyproject.toml", "setup.py", "Pipfile"}, false},
	{TypeRuby, "", []string{"Gemfile"}, false},
	{TypePHP, "", []string{"composer.json"}, false},
	{TypeJava, "", []string{"pom.xml", "build.gradle", "build.gradle.kts"}, false},
	{TypeDotNet, "", []string{".csproj", ".sln", ".fsproj"}, true},
	{TypeStatic, "", []string{"index.html"}, false},
}

// Detect analyzes a directory to determine the project type.
func Detect(dir string) *DetectResult {
	for _, entry := range detectionTable {
		if ok, indicators := detect(entry, dir); ok {
			return &DetectResult{
				Type:       entry.ptype,
				Confidence: 90,
				Indicators: indicators,
			}
		}
	}
	return &DetectResult{Type: TypeUnknown, Confidence: 0}
}

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// exists is an alias for fileExists kept for backward compatibility with
// internal callers (e.g. builder.go) that reference the old name directly.
// Deprecated: prefer fileExists.
func exists(path string) bool { return fileExists(path) }

// detect checks if dir matches the given entry's criteria.
func detect(entry detectorEntry, dir string) (bool, []string) {
	if entry.prereq != "" && !fileExists(filepath.Join(dir, entry.prereq)) {
		return false, nil
	}

	if entry.isExt {
		return detectByExt(entry, dir)
	}

	for _, name := range entry.filenames {
		if fileExists(filepath.Join(dir, name)) {
			return true, []string{name}
		}
	}
	return false, nil
}

// detectByExt checks for files with matching extensions in dir.
func detectByExt(entry detectorEntry, dir string) (bool, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, nil
	}
	exts := make(map[string]bool)
	for _, e := range entry.filenames {
		exts[e] = true
	}
	for _, e := range entries {
		if exts[filepath.Ext(e.Name())] {
			return true, []string{e.Name()}
		}
	}
	return false, nil
}
