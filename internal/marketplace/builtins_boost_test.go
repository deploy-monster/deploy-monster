package marketplace

import "testing"

func TestGetMoreTemplates100(t *testing.T) {
	templates := GetMoreTemplates100()
	if templates == nil {
		t.Fatal("GetMoreTemplates100() returned nil")
	}
	if len(templates) == 0 {
		t.Error("expected moreTemplates100 to be non-empty")
	}
	// Verify at least one well-known template
	found := false
	for _, tm := range templates {
		if tm.Slug == "grafana" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'grafana' template in moreTemplates100")
	}
}
