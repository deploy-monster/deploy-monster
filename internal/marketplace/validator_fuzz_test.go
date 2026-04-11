package marketplace

import (
	"testing"
)

// FuzzValidateTemplate runs ValidateTemplate against randomly-shaped
// templates. ValidateTemplate must never panic — it either returns nil
// (for well-formed templates) or a *ValidationError listing every issue
// found. The fuzzer checks the no-panic invariant plus the type-safety
// invariant: a non-nil error is always a *ValidationError.
func FuzzValidateTemplate(f *testing.F) {
	// Well-formed seed.
	f.Add(
		"postgres", "Postgres", "official Postgres image",
		"database", "me", "1.0",
		`version: "3.8"
services:
  db:
    image: postgres:16
    ports:
      - "5432:5432"
`,
		int32(128), int32(256), int32(1024),
	)

	// Missing metadata.
	f.Add("", "", "", "", "", "", ``, int32(0), int32(0), int32(0))

	// Malformed compose YAML.
	f.Add("bad", "bad", "bad", "db", "a", "1", `not: [valid: yaml`, int32(1), int32(1), int32(1))

	// Negative resource requirements.
	f.Add("neg", "neg", "neg", "misc", "a", "1",
		`services:
  web:
    image: nginx
`, int32(-1), int32(-1), int32(-1))

	// Empty services section.
	f.Add("nosvc", "nosvc", "nosvc", "misc", "a", "1", `version: "3"`, int32(0), int32(0), int32(0))

	// Undeclared volume reference.
	f.Add("vol", "vol", "vol", "db", "a", "1",
		`services:
  db:
    image: postgres
    volumes:
      - "pgdata:/var/lib/postgresql/data"
`, int32(0), int32(0), int32(0))

	f.Fuzz(func(t *testing.T,
		slug, name, description, category, author, version, composeYAML string,
		cpuMB, memoryMB, diskMB int32,
	) {
		tpl := &Template{
			Slug:        slug,
			Name:        name,
			Description: description,
			Category:    category,
			Author:      author,
			Version:     version,
			ComposeYAML: composeYAML,
			MinResources: ResourceReq{
				CPUMB:    int(cpuMB),
				MemoryMB: int(memoryMB),
				DiskMB:   int(diskMB),
			},
		}

		err := ValidateTemplate(tpl)
		if err == nil {
			return
		}
		// Non-nil errors must always be *ValidationError so callers can
		// rely on type-asserting to get the issue list.
		if _, ok := err.(*ValidationError); !ok {
			t.Errorf("ValidateTemplate returned non-*ValidationError: %T: %v", err, err)
		}
	})
}

// FuzzValidateTemplateNil asserts ValidateTemplate handles a nil template
// pointer gracefully — it must return an error, never panic.
func FuzzValidateTemplateNil(f *testing.F) {
	f.Add(int(0))
	f.Fuzz(func(t *testing.T, _ int) {
		if err := ValidateTemplate(nil); err == nil {
			t.Error("ValidateTemplate(nil) should return an error")
		}
	})
}
