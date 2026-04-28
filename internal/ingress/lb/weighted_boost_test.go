package lb

import "testing"

func TestCanary_CanaryWeights(t *testing.T) {
	c := NewCanary([]string{"s1"}, []string{"c1"}, 10)
	w := c.CanaryWeights()
	if w == nil {
		t.Fatal("CanaryWeights() returned nil")
	}
}
