package lb

import "testing"

// TestWeighted_DefaultWeightIsOne asserts that an unconfigured Weighted
// behaves like RoundRobin — each backend gets the same number of hits.
// This is the "drop-in" property: swapping Strategy from RoundRobin to
// Weighted without calling SetWeight must not change traffic shape.
func TestWeighted_DefaultWeightIsOne(t *testing.T) {
	w := NewWeighted()
	backends := []string{"a", "b", "c"}

	counts := map[string]int{}
	for range 300 {
		counts[w.Next(backends, nil)]++
	}
	for _, b := range backends {
		if counts[b] != 100 {
			t.Errorf("backend %s got %d picks, want 100 (perfectly balanced)", b, counts[b])
		}
	}
}

// TestWeighted_Distribution asserts that over a full cycle each backend
// is picked exactly its-weight times out of sum-of-weights. Smooth-WRR
// guarantees exactness, not just probabilistic balance.
func TestWeighted_Distribution(t *testing.T) {
	w := NewWeighted()
	w.SetWeight("a", 5)
	w.SetWeight("b", 3)
	w.SetWeight("c", 2)

	backends := []string{"a", "b", "c"}
	counts := map[string]int{}
	for range 100 { // 10 full cycles of total-weight-10
		counts[w.Next(backends, nil)]++
	}
	want := map[string]int{"a": 50, "b": 30, "c": 20}
	for b, wantN := range want {
		if counts[b] != wantN {
			t.Errorf("backend %s: got %d picks, want %d", b, counts[b], wantN)
		}
	}
}

// TestWeighted_Smoothness asserts that smooth-WRR interleaves backends
// rather than clustering them. With weights {a:5, b:1}, naive weighted-
// random would sometimes pick "a a a a a a b" (six a's in a row);
// smooth-WRR guarantees no more than `ceil(a_weight/b_weight)` a's
// between b picks — here, at most 5.
func TestWeighted_Smoothness(t *testing.T) {
	w := NewWeighted()
	w.SetWeight("a", 5)
	w.SetWeight("b", 1)

	backends := []string{"a", "b"}
	longestARun := 0
	currentARun := 0
	for range 60 { // 10 cycles of 6
		pick := w.Next(backends, nil)
		if pick == "a" {
			currentARun++
			if currentARun > longestARun {
				longestARun = currentARun
			}
		} else {
			currentARun = 0
		}
	}
	if longestARun > 5 {
		t.Errorf("smooth-WRR broken: saw a run of %d consecutive 'a' picks, want <=5 with weights {a:5, b:1}", longestARun)
	}
}

// TestWeighted_ZeroWeightSkipsBackend proves that setting weight to 0
// excludes a backend from the rotation without needing to remove it from
// the pool. This is the graceful-drain use case — app config says "stop
// sending new traffic here" without a pool reconfig.
func TestWeighted_ZeroWeightSkipsBackend(t *testing.T) {
	w := NewWeighted()
	w.SetWeight("a", 1)
	w.SetWeight("b", 0)
	w.SetWeight("c", 1)

	backends := []string{"a", "b", "c"}
	counts := map[string]int{}
	for range 40 {
		counts[w.Next(backends, nil)]++
	}
	if counts["b"] != 0 {
		t.Errorf("backend b with weight 0 got %d picks, want 0", counts["b"])
	}
	if counts["a"] == 0 || counts["c"] == 0 {
		t.Errorf("non-zero-weight backends got zero picks: a=%d c=%d", counts["a"], counts["c"])
	}
}

// TestWeighted_AllZeroWeightsFallsBackToFirst — degenerate safety. If
// every backend is weight 0, Next should still return something (the
// first backend) rather than returning an empty string, because the
// caller is about to make an HTTP request and can't do anything with
// "". The caller is expected to detect an all-zero pool and 503 rather
// than rely on this fallback.
func TestWeighted_AllZeroWeightsFallsBackToFirst(t *testing.T) {
	w := NewWeighted()
	w.SetWeight("a", 0)
	w.SetWeight("b", 0)

	got := w.Next([]string{"a", "b"}, nil)
	if got != "a" {
		t.Errorf("all-zero pool: got %q, want %q (first backend fallback)", got, "a")
	}
}

// TestWeighted_EmptyPool returns empty string so callers can 5xx.
func TestWeighted_EmptyPool(t *testing.T) {
	w := NewWeighted()
	got := w.Next(nil, nil)
	if got != "" {
		t.Errorf("empty pool: got %q, want \"\"", got)
	}
}

// TestWeighted_NegativeWeightClampedToZero — guards a common misuse: a
// config loader passes through a signed int and a -1 arrives. Silent
// clamping to 0 is better than a panic or, worse, a negative running
// current weight that permanently biases the distribution.
func TestWeighted_NegativeWeightClampedToZero(t *testing.T) {
	w := NewWeighted()
	w.SetWeight("a", -5)
	if got := w.Weight("a"); got != 0 {
		t.Errorf("negative weight not clamped: got %d, want 0", got)
	}
}

// TestWeighted_SetWeightsReplacesAtomically — SetWeights replaces the
// whole table rather than merging. Regression guard: an earlier draft
// of this code merged, which meant a backend removed from config kept
// its old weight until explicitly zeroed.
func TestWeighted_SetWeightsReplacesAtomically(t *testing.T) {
	w := NewWeighted()
	w.SetWeight("a", 10)
	w.SetWeight("b", 10)

	w.SetWeights(map[string]int{"c": 3})
	if got := w.Weight("a"); got != 1 {
		t.Errorf("after SetWeights, old backend a weight = %d, want 1 (default)", got)
	}
	if got := w.Weight("c"); got != 3 {
		t.Errorf("after SetWeights, new backend c weight = %d, want 3", got)
	}
}

// TestWeighted_WireThroughFactory — the string-based New() factory must
// return a Weighted when asked for "weighted".
func TestWeighted_WireThroughFactory(t *testing.T) {
	s := New("weighted")
	if _, ok := s.(*Weighted); !ok {
		t.Errorf("New(\"weighted\") returned %T, want *Weighted", s)
	}
}
