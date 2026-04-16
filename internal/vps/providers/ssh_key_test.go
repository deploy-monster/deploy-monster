package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Sprint 3 — SSH-key wiring regression tests. VPSCreateOpts.SSHKeyID
// had been plumbed through the interface but every provider was
// discarding it before the cloud-API call, so provisioned boxes booted
// with a generated root password and no way to log in short of the
// password (Linode) or the out-of-band user-data bootstrap flow (DO,
// Hetzner, Vultr). These tests pin both directions: when SSHKeyID is
// set, the provider's Create payload must carry the key under the
// field name the cloud API expects; when empty, the field must be
// absent so the existing no-key flow stays unchanged.

// captureCreateBody returns an httptest.Server that records the body of
// the first non-GET request it receives. Get() runs after Close() to
// avoid a race with the request goroutine.
type createBodyCapture struct {
	server *httptest.Server
	body   []byte
}

func newCreateBodyCapture(t *testing.T, respondWith string) *createBodyCapture {
	t.Helper()
	cap := &createBodyCapture{}
	cap.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			body, _ := io.ReadAll(r.Body)
			cap.body = body
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, respondWith)
	}))
	return cap
}

func (c *createBodyCapture) close() { c.server.Close() }

func (c *createBodyCapture) unmarshal(t *testing.T) map[string]any {
	t.Helper()
	if len(c.body) == 0 {
		t.Fatal("no POST body captured — Create did not reach the mock server")
	}
	var m map[string]any
	if err := json.Unmarshal(c.body, &m); err != nil {
		t.Fatalf("captured body not JSON: %v (body=%s)", err, c.body)
	}
	return m
}

func TestDigitalOcean_Create_WithSSHKey(t *testing.T) {
	cap := newCreateBodyCapture(t, `{"droplet":{"id":1,"name":"n","status":"new"}}`)
	defer cap.close()
	d := &DigitalOcean{token: "t", client: rewriteClient(cap.server.URL, doAPI)}
	if _, err := d.Create(context.Background(), core.VPSCreateOpts{
		Name: "n", Region: "nyc1", Size: "s-1vcpu-1gb", Image: "ubuntu-22-04", SSHKeyID: "do-key-123",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload := cap.unmarshal(t)
	keys, ok := payload["ssh_keys"].([]any)
	if !ok || len(keys) != 1 || keys[0] != "do-key-123" {
		t.Errorf("expected ssh_keys=[do-key-123] in payload, got %v", payload["ssh_keys"])
	}
}

func TestDigitalOcean_Create_WithoutSSHKey(t *testing.T) {
	cap := newCreateBodyCapture(t, `{"droplet":{"id":1,"name":"n","status":"new"}}`)
	defer cap.close()
	d := &DigitalOcean{token: "t", client: rewriteClient(cap.server.URL, doAPI)}
	if _, err := d.Create(context.Background(), core.VPSCreateOpts{
		Name: "n", Region: "nyc1", Size: "s-1vcpu-1gb", Image: "ubuntu-22-04",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload := cap.unmarshal(t)
	if _, present := payload["ssh_keys"]; present {
		t.Errorf("ssh_keys should be absent when SSHKeyID is empty, got %v", payload["ssh_keys"])
	}
}

func TestHetzner_Create_WithSSHKey(t *testing.T) {
	cap := newCreateBodyCapture(t, `{"server":{"id":1,"name":"n","status":"initializing","public_net":{"ipv4":{"ip":"1.2.3.4"}}}}`)
	defer cap.close()
	h := &Hetzner{token: "t", client: rewriteClient(cap.server.URL, hetznerAPI)}
	if _, err := h.Create(context.Background(), core.VPSCreateOpts{
		Name: "n", Region: "fsn1", Size: "cx11", Image: "ubuntu-22.04", SSHKeyID: "my-fingerprint",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload := cap.unmarshal(t)
	keys, ok := payload["ssh_keys"].([]any)
	if !ok || len(keys) != 1 || keys[0] != "my-fingerprint" {
		t.Errorf("expected ssh_keys=[my-fingerprint], got %v", payload["ssh_keys"])
	}
}

func TestHetzner_Create_WithoutSSHKey(t *testing.T) {
	cap := newCreateBodyCapture(t, `{"server":{"id":1,"name":"n","status":"initializing","public_net":{"ipv4":{"ip":"1.2.3.4"}}}}`)
	defer cap.close()
	h := &Hetzner{token: "t", client: rewriteClient(cap.server.URL, hetznerAPI)}
	if _, err := h.Create(context.Background(), core.VPSCreateOpts{
		Name: "n", Region: "fsn1", Size: "cx11", Image: "ubuntu-22.04",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload := cap.unmarshal(t)
	if _, present := payload["ssh_keys"]; present {
		t.Errorf("ssh_keys should be absent when SSHKeyID is empty")
	}
}

func TestVultr_Create_WithSSHKey(t *testing.T) {
	cap := newCreateBodyCapture(t, `{"instance":{"id":"v1","label":"n","main_ip":"1.2.3.4","status":"pending"}}`)
	defer cap.close()
	v := &Vultr{token: "t", client: rewriteClient(cap.server.URL, vultrAPI)}
	if _, err := v.Create(context.Background(), core.VPSCreateOpts{
		Name: "n", Region: "ewr", Size: "vc2-1c-1gb", Image: "387", SSHKeyID: "vultr-uuid",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload := cap.unmarshal(t)
	keys, ok := payload["sshkey_id"].([]any)
	if !ok || len(keys) != 1 || keys[0] != "vultr-uuid" {
		t.Errorf("expected sshkey_id=[vultr-uuid], got %v", payload["sshkey_id"])
	}
}

func TestVultr_Create_WithoutSSHKey(t *testing.T) {
	cap := newCreateBodyCapture(t, `{"instance":{"id":"v1","label":"n","main_ip":"1.2.3.4","status":"pending"}}`)
	defer cap.close()
	v := &Vultr{token: "t", client: rewriteClient(cap.server.URL, vultrAPI)}
	if _, err := v.Create(context.Background(), core.VPSCreateOpts{
		Name: "n", Region: "ewr", Size: "vc2-1c-1gb", Image: "387",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload := cap.unmarshal(t)
	if _, present := payload["sshkey_id"]; present {
		t.Errorf("sshkey_id should be absent when SSHKeyID is empty")
	}
}

func TestLinode_Create_WithSSHKey(t *testing.T) {
	cap := newCreateBodyCapture(t, `{"id":1,"label":"n","ipv4":["1.2.3.4"],"status":"provisioning"}`)
	defer cap.close()
	l := &Linode{token: "t", client: rewriteClient(cap.server.URL, linodeAPI)}
	// Linode's authorized_keys takes literal OpenSSH public keys.
	pubkey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... user@host"
	if _, err := l.Create(context.Background(), core.VPSCreateOpts{
		Name: "n", Region: "us-east", Size: "g6-nanode-1", Image: "linode/ubuntu22.04", SSHKeyID: pubkey,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload := cap.unmarshal(t)
	keys, ok := payload["authorized_keys"].([]any)
	if !ok || len(keys) != 1 || keys[0] != pubkey {
		t.Errorf("expected authorized_keys=[<pubkey>], got %v", payload["authorized_keys"])
	}
}

func TestLinode_Create_WithoutSSHKey(t *testing.T) {
	cap := newCreateBodyCapture(t, `{"id":1,"label":"n","ipv4":["1.2.3.4"],"status":"provisioning"}`)
	defer cap.close()
	l := &Linode{token: "t", client: rewriteClient(cap.server.URL, linodeAPI)}
	if _, err := l.Create(context.Background(), core.VPSCreateOpts{
		Name: "n", Region: "us-east", Size: "g6-nanode-1", Image: "linode/ubuntu22.04",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload := cap.unmarshal(t)
	if _, present := payload["authorized_keys"]; present {
		t.Errorf("authorized_keys should be absent when SSHKeyID is empty")
	}
}
