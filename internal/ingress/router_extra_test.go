package ingress

import "testing"

func TestRouteTable_All_Empty(t *testing.T) {
	rt := NewRouteTable()
	all := rt.All()

	if len(all) != 0 {
		t.Errorf("expected 0 routes, got %d", len(all))
	}
}

func TestRouteTable_All_ReturnsCopy(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "a.com", PathPrefix: "/", Backends: []string{"127.0.0.1:3000"}})
	rt.Upsert(&RouteEntry{Host: "b.com", PathPrefix: "/", Backends: []string{"127.0.0.1:3001"}})
	rt.Upsert(&RouteEntry{Host: "c.com", PathPrefix: "/api", Backends: []string{"127.0.0.1:3002"}})

	all := rt.All()
	if len(all) != 3 {
		t.Errorf("expected 3 routes, got %d", len(all))
	}

	// Verify it's a copy (modifying returned slice shouldn't affect route table)
	all[0] = nil
	allAgain := rt.All()
	if allAgain[0] == nil {
		t.Error("All() should return a copy, not a reference to internal slice")
	}
}

func TestRouteTable_All_ContainsCorrectEntries(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "x.com", PathPrefix: "/", ServiceName: "svc-x"})
	rt.Upsert(&RouteEntry{Host: "y.com", PathPrefix: "/", ServiceName: "svc-y"})

	all := rt.All()

	services := make(map[string]bool)
	for _, entry := range all {
		services[entry.ServiceName] = true
	}

	if !services["svc-x"] {
		t.Error("expected svc-x in All()")
	}
	if !services["svc-y"] {
		t.Error("expected svc-y in All()")
	}
}

func TestRouteTable_Count_Empty(t *testing.T) {
	rt := NewRouteTable()
	if rt.Count() != 0 {
		t.Errorf("expected count 0, got %d", rt.Count())
	}
}

func TestRouteTable_Count_AfterInserts(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "a.com", PathPrefix: "/"})
	rt.Upsert(&RouteEntry{Host: "b.com", PathPrefix: "/"})
	rt.Upsert(&RouteEntry{Host: "c.com", PathPrefix: "/"})

	if rt.Count() != 3 {
		t.Errorf("expected count 3, got %d", rt.Count())
	}
}

func TestRouteTable_Count_AfterRemove(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "a.com", PathPrefix: "/"})
	rt.Upsert(&RouteEntry{Host: "b.com", PathPrefix: "/"})

	rt.Remove("a.com", "/")
	if rt.Count() != 1 {
		t.Errorf("expected count 1 after remove, got %d", rt.Count())
	}
}

func TestRouteTable_Count_AfterUpsertReplace(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "a.com", PathPrefix: "/", Backends: []string{"old:80"}})
	rt.Upsert(&RouteEntry{Host: "a.com", PathPrefix: "/", Backends: []string{"new:80"}})

	// Upsert with same host+path replaces, count should remain 1
	if rt.Count() != 1 {
		t.Errorf("expected count 1 after upsert replace, got %d", rt.Count())
	}
}

func TestRouteTable_All_SortedByPriority(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "a.com", PathPrefix: "/", Priority: 10, ServiceName: "low"})
	rt.Upsert(&RouteEntry{Host: "b.com", PathPrefix: "/", Priority: 100, ServiceName: "high"})
	rt.Upsert(&RouteEntry{Host: "c.com", PathPrefix: "/", Priority: 50, ServiceName: "mid"})

	all := rt.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(all))
	}

	// Routes should be sorted by priority descending
	if all[0].Priority < all[1].Priority || all[1].Priority < all[2].Priority {
		t.Errorf("routes not sorted by priority: %d, %d, %d",
			all[0].Priority, all[1].Priority, all[2].Priority)
	}
}

func TestRouteTable_Remove_EmptyPathPrefix(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "a.com", PathPrefix: "/"})

	// Remove with empty path prefix should default to "/" and still remove the route
	rt.Remove("a.com", "")
	if rt.Count() != 0 {
		t.Errorf("expected count 0 after remove with empty prefix, got %d", rt.Count())
	}
}

func TestRouteTable_Upsert_EmptyPathPrefix(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "a.com", PathPrefix: ""})

	// Empty path prefix should default to "/"
	got := rt.Match("a.com", "/anything")
	if got == nil {
		t.Error("expected match after upsert with empty path prefix")
	}
	if got.PathPrefix != "/" {
		t.Errorf("expected path prefix '/', got %q", got.PathPrefix)
	}
}
