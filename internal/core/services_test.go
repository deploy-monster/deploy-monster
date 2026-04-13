package core

import (
	"context"
	"io"
	"sort"
	"testing"
)

// --- Mock implementations ---

type mockDNSProvider struct {
	name string
}

func (m *mockDNSProvider) Name() string                                      { return m.name }
func (m *mockDNSProvider) CreateRecord(_ context.Context, _ DNSRecord) error { return nil }
func (m *mockDNSProvider) UpdateRecord(_ context.Context, _ DNSRecord) error { return nil }
func (m *mockDNSProvider) DeleteRecord(_ context.Context, _ DNSRecord) error { return nil }
func (m *mockDNSProvider) Verify(_ context.Context, _ string) (bool, error)  { return true, nil }

type mockBackupStorage struct {
	name string
}

func (m *mockBackupStorage) Name() string { return m.name }
func (m *mockBackupStorage) Upload(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return nil
}
func (m *mockBackupStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockBackupStorage) Delete(_ context.Context, _ string) error                { return nil }
func (m *mockBackupStorage) List(_ context.Context, _ string) ([]BackupEntry, error) { return nil, nil }

type mockVPSProvisioner struct {
	name string
}

func (m *mockVPSProvisioner) Name() string                                       { return m.name }
func (m *mockVPSProvisioner) ListRegions(_ context.Context) ([]VPSRegion, error) { return nil, nil }
func (m *mockVPSProvisioner) ListSizes(_ context.Context, _ string) ([]VPSSize, error) {
	return nil, nil
}
func (m *mockVPSProvisioner) Create(_ context.Context, _ VPSCreateOpts) (*VPSInstance, error) {
	return nil, nil
}
func (m *mockVPSProvisioner) Delete(_ context.Context, _ string) error { return nil }
func (m *mockVPSProvisioner) Status(_ context.Context, _ string) (string, error) {
	return "running", nil
}

type mockGitProvider struct {
	name string
}

func (m *mockGitProvider) Name() string                                             { return m.name }
func (m *mockGitProvider) ListRepos(_ context.Context, _, _ int) ([]GitRepo, error) { return nil, nil }
func (m *mockGitProvider) ListBranches(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (m *mockGitProvider) GetRepoInfo(_ context.Context, _ string) (*GitRepo, error) { return nil, nil }
func (m *mockGitProvider) CreateWebhook(_ context.Context, _, _, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *mockGitProvider) DeleteWebhook(_ context.Context, _, _ string) error { return nil }

// --- Tests ---

func TestNewServices(t *testing.T) {
	svc := NewServices()
	if svc == nil {
		t.Fatal("NewServices returned nil")
	}

	// All singleton services should be nil by default
	if svc.Container != nil {
		t.Error("Container should be nil on new Services")
	}
	if svc.SSH != nil {
		t.Error("SSH should be nil on new Services")
	}
	if svc.Secrets != nil {
		t.Error("Secrets should be nil on new Services")
	}
	if svc.Notifications != nil {
		t.Error("Notifications should be nil on new Services")
	}

	// Provider registries should be initialized but empty
	if providers := svc.DNSProviders(); len(providers) != 0 {
		t.Errorf("expected 0 DNS providers, got %d", len(providers))
	}
	if providers := svc.GitProviders(); len(providers) != 0 {
		t.Errorf("expected 0 Git providers, got %d", len(providers))
	}
}

func TestServices_RegisterDNSProvider(t *testing.T) {
	svc := NewServices()

	cf := &mockDNSProvider{name: "cloudflare"}
	r53 := &mockDNSProvider{name: "route53"}

	svc.RegisterDNSProvider("cloudflare", cf)
	svc.RegisterDNSProvider("route53", r53)

	// Lookup by name
	got := svc.DNSProvider("cloudflare")
	if got == nil {
		t.Fatal("DNSProvider('cloudflare') returned nil")
	}
	if got.Name() != "cloudflare" {
		t.Errorf("expected name 'cloudflare', got %q", got.Name())
	}

	got = svc.DNSProvider("route53")
	if got == nil {
		t.Fatal("DNSProvider('route53') returned nil")
	}
	if got.Name() != "route53" {
		t.Errorf("expected name 'route53', got %q", got.Name())
	}

	// Non-existent provider returns nil
	if svc.DNSProvider("nonexistent") != nil {
		t.Error("expected nil for unknown provider")
	}

	// List all
	names := svc.DNSProviders()
	sort.Strings(names)
	if len(names) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(names))
	}
	if names[0] != "cloudflare" || names[1] != "route53" {
		t.Errorf("expected [cloudflare route53], got %v", names)
	}
}

func TestServices_RegisterDNSProvider_Overwrite(t *testing.T) {
	svc := NewServices()

	old := &mockDNSProvider{name: "old"}
	new := &mockDNSProvider{name: "new"}

	svc.RegisterDNSProvider("cf", old)
	svc.RegisterDNSProvider("cf", new)

	got := svc.DNSProvider("cf")
	if got.Name() != "new" {
		t.Errorf("expected overwritten provider 'new', got %q", got.Name())
	}
	if len(svc.DNSProviders()) != 1 {
		t.Errorf("expected 1 provider after overwrite, got %d", len(svc.DNSProviders()))
	}
}

func TestServices_RegisterBackupStorage(t *testing.T) {
	svc := NewServices()

	local := &mockBackupStorage{name: "local"}
	s3 := &mockBackupStorage{name: "s3"}

	svc.RegisterBackupStorage("local", local)
	svc.RegisterBackupStorage("s3", s3)

	got := svc.BackupStorage("local")
	if got == nil {
		t.Fatal("BackupStorage('local') returned nil")
	}
	if got.Name() != "local" {
		t.Errorf("expected name 'local', got %q", got.Name())
	}

	got = svc.BackupStorage("s3")
	if got == nil {
		t.Fatal("BackupStorage('s3') returned nil")
	}
	if got.Name() != "s3" {
		t.Errorf("expected name 's3', got %q", got.Name())
	}

	// Non-existent returns nil
	if svc.BackupStorage("gcs") != nil {
		t.Error("expected nil for unknown storage")
	}
}

func TestServices_RegisterVPSProvisioner(t *testing.T) {
	svc := NewServices()

	hz := &mockVPSProvisioner{name: "hetzner"}
	do := &mockVPSProvisioner{name: "digitalocean"}

	svc.RegisterVPSProvisioner("hetzner", hz)
	svc.RegisterVPSProvisioner("digitalocean", do)

	got := svc.VPSProvisioner("hetzner")
	if got == nil {
		t.Fatal("VPSProvisioner('hetzner') returned nil")
	}
	if got.Name() != "hetzner" {
		t.Errorf("expected name 'hetzner', got %q", got.Name())
	}

	got = svc.VPSProvisioner("digitalocean")
	if got == nil {
		t.Fatal("VPSProvisioner('digitalocean') returned nil")
	}

	// Non-existent returns nil
	if svc.VPSProvisioner("vultr") != nil {
		t.Error("expected nil for unknown provisioner")
	}
}

func TestServices_RegisterGitProvider(t *testing.T) {
	svc := NewServices()

	gh := &mockGitProvider{name: "github"}
	gl := &mockGitProvider{name: "gitlab"}
	gt := &mockGitProvider{name: "gitea"}

	svc.RegisterGitProvider("github", gh)
	svc.RegisterGitProvider("gitlab", gl)
	svc.RegisterGitProvider("gitea", gt)

	// Lookup each
	tests := []struct {
		key      string
		wantName string
	}{
		{"github", "github"},
		{"gitlab", "gitlab"},
		{"gitea", "gitea"},
	}
	for _, tt := range tests {
		got := svc.GitProvider(tt.key)
		if got == nil {
			t.Fatalf("GitProvider(%q) returned nil", tt.key)
		}
		if got.Name() != tt.wantName {
			t.Errorf("GitProvider(%q).Name() = %q, want %q", tt.key, got.Name(), tt.wantName)
		}
	}

	// Non-existent returns nil
	if svc.GitProvider("bitbucket") != nil {
		t.Error("expected nil for unknown git provider")
	}

	// List all
	names := svc.GitProviders()
	sort.Strings(names)
	if len(names) != 3 {
		t.Fatalf("expected 3 git providers, got %d", len(names))
	}
	if names[0] != "gitea" || names[1] != "github" || names[2] != "gitlab" {
		t.Errorf("expected [gitea github gitlab], got %v", names)
	}
}
