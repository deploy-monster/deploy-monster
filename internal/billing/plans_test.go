package billing

import "testing"

func TestBuiltinPlans(t *testing.T) {
	if len(BuiltinPlans) != 4 {
		t.Errorf("expected 4 plans, got %d", len(BuiltinPlans))
	}

	// Free plan
	free := BuiltinPlans[0]
	if free.ID != "free" {
		t.Errorf("first plan should be free, got %s", free.ID)
	}
	if free.PriceCents != 0 {
		t.Error("free plan should cost 0")
	}

	// Enterprise plan
	ent := BuiltinPlans[3]
	if ent.ID != "enterprise" {
		t.Errorf("last plan should be enterprise, got %s", ent.ID)
	}
	if ent.MaxApps != -1 {
		t.Error("enterprise should have unlimited apps (-1)")
	}
}

func TestPlanLimits(t *testing.T) {
	for _, p := range BuiltinPlans {
		if p.Name == "" {
			t.Error("plan name should not be empty")
		}
		if p.Currency == "" {
			t.Error("plan currency should not be empty")
		}
		if len(p.Features) == 0 {
			t.Errorf("plan %s should have features", p.ID)
		}
	}
}
