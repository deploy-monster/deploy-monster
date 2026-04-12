package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readPackageJSON reads the package.json and returns its content.
func readPackageJSON(dir string) map[string]any {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var pkg map[string]any
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	return pkg
}

// setupDir creates a temporary directory and writes the given filenames
// into it so detector tests have a clean workspace per case.
func setupDir(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return dir
}
