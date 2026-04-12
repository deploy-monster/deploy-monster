package compose

import "testing"

// FuzzParseCompose feeds random YAML to Parse and asserts it never panics.
// Errors are expected for invalid or structurally unsupported input.
func FuzzParseCompose(f *testing.F) {
	// Valid minimal compose
	f.Add([]byte("services:\n  web:\n    image: nginx\n"))
	// Empty
	f.Add([]byte(""))
	// Not YAML at all
	f.Add([]byte("{{{invalid yaml!!! :::"))
	// Valid YAML, no services
	f.Add([]byte("version: '3'\n"))
	// Complex compose
	f.Add([]byte(`services:
  api:
    image: node:22
    ports: ["3000:3000"]
    environment:
      - DB_URL=postgres://localhost/db
    depends_on:
      db:
        condition: service_healthy
  db:
    image: postgres:17
    volumes: ["data:/var/lib/postgresql/data"]
volumes:
  data:
networks:
  default:
    driver: bridge
`))
	// Deeply nested garbage
	f.Add([]byte("a:\n  b:\n    c:\n      d:\n        e: f\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic — errors are expected and fine
		_, _ = Parse(data)
	})
}
