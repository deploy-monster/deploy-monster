package secrets

import (
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// FuzzResolveAll exercises the ${SECRET:name} template parser in
// ResolveAll. The parser does index arithmetic on substrings, so the
// classes of bugs worth hunting for are:
//   - slice-out-of-bounds from mismatched ${SECRET: / } pairs
//   - infinite loops from a replacement that is itself a ${SECRET:...}
//     reference (recursive expansion)
//   - panics on unusual UTF-8 inputs at byte indices inside multi-byte
//     runes
//
// The seed corpus covers the documented edge cases (unclosed pattern,
// empty name, adjacent markers, nested braces) plus the exploit-ish
// inputs that matter for a template parser exposed to tenant-supplied
// env var and compose strings.
func FuzzResolveAll(f *testing.F) {
	// Seeds span the documented edge cases plus a handful of inputs
	// designed to provoke index-arithmetic bugs.
	f.Add("")
	f.Add("no secrets here")
	f.Add("${SECRET:db-user}")
	f.Add("user=${SECRET:db-user}&pass=${SECRET:db-pass}")
	f.Add("${SECRET:}")                        // empty name
	f.Add("${SECRET:name")                     // unclosed
	f.Add("}${SECRET:a}")                      // stray closing brace before marker
	f.Add("${SECRET:${SECRET:inner}}")         // nested — parser is non-recursive
	f.Add("${SECRET:a}${SECRET:b}${SECRET:c}") // multiple
	f.Add("prefix${SECRET:x}suffix")
	f.Add("${SECRET:\x00}")                  // NUL in name
	f.Add("${SECRET:\u00e4\u00f6\u00fc}")    // multi-byte UTF-8 in name
	f.Add(strings.Repeat("${SECRET:x}", 64)) // many markers
	f.Add(strings.Repeat("${SECRET:", 8) + strings.Repeat("}", 8))

	// Seed the store with two resolvable secrets so the happy-path
	// replacement branch is exercised, plus the ErrNotFound branch for
	// anything else.
	vault := NewVault("fuzz-master-key-for-testing-32b!")
	store := newMockSecretStore()

	enc1, err := vault.Encrypt("VALUE_A")
	if err != nil {
		f.Fatalf("seed encrypt a: %v", err)
	}
	enc2, err := vault.Encrypt("VALUE_B")
	if err != nil {
		f.Fatalf("seed encrypt b: %v", err)
	}
	store.secrets["global/a"] = &core.Secret{ID: "s-a", Scope: "global", Name: "a"}
	store.secrets["global/b"] = &core.Secret{ID: "s-b", Scope: "global", Name: "b"}
	store.versions["s-a"] = &core.SecretVersion{ID: "v-a", SecretID: "s-a", ValueEnc: enc1, Version: 1}
	store.versions["s-b"] = &core.SecretVersion{ID: "v-b", SecretID: "s-b", ValueEnc: enc2, Version: 1}

	m := &Module{store: store, vault: vault}

	f.Fuzz(func(t *testing.T, template string) {
		// Cap input size so the fuzzer doesn't spend cycles on inputs
		// larger than any real template would ever be. The 10 MB API
		// body limit already rejects anything larger at ingress.
		if len(template) > 64*1024 {
			return
		}

		// The contract is: no panic, no infinite loop. An error return
		// is fine — half the seeds intentionally fail to resolve.
		out, err := m.ResolveAll("global", template)
		if err != nil {
			return
		}

		// Replacement values never contain `${SECRET:`, so the number
		// of markers in the output must be <= the number in the input
		// (unclosed patterns remain, resolved ones are consumed; nothing
		// produces new markers). This pins the non-recursive-expansion
		// guarantee: a future refactor that enabled recursive
		// substitution would open a DoS via `${SECRET:x}` where x
		// resolves to `${SECRET:x}`.
		if strings.Count(out, "${SECRET:") > strings.Count(template, "${SECRET:") {
			t.Errorf("marker count grew after resolve: input=%q output=%q", template, out)
		}
	})
}
