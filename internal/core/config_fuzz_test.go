package core

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// FuzzLoadConfig runs LoadConfig against random YAML blobs written to a
// temporary monster.yaml. The invariant is: LoadConfig must never panic —
// it may return a parse error, a validation error, or a valid *Config, but
// no input should trip a runtime panic inside yaml.Unmarshal or the
// applyDefaults / applyEnvOverrides / Validate pipeline.
func FuzzLoadConfig(f *testing.F) {
	// Seed corpus: well-formed, malformed, and edge-shape YAML.
	f.Add([]byte(`server:
  port: 8443
  secret_key: "0123456789abcdef0123456789abcdef"
database:
  driver: sqlite
`))
	f.Add([]byte(``))
	f.Add([]byte(`not yaml at all: [`))
	f.Add([]byte(`server: {port: -1, secret_key: "short"}`))
	f.Add([]byte(`---
foo: bar
baz:
  - 1
  - 2
`))
	f.Add([]byte(`server:
  port: 999999
  secret_key: ""
  log_level: not-a-level
`))

	dir := f.TempDir()
	path := filepath.Join(dir, "monster.yaml")

	f.Fuzz(func(t *testing.T, yamlBytes []byte) {
		if err := os.WriteFile(path, yamlBytes, 0o600); err != nil {
			t.Fatalf("write temp config: %v", err)
		}
		// Must not panic; errors are expected for most inputs.
		_, _ = LoadConfig(path)
	})
}

// FuzzConfigYAMLUnmarshal is a narrower fuzzer that targets only the YAML
// decoder path for Config. It avoids the file I/O overhead of
// FuzzLoadConfig so the corpus explores the parser faster, and it catches
// any struct-tag / field-type mismatch that would cause yaml.Unmarshal to
// panic on an unexpected shape.
func FuzzConfigYAMLUnmarshal(f *testing.F) {
	f.Add([]byte(`server:
  port: 8443
`))
	f.Add([]byte(`limits:
  max_concurrent_builds: 5
  max_concurrent_builds_per_tenant: 2
`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`null`))

	f.Fuzz(func(t *testing.T, yamlBytes []byte) {
		var cfg Config
		// Must not panic.
		_ = yaml.Unmarshal(yamlBytes, &cfg)
	})
}
