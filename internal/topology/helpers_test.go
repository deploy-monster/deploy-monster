package topology

import (
	"strings"
	"testing"
)

// TestRandomString verifies length and charset; the entropy-failure
// fallback path inside randomString is not exercised here because
// crypto/rand.Reader does not fail under normal test conditions.
func TestRandomString(t *testing.T) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"

	for _, length := range []int{0, 1, 12, 64} {
		t.Run("len/"+itoa(length), func(t *testing.T) {
			got := randomString(length)
			if len(got) != length {
				t.Fatalf("randomString(%d) returned %d chars: %q", length, len(got), got)
			}
			for i, r := range got {
				if !strings.ContainsRune(charset, r) {
					t.Fatalf("randomString result has out-of-charset rune %q at index %d (full=%q)", r, i, got)
				}
			}
		})
	}

	// Two successive calls should not produce identical output for
	// non-trivial lengths — sanity check that rand.Reader is wired up.
	a := randomString(32)
	b := randomString(32)
	if a == b {
		t.Fatalf("expected randomString(32) to differ between calls, got %q twice", a)
	}
}

func TestCompilerFindVolume(t *testing.T) {
	top := &Topology{
		Volumes: []Volume{
			{ID: "vol-1", Name: "data", SizeGB: 5},
			{ID: "vol-2", Name: "cache", SizeGB: 1},
		},
	}
	c := NewCompiler(top, "proj-1", "staging")

	if got := c.findVolume("vol-2"); got == nil || got.Name != "cache" {
		t.Fatalf("findVolume(vol-2) = %+v, want cache volume", got)
	}
	if got := c.findVolume("vol-missing"); got != nil {
		t.Fatalf("findVolume(missing) = %+v, want nil", got)
	}
}

// itoa is a tiny stdlib-free int-to-string used only in subtest names so
// the subtest output reads nicely in -v mode.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
