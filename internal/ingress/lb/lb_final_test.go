package lb

import (
	"net/http"
	"testing"
)

// =============================================================================
// IPHash.Next — X-Forwarded-For header branch (line 83-84)
// =============================================================================

func TestIPHash_Next_WithXFF(t *testing.T) {
	ih := &IPHash{}
	backends := []string{"backend-a", "backend-b", "backend-c"}

	// Without XFF — uses RemoteAddr
	reqNoXFF := &http.Request{RemoteAddr: "10.0.0.1:12345"}
	resultA := ih.Next(backends, reqNoXFF)
	if resultA == "" {
		t.Error("expected non-empty result without XFF")
	}

	// With XFF — should use the forwarded IP instead
	reqWithXFF := &http.Request{
		RemoteAddr: "10.0.0.1:12345",
		Header:     http.Header{"X-Forwarded-For": []string{"192.168.1.100"}},
	}
	resultB := ih.Next(backends, reqWithXFF)
	if resultB == "" {
		t.Error("expected non-empty result with XFF")
	}

	// Different XFF should potentially yield a different backend (hash-based)
	reqWithXFF2 := &http.Request{
		RemoteAddr: "10.0.0.1:12345",
		Header:     http.Header{"X-Forwarded-For": []string{"10.20.30.40"}},
	}
	_ = ih.Next(backends, reqWithXFF2)
}

// =============================================================================
// Weighted.Next — pool empty fallback (line 134), weight <= 0 branch (line 126-127)
// =============================================================================

func TestWeighted_Next_EmptyWeightsPool(t *testing.T) {
	// Create Weighted with no matching weights — backends not in weight map
	// This forces weight <= 0 branch, which sets weight to 1
	w := NewWeighted(map[string]int{})
	backends := []string{"b1", "b2"}

	result := w.Next(backends, nil)
	if result != "b1" && result != "b2" {
		t.Errorf("expected one of the backends, got %q", result)
	}
}

func TestWeighted_Next_ZeroWeight(t *testing.T) {
	// Weight of 0 should be treated as 1
	w := NewWeighted(map[string]int{"b1": 0, "b2": 0})
	backends := []string{"b1", "b2"}

	result := w.Next(backends, nil)
	if result != "b1" && result != "b2" {
		t.Errorf("expected one of the backends, got %q", result)
	}
}

func TestWeighted_Next_NegativeWeight(t *testing.T) {
	// Negative weight should be treated as 1
	w := NewWeighted(map[string]int{"b1": -5, "b2": -10})
	backends := []string{"b1", "b2"}

	result := w.Next(backends, nil)
	if result != "b1" && result != "b2" {
		t.Errorf("expected one of the backends, got %q", result)
	}
}
