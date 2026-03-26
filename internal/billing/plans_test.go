package billing

import (
	"testing"
)

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

func TestPlanOrdering(t *testing.T) {
	// Plans should be ordered free < pro < business < enterprise by price
	// (enterprise is custom pricing at 0, so skip that for price ordering).
	tests := []struct {
		name      string
		planIndex int
		wantID    string
		wantPrice int
	}{
		{"free", 0, "free", 0},
		{"pro", 1, "pro", 1500},
		{"business", 2, "business", 4900},
		{"enterprise", 3, "enterprise", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := BuiltinPlans[tt.planIndex]
			if p.ID != tt.wantID {
				t.Errorf("plan[%d].ID = %q, want %q", tt.planIndex, p.ID, tt.wantID)
			}
			if p.PriceCents != tt.wantPrice {
				t.Errorf("plan[%d].PriceCents = %d, want %d", tt.planIndex, p.PriceCents, tt.wantPrice)
			}
		})
	}
}

func TestPlanFeatureLimitsScaleUp(t *testing.T) {
	// Non-enterprise plans should have increasing limits from free -> pro -> business.
	free := BuiltinPlans[0]
	pro := BuiltinPlans[1]
	business := BuiltinPlans[2]

	tests := []struct {
		field   string
		freeVal int
		proVal  int
		bizVal  int
	}{
		{"MaxApps", free.MaxApps, pro.MaxApps, business.MaxApps},
		{"MaxContainers", free.MaxContainers, pro.MaxContainers, business.MaxContainers},
		{"MaxCPUCores", free.MaxCPUCores, pro.MaxCPUCores, business.MaxCPUCores},
		{"MaxRAMMB", free.MaxRAMMB, pro.MaxRAMMB, business.MaxRAMMB},
		{"MaxDiskGB", free.MaxDiskGB, pro.MaxDiskGB, business.MaxDiskGB},
		{"MaxBandwidthGB", free.MaxBandwidthGB, pro.MaxBandwidthGB, business.MaxBandwidthGB},
		{"MaxDomains", free.MaxDomains, pro.MaxDomains, business.MaxDomains},
		{"MaxDatabases", free.MaxDatabases, pro.MaxDatabases, business.MaxDatabases},
		{"MaxTeamMembers", free.MaxTeamMembers, pro.MaxTeamMembers, business.MaxTeamMembers},
		{"MaxServers", free.MaxServers, pro.MaxServers, business.MaxServers},
		{"BuildMinutes", free.BuildMinutes, pro.BuildMinutes, business.BuildMinutes},
		{"BackupGB", free.BackupGB, pro.BackupGB, business.BackupGB},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if tt.freeVal >= tt.proVal {
				t.Errorf("free %s (%d) should be less than pro (%d)", tt.field, tt.freeVal, tt.proVal)
			}
			if tt.proVal >= tt.bizVal {
				t.Errorf("pro %s (%d) should be less than business (%d)", tt.field, tt.proVal, tt.bizVal)
			}
		})
	}
}

func TestEnterprisePlanUnlimited(t *testing.T) {
	ent := BuiltinPlans[3]

	// All resource limits should be -1 (unlimited) for enterprise.
	limits := map[string]int{
		"MaxApps":        ent.MaxApps,
		"MaxContainers":  ent.MaxContainers,
		"MaxCPUCores":    ent.MaxCPUCores,
		"MaxRAMMB":       ent.MaxRAMMB,
		"MaxDiskGB":      ent.MaxDiskGB,
		"MaxBandwidthGB": ent.MaxBandwidthGB,
		"MaxDomains":     ent.MaxDomains,
		"MaxDatabases":   ent.MaxDatabases,
		"MaxTeamMembers": ent.MaxTeamMembers,
		"MaxServers":     ent.MaxServers,
		"BuildMinutes":   ent.BuildMinutes,
		"BackupGB":       ent.BackupGB,
	}

	for name, val := range limits {
		if val != -1 {
			t.Errorf("enterprise %s = %d, want -1 (unlimited)", name, val)
		}
	}
}

func TestPlanFeatureInclusion(t *testing.T) {
	tests := []struct {
		planID      string
		planIndex   int
		wantFeature string
		wantPresent bool
	}{
		{"free", 0, "community_support", true},
		{"free", 0, "priority_support", false},
		{"free", 0, "sso", false},
		{"pro", 1, "priority_support", true},
		{"pro", 1, "custom_domains", true},
		{"pro", 1, "auto_backups", true},
		{"pro", 1, "sso", false},
		{"business", 2, "rbac", true},
		{"business", 2, "audit_log", true},
		{"business", 2, "sso", true},
		{"enterprise", 3, "white_label", true},
		{"enterprise", 3, "dedicated_support", true},
		{"enterprise", 3, "reseller", true},
		{"enterprise", 3, "whmcs", true},
	}

	for _, tt := range tests {
		t.Run(tt.planID+"/"+tt.wantFeature, func(t *testing.T) {
			plan := BuiltinPlans[tt.planIndex]
			found := false
			for _, f := range plan.Features {
				if f == tt.wantFeature {
					found = true
					break
				}
			}
			if found != tt.wantPresent {
				if tt.wantPresent {
					t.Errorf("plan %q should contain feature %q", tt.planID, tt.wantFeature)
				} else {
					t.Errorf("plan %q should NOT contain feature %q", tt.planID, tt.wantFeature)
				}
			}
		})
	}
}

func TestPlanCurrency(t *testing.T) {
	for _, p := range BuiltinPlans {
		t.Run(p.ID, func(t *testing.T) {
			if p.Currency != "USD" {
				t.Errorf("plan %q currency = %q, want USD", p.ID, p.Currency)
			}
		})
	}
}

func TestPlanDescriptionsNotEmpty(t *testing.T) {
	for _, p := range BuiltinPlans {
		t.Run(p.ID, func(t *testing.T) {
			if p.Description == "" {
				t.Errorf("plan %q should have a description", p.ID)
			}
		})
	}
}

func TestPlanIDsUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, p := range BuiltinPlans {
		if seen[p.ID] {
			t.Errorf("duplicate plan ID: %q", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestPlanNamesUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, p := range BuiltinPlans {
		if seen[p.Name] {
			t.Errorf("duplicate plan name: %q", p.Name)
		}
		seen[p.Name] = true
	}
}

func TestFreePlanResourceConstraints(t *testing.T) {
	free := BuiltinPlans[0]

	// Free plan should have strict, positive, but limited resources.
	tests := []struct {
		field string
		value int
		maxOK int // Maximum reasonable value for free plan
	}{
		{"MaxApps", free.MaxApps, 10},
		{"MaxContainers", free.MaxContainers, 10},
		{"MaxCPUCores", free.MaxCPUCores, 4},
		{"MaxTeamMembers", free.MaxTeamMembers, 2},
		{"MaxServers", free.MaxServers, 2},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if tt.value <= 0 {
				t.Errorf("free %s = %d, should be > 0", tt.field, tt.value)
			}
			if tt.value > tt.maxOK {
				t.Errorf("free %s = %d, seems too generous (max expected %d)", tt.field, tt.value, tt.maxOK)
			}
		})
	}
}
