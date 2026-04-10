package billing

// Plan defines a billing plan with resource limits.
type Plan struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	PriceCents     int      `json:"price_cents"` // Monthly price in cents
	Currency       string   `json:"currency"`
	StripePriceID  string   `json:"stripe_price_id,omitempty"` // operator-configured Stripe price
	MaxApps        int      `json:"max_apps"`
	MaxContainers  int      `json:"max_containers"`
	MaxCPUCores    int      `json:"max_cpu_cores"`
	MaxRAMMB       int      `json:"max_ram_mb"`
	MaxDiskGB      int      `json:"max_disk_gb"`
	MaxBandwidthGB int      `json:"max_bandwidth_gb"`
	MaxDomains     int      `json:"max_domains"`
	MaxDatabases   int      `json:"max_databases"`
	MaxTeamMembers int      `json:"max_team_members"`
	MaxServers     int      `json:"max_servers"`
	BuildMinutes   int      `json:"build_minutes"`
	BackupGB       int      `json:"backup_gb"`
	Features       []string `json:"features"`
}

// BuiltinPlans are the default plans.
var BuiltinPlans = []Plan{
	{
		ID: "free", Name: "Free", Description: "For personal projects",
		PriceCents: 0, Currency: "USD",
		MaxApps: 3, MaxContainers: 5, MaxCPUCores: 2, MaxRAMMB: 2048,
		MaxDiskGB: 10, MaxBandwidthGB: 100, MaxDomains: 3, MaxDatabases: 2,
		MaxTeamMembers: 1, MaxServers: 1, BuildMinutes: 300, BackupGB: 5,
		Features: []string{"community_support"},
	},
	{
		ID: "pro", Name: "Pro", Description: "For professional developers",
		PriceCents: 1500, Currency: "USD",
		MaxApps: 25, MaxContainers: 50, MaxCPUCores: 8, MaxRAMMB: 16384,
		MaxDiskGB: 100, MaxBandwidthGB: 1000, MaxDomains: 50, MaxDatabases: 10,
		MaxTeamMembers: 5, MaxServers: 5, BuildMinutes: 3000, BackupGB: 50,
		Features: []string{"priority_support", "custom_domains", "auto_backups"},
	},
	{
		ID: "business", Name: "Business", Description: "For teams and companies",
		PriceCents: 4900, Currency: "USD",
		MaxApps: 100, MaxContainers: 200, MaxCPUCores: 32, MaxRAMMB: 65536,
		MaxDiskGB: 500, MaxBandwidthGB: 5000, MaxDomains: 200, MaxDatabases: 50,
		MaxTeamMembers: 25, MaxServers: 20, BuildMinutes: 10000, BackupGB: 200,
		Features: []string{"priority_support", "custom_domains", "auto_backups", "rbac", "audit_log", "sso"},
	},
	{
		ID: "enterprise", Name: "Enterprise", Description: "Custom limits, white-label, dedicated support",
		PriceCents: 0, Currency: "USD", // Custom pricing
		MaxApps: -1, MaxContainers: -1, MaxCPUCores: -1, MaxRAMMB: -1, // -1 = unlimited
		MaxDiskGB: -1, MaxBandwidthGB: -1, MaxDomains: -1, MaxDatabases: -1,
		MaxTeamMembers: -1, MaxServers: -1, BuildMinutes: -1, BackupGB: -1,
		Features: []string{"priority_support", "custom_domains", "auto_backups", "rbac", "audit_log", "sso", "white_label", "reseller", "whmcs", "dedicated_support"},
	},
}
