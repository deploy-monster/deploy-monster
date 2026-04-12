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
