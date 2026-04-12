package lb

import (
	"net/http"
	"testing"
)

func TestRandom_Next_ReturnsValidBackend(t *testing.T) {
	rn := &Random{}
	backends := []string{"a:80", "b:80", "c:80"}

	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:12345"

	for range 50 {
		got := rn.Next(backends, r)

		found := false
		for _, b := range backends {
			if b == got {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Random.Next returned %q which is not in backends", got)
		}
	}
}

func TestRandom_Next_SingleBackend(t *testing.T) {
	rn := &Random{}
	backends := []string{"only:80"}

	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"

	for range 10 {
		got := rn.Next(backends, r)
		if got != "only:80" {
			t.Errorf("expected 'only:80', got %q", got)
		}
	}
}

func TestRandom_Next_DifferentRemoteAddrs(t *testing.T) {
	rn := &Random{}
	backends := []string{"a:80", "b:80", "c:80", "d:80"}

	results := make(map[string]bool)
	for i := range 100 {
		r, _ := http.NewRequest("GET", "/", nil)
		r.RemoteAddr = "10.0.0." + itoa(i%256) + ":1234"
		got := rn.Next(backends, r)
		results[got] = true
	}

	// With different remote addresses and counter mixing, we should hit multiple backends
	if len(results) < 2 {
		t.Error("Random should distribute across multiple backends with different addresses")
	}
}

func TestWeighted_SetWeight(t *testing.T) {
	w := NewWeighted(map[string]int{
		"a:80": 1,
		"b:80": 1,
	})
	backends := []string{"a:80", "b:80"}

	// Initially equal weights, should distribute roughly 50/50
	results := make(map[string]int)
	for range 100 {
		b := w.Next(backends, nil)
		results[b]++
	}

	if results["a:80"] != 50 || results["b:80"] != 50 {
		t.Logf("initial distribution: a=%d, b=%d (expected ~50/50)", results["a:80"], results["b:80"])
	}

	// Now heavily weight b
	w.SetWeight("b:80", 99)

	results = make(map[string]int)
	for range 100 {
		b := w.Next(backends, nil)
		results[b]++
	}

	// b should get the vast majority (99/100)
	if results["b:80"] < 80 {
		t.Errorf("after SetWeight(b, 99): expected b to get >80%%, got %d%%", results["b:80"])
	}
}

func TestWeighted_SetWeight_NewBackend(t *testing.T) {
	w := NewWeighted(map[string]int{
		"a:80": 5,
	})

	// Set weight for a backend not originally in the weights map
	w.SetWeight("b:80", 5)

	backends := []string{"a:80", "b:80"}
	results := make(map[string]int)
	for range 100 {
		b := w.Next(backends, nil)
		results[b]++
	}

	// Both should get roughly equal traffic now (5/5 each)
	if results["a:80"] != 50 || results["b:80"] != 50 {
		t.Logf("distribution after SetWeight new backend: a=%d, b=%d", results["a:80"], results["b:80"])
	}

	// Both should at least be present
	if results["a:80"] == 0 {
		t.Error("a:80 should receive some traffic")
	}
	if results["b:80"] == 0 {
		t.Error("b:80 should receive some traffic")
	}
}

func TestWeighted_SetWeight_ZeroWeight(t *testing.T) {
	w := NewWeighted(map[string]int{
		"a:80": 5,
		"b:80": 5,
	})

	// Set weight to 0 (should default to 1 in Next)
	w.SetWeight("b:80", 0)

	backends := []string{"a:80", "b:80"}
	results := make(map[string]int)
	for range 60 {
		b := w.Next(backends, nil)
		results[b]++
	}

	// a should get more traffic (5 vs 1 default weight for 0)
	if results["a:80"] < results["b:80"] {
		t.Errorf("a (weight=5) should get more traffic than b (weight=0->1): a=%d, b=%d",
			results["a:80"], results["b:80"])
	}
}

// itoa is a simple int to string helper to avoid importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 3)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
