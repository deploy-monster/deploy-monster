package handlers

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestContainsStringAllBranches(t *testing.T) {
	if !containsString([]string{"a", "b"}, "a") { t.Error("should find a") }
	if containsString([]string{"a", "b"}, "c") { t.Error("should not find c") }
	if containsString(nil, "x") { t.Error("nil should not contain x") }
	if containsString([]string{}, "x") { t.Error("empty should not contain x") }
}

func TestGitProviderDisplayNameBranches(t *testing.T) {
	for in, want := range map[string]string{
		"github": "GitHub", "gitlab": "GitLab", "gitea": "Gitea",
		"bitbucket": "Bitbucket", "unknown": "unknown", "": "",
	} {
		if got := gitProviderDisplayName(in); got != want {
			t.Errorf("gitProviderDisplayName(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestExtractAppActionAllCases(t *testing.T) {
	id, act, cid := extractAppAction(core.Event{Data: core.AppEventData{AppID: "a1"}})
	if id != "a1" || act != "" || cid != "" { t.Errorf("AppEventData: (%q,%q,%q)", id, act, cid) }

	id, act, cid = extractAppAction(core.Event{Data: map[string]string{"id": "a2", "action": "restart", "container_id": "c1"}})
	if id != "a2" || act != "restart" || cid != "c1" { t.Errorf("map: (%q,%q,%q)", id, act, cid) }

	id, act, cid = extractAppAction(core.Event{Data: "raw"})
	if id != "" || act != "" || cid != "" { t.Errorf("unknown: (%q,%q,%q)", id, act, cid) }

	id, act, cid = extractAppAction(core.Event{})
	if id != "" || act != "" || cid != "" { t.Errorf("nil: (%q,%q,%q)", id, act, cid) }
}

func TestIsStrictBackupKeyAllCases(t *testing.T) {
	for _, k := range []string{"t/b1", "a/b.c"} {
		if !isStrictBackupKey(k) { t.Errorf("should be valid: %q", k) }
	}
	for _, k := range []string{"", "../etc", "a//b", "a/b/", "a/./b", "a/../b", "a/b@c", "a%2Fb"} {
		if isStrictBackupKey(k) { t.Errorf("should be invalid: %q", k) }
	}
}

func TestActiveDeployFreezeAllPaths(t *testing.T) {
	if activeDeployFreeze(nil, "t1") { t.Error("nil bolt should be false") }
	if activeDeployFreeze(newMockBoltStore(), "") { t.Error("empty tenant should be false") }
	if activeDeployFreeze(newMockBoltStore(), "t1") { t.Error("no data should be false") }

	b := newMockBoltStore()
	now := time.Now()
	b.Set("deploy_freeze", "t1", freezeWindowList{Windows: []FreezeWindow{{
		ID: "f1", StartsAt: now.Add(-1 * time.Hour), EndsAt: now.Add(1 * time.Hour), Active: true,
	}}}, 0)
	if !activeDeployFreeze(b, "t1") { t.Error("active freeze should be true") }

	b2 := newMockBoltStore()
	b2.Set("deploy_freeze", "t2", freezeWindowList{Windows: []FreezeWindow{{
		ID: "f2", StartsAt: now.Add(-48 * time.Hour), EndsAt: now.Add(-24 * time.Hour), Active: true,
	}}}, 0)
	if activeDeployFreeze(b2, "t2") { t.Error("expired freeze should be false") }

	b3 := newMockBoltStore()
	b3.Set("deploy_freeze", "t3", freezeWindowList{Windows: []FreezeWindow{{
		ID: "f3", StartsAt: now.Add(-1 * time.Hour), EndsAt: now.Add(1 * time.Hour), Active: false,
	}}}, 0)
	if activeDeployFreeze(b3, "t3") { t.Error("inactive freeze should be false") }

	b4 := newMockBoltStore()
	b4.Set("deploy_freeze", "t4", freezeWindowList{Windows: []FreezeWindow{{
		ID: "f4", StartsAt: now.Add(24 * time.Hour), EndsAt: now.Add(48 * time.Hour), Active: true,
	}}}, 0)
	if activeDeployFreeze(b4, "t4") { t.Error("future freeze should be false") }
}

func TestContainsAnyAll(t *testing.T) {
	if containsAny("hello", "xyz") { t.Error("should not match") }
	if !containsAny("hello", "ae") { t.Error("should match 'e'") }
	if !containsAny("test.com", ".:") { t.Error("should match '.'") }
	if containsAny("", ".:") { t.Error("empty should not match") }
}

func TestImageRefHasRegistryAllCases(t *testing.T) {
	cases := []struct{ ref string; want bool }{
		{"alpine", false}, {"library/nginx", false}, {"localhost:5000/img", true},
		{"reg.example.com/img", true}, {"docker.io/img", true}, {"", false},
	}
	for _, c := range cases {
		if got := imageRefHasRegistry(c.ref); got != c.want {
			t.Errorf("imageRefHasRegistry(%q)=%v, want %v", c.ref, got, c.want)
		}
	}
}

func TestImageNamePartAllCases(t *testing.T) {
	if got := imageNamePart("My App", ""); got != "my-app" { t.Errorf("got %q", got) }
	if got := imageNamePart("", "fallback"); got != "fallback" { t.Errorf("got %q", got) }
	if got := imageNamePart("", ""); !strings.HasPrefix(got, "app-") { t.Errorf("got %q", got) }
}

func TestBuildImageTagForRegistryEdgeCases(t *testing.T) {
	if v := buildImageTagForRegistry("", &core.Application{Name: "a", ID: "1"}, "abc"); v != "" { t.Errorf("expected empty, got %q", v) }
	if v := buildImageTagForRegistry("r.io", nil, "abc"); v != "" { t.Errorf("expected empty, got %q", v) }
	if v := buildImageTagForRegistry("r.io/r", &core.Application{Name: "MyApp", ID: "id"}, ""); v == "" { t.Error("empty sha should still produce tag") }
}

func TestTenantBackupPrefixAll(t *testing.T) {
	if p := tenantBackupPrefix("tenant1"); p != "tenant1/" { t.Errorf("got %q", p) }
	if p := tenantBackupPrefix("/t/"); p != "t/" { t.Errorf("got %q", p) }
}

type noMutateBolt struct{ core.BoltStorer }

func TestMutateBoltValueFallbackGetError(t *testing.T) {
	inner := newMockBoltStore()
	inner.errGet = fmt.Errorf("get error")
	var list eventWebhookList
	err := mutateBoltValue(&noMutateBolt{inner}, "b", "k", &list, 0, func(_ bool) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "get error") {
		t.Errorf("expected 'get error', got %v", err)
	}
}

func TestMutateBoltValueFallbackMutateError(t *testing.T) {
	inner := newMockBoltStore()
	var list eventWebhookList
	err := mutateBoltValue(&noMutateBolt{inner}, "b", "k", &list, 0, func(_ bool) error {
		return fmt.Errorf("custom err")
	})
	if err == nil || err.Error() != "custom err" { t.Errorf("expected 'custom err', got %v", err) }
}

func TestMutateBoltValueWithMutate(t *testing.T) {
	bolt := newMockBoltStore()
	var list eventWebhookList
	err := mutateBoltValue(bolt, "wh", "k1", &list, 0, func(exists bool) error {
		if exists { t.Error("first call should have exists=false") }
		list.Webhooks = append(list.Webhooks, EventWebhookConfig{ID: "wh1"})
		return nil
	})
	if err != nil { t.Fatalf("first mutate: %v", err) }

	err = mutateBoltValue(bolt, "wh", "k1", &list, 0, func(exists bool) error {
		if !exists { t.Error("second call should have exists=true") }
		list.Webhooks = append(list.Webhooks, EventWebhookConfig{ID: "wh2"})
		return nil
	})
	if err != nil { t.Fatalf("second mutate: %v", err) }
	if len(list.Webhooks) != 2 { t.Fatalf("expected 2, got %d", len(list.Webhooks)) }
}

func TestCertMatchesDomainAllCases(t *testing.T) {
	cert := &x509.Certificate{DNSNames: []string{"example.com"}}
	if !certMatchesDomain(cert, "example.com") { t.Error("direct") }

	cert = &x509.Certificate{DNSNames: []string{"*.example.com"}}
	if !certMatchesDomain(cert, "sub.example.com") { t.Error("wildcard") }
	if certMatchesDomain(cert, "example.com") { t.Error("apex should not match wildcard") }

	cert = &x509.Certificate{}
	cert.Subject.CommonName = "myapp.com"
	if !certMatchesDomain(cert, "myapp.com") { t.Error("CN match") }
	if certMatchesDomain(cert, "other.com") { t.Error("CN mismatch") }

	cert.Subject.CommonName = "*.example.com"
	if !certMatchesDomain(cert, "sub.example.com") { t.Error("wildcard CN") }
	if certMatchesDomain(cert, "example.com") { t.Error("wildcard CN apex") }

	cert = &x509.Certificate{}
	if certMatchesDomain(cert, "x") { t.Error("empty") }
}

func TestAppVisibleToTenantAllPaths(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1"})
	h := NewVolumeHandler(nil, s, nil)

	if !h.appVisibleToTenant(context.Background(), "a1", "t1", map[string]string{"monster.tenant": "t1"}) { t.Error("label match") }
	if h.appVisibleToTenant(context.Background(), "a1", "t2", map[string]string{"monster.tenant": "t1"}) { t.Error("label mismatch") }
	if !h.appVisibleToTenant(context.Background(), "a1", "t1", nil) { t.Error("store match") }
	if h.appVisibleToTenant(context.Background(), "a1", "t2", nil) { t.Error("store mismatch") }

	h2 := NewVolumeHandler(nil, nil, nil)
	if h2.appVisibleToTenant(context.Background(), "a1", "t1", nil) { t.Error("nil store") }
}

func TestRequireTenantDomainPaths(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "app1", TenantID: "t1"})
	s.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "ex.com"})
	h := NewDomainVerifyHandler(s, newMockBoltStore())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	_, ok := h.requireTenantDomain(rr, req, "nonexistent", "t1")
	if ok { t.Error("should fail for missing domain") }

	s.addApp(&core.Application{ID: "app2", TenantID: "t2"})
	s.addDomain(&core.Domain{ID: "d2", AppID: "app2", FQDN: "other.com"})
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	_, ok = h.requireTenantDomain(rr, req, "d2", "t1")
	if ok { t.Error("should fail for wrong tenant") }
}

func TestRequireTenantCertDomainPaths(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "app1", TenantID: "t1"})
	s.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "ex.com"})
	h := NewCertificateHandler(s, newMockBoltStore())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	_, ok := h.requireTenantCertificateDomain(rr, req, "nonexistent", "t1")
	if ok { t.Error("should fail for missing domain") }

	_, ok = h.requireTenantCertificateDomain(rr, req, "ex.com", "t1")
	if !ok { t.Error("should find by FQDN") }

	s.addApp(&core.Application{ID: "app2", TenantID: "t2"})
	s.addDomain(&core.Domain{ID: "d2", AppID: "app2", FQDN: "other.com"})
	_, ok = h.requireTenantCertificateDomain(rr, req, "d2", "t1")
	if ok { t.Error("should fail for wrong tenant") }
}

func TestCheckAndIncrementRateLimit(t *testing.T) {
	h := &AuthHandler{}
	locked, _ := h.checkPerAccountRateLimit("u@e.com")
	if locked { t.Error("nil bolt should not lock") }

	bolt := newMockBoltStore()
	h.bolt = bolt

	// Expired lock
	bolt.Set("account_rl", "u@e.com", accountRateLimitEntry{LockedUntil: time.Now().Add(-1 * time.Minute).Unix()}, 0)
	locked, _ = h.checkPerAccountRateLimit("u@e.com")
	if locked { t.Error("expired lock should not lock") }

	// Bolt error on check
	bolt.errGet = fmt.Errorf("fail")
	locked, _ = h.checkPerAccountRateLimit("e@e.com")
	if locked { t.Error("bolt error should not lock") }
	bolt.errGet = nil

	// Increment nil bolt
	(&AuthHandler{}).incrementPerAccountRateLimit(context.Background(), "u@e.com")

	// Already locked - should not increment
	bolt.Set("account_rl", "lk@e.com", accountRateLimitEntry{FailedCount: 5, LockedUntil: time.Now().Add(15 * time.Minute).Unix()}, 0)
	h.incrementPerAccountRateLimit(context.Background(), "lk@e.com")
	var entry accountRateLimitEntry
	bolt.Get("account_rl", "lk@e.com", &entry)
	if entry.FailedCount != 5 { t.Errorf("should stay at 5, got %d", entry.FailedCount) }

	// Bolt get error on increment
	bolt2 := newMockBoltStore()
	bolt2.errGet = fmt.Errorf("fail")
	h.bolt = bolt2
	h.incrementPerAccountRateLimit(context.Background(), "u@e.com")
}

func TestLoginRateLimitEdge(t *testing.T) {
	h := &AuthHandler{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	if r := h.loginRateLimitCheck(rr, req, "u@e.com"); r != 0 {
		t.Errorf("expected 0, got %d", r)
	}

	bolt := newMockBoltStore()
	lu := time.Now().Add(15 * time.Minute).Unix()
	bolt.Set("account_rl", "l@e.com", accountRateLimitEntry{LockedUntil: lu}, 0)
	h.bolt = bolt
	rr = httptest.NewRecorder()
	r := h.loginRateLimitCheck(rr, req, "l@e.com")
	if r != lu { t.Errorf("expected %d, got %d", lu, r) }
	if rr.Code != http.StatusTooManyRequests { t.Errorf("expected 429, got %d", rr.Code) }
}

func TestRevokeAccessTokenEdgeCases(t *testing.T) {
	(&AuthHandler{}).revokeAccessTokenFromRequest(httptest.NewRequest("GET", "/", nil))
	h := &AuthHandler{bolt: newMockBoltStore(), authMod: testAuthModule(nil)}
	h.revokeAccessTokenFromRequest(httptest.NewRequest("GET", "/", nil))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	h.revokeAccessTokenFromRequest(req)
}

func TestTrackAndEnforceSessionEdge(t *testing.T) {
	(&AuthHandler{}).trackSession(httptest.NewRequest("GET", "/", nil), "u1", "tok")
	(&AuthHandler{}).enforceSessionLimit("u1")
	(&AuthHandler{bolt: newMockBoltStore()}).enforceSessionLimit("")
}

func TestStripPortAllVariants(t *testing.T) {
	for in, want := range map[string]string{
		"1.2.3.4:8080": "1.2.3.4", "192.168.1.1:80": "192.168.1.1",
		"hostname:443": "hostname", "[::1]:8080": "[::1]:8080", "no-port": "no-port",
	} {
		if got := stripPort(in); got != want { t.Errorf("stripPort(%q)=%q, want %q", in, got, want) }
	}
}

func TestSubscribeRestartHistoryNilBoltEvents(t *testing.T) {
	SubscribeRestartHistory(nil, nil)
	SubscribeRestartHistory(core.NewEventBus(slog.Default()), nil)
	SubscribeRestartHistory(nil, newMockBoltStore())
}

func TestGetConnectionNilBolt(t *testing.T) {
	h := &GitSourceHandler{}
	_, err := h.getConnection("t1", "github")
	if !errors.Is(err, core.ErrNotFound) { t.Errorf("expected ErrNotFound, got %v", err) }
}

func TestListConnectionsNilBolt(t *testing.T) {
	h := &GitSourceHandler{}
	records, err := h.listConnections("t1")
	if err != nil { t.Fatalf("unexpected err: %v", err) }
	if records != nil { t.Errorf("expected nil, got %v", records) }
}

func TestProviderForRequestEdge(t *testing.T) {
	h := NewGitSourceHandler(core.NewServices(), newMockBoltStore(), nil)
	_ = h.providerForRequest(httptest.NewRequest("GET", "/", nil), "github")

	req := httptest.NewRequest("GET", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	_ = h.providerForRequest(req, "github")
}

func TestAuthHandlerHelpers(t *testing.T) {
	h := NewAuthHandler(nil, newMockStore(), newMockBoltStore())
	if h.totpValidator != nil { t.Error("totpValidator should be nil") }

	h2 := &AuthHandler{}
	if h2.log() == nil { t.Error("log() should return default") }
	if h2.validateTOTP("u1", "c") { t.Error("validateTOTP should return false") }
}

func TestRegistrationSlugAll(t *testing.T) {
	slug := registrationTenantSlug("Test User")
	if !strings.HasPrefix(slug, "test-user-") { t.Errorf("got %q", slug) }
	slug2 := registrationTenantSlug(strings.Repeat("a", 100))
	if len(slug2) > 90 { t.Errorf("slug too long: %d", len(slug2)) }
}

func TestServerDeleteErrorPaths(t *testing.T) {
	h := NewServerHandler(newMockStore(), core.NewServices(), testCore().Events)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/local", nil)
	req.SetPathValue("id", "local")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Delete(rr, req)
	if rr.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", rr.Code) }

	s := newMockStore()
	s.addServer(&core.Server{ID: "s1", TenantID: ""})
	h2 := NewServerHandler(s, core.NewServices(), testCore().Events)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/s1", nil)
	req.SetPathValue("id", "s1")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h2.Delete(rr, req)
	if rr.Code != http.StatusForbidden { t.Errorf("expected 403, got %d", rr.Code) }
}

func TestGitDisconnectErrorPaths(t *testing.T) {
	h := NewGitSourceHandler(core.NewServices(), newMockBoltStore(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/", nil)
	h.Disconnect(rr, req)
	if rr.Code != http.StatusUnauthorized { t.Errorf("expected 401, got %d", rr.Code) }

	h2 := NewGitSourceHandler(core.NewServices(), nil, nil)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h2.Disconnect(rr, req)
	t.Logf("restore status=%d", rr.Code)

	h3 := NewGitSourceHandler(core.NewServices(), newMockBoltStore(), nil)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h3.Disconnect(rr, req)
	if rr.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", rr.Code) }
}

func TestSecretDeleteErrorPaths(t *testing.T) {
	h := NewSecretHandler(newMockStore(), nil, testCore().Events)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Delete(rr, req)
	if rr.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", rr.Code) }

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/s1", nil)
	req.SetPathValue("id", "s1")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Delete(rr, req)
	if rr.Code != http.StatusNotImplemented { t.Errorf("expected 501, got %d", rr.Code) }
}

func TestBackupRestoreNoStorage(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1"})
	h := NewBackupHandler(s, nil, testCore().Events)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/k1/restore", nil)
	req.SetPathValue("key", "k1")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Restore(rr, req)
	t.Logf("restore status=%d", rr.Code)
}

func TestStorageUsageNoAuth(t *testing.T) {
	h := NewStorageHandler(newMockStore(), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.Usage(rr, req)
	if rr.Code != http.StatusUnauthorized { t.Errorf("expected 401, got %d", rr.Code) }

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Usage(rr, req)
	if rr.Code != http.StatusOK { t.Errorf("expected 200, got %d", rr.Code) }
}

func TestSuspendResumeConflict(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "T", Status: "suspended"})
	h := NewSuspendHandler(s, nil, testCore().Events)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Suspend(rr, req)
	t.Logf("suspend status=%d", rr.Code)

	s2 := newMockStore()
	s2.addApp(&core.Application{ID: "a2", TenantID: "t1", Name: "T2", Status: "running"})
	h2 := NewSuspendHandler(s2, nil, testCore().Events)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/", nil)
	req.SetPathValue("id", "a2")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h2.Resume(rr, req)
}

func TestMonitoringAlertsListError(t *testing.T) {
	c := testCore()
	c.Services.Container = &mockContainerRuntime{listErr: fmt.Errorf("list failed")}
	h := NewMonitoringHandler(c, time.Now())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.Alerts(rr, req)
	if rr.Code != http.StatusOK { t.Errorf("expected 200, got %d", rr.Code) }
}

// TestSystemDiskAndAppDiskNil covers SysDisk and AppDisk both nil in
func TestSystemDiskAndAppDiskNil(t *testing.T) {
	h := NewDiskUsageHandler(newMockStore(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.SystemDisk(rr, req)
	t.Logf("system disk status=%d", rr.Code)

	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1"})
	h2 := NewDiskUsageHandler(s, nil)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/a1/disk", nil)
	req.SetPathValue("id", "a1")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h2.AppDisk(rr, req)
	if rr.Code != http.StatusOK { t.Errorf("expected 200, got %d", rr.Code) }
}

func TestVolumeCreateAndListErrors(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1"})
	r := &mockContainerRuntime{listErr: fmt.Errorf("err")}
	h := NewVolumeHandler(r, s, testCore().Events)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.List(rr, req)
	if rr.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", rr.Code) }

	h2 := NewVolumeHandler(nil, newMockStore(), testCore().Events)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/", strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h2.Create(rr, req)
	if rr.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", rr.Code) }
}

func TestDeployFreezeEndpointsNoAuth(t *testing.T) {
	h := NewDeployFreezeHandler(newMockStore(), testCore().Events, newMockBoltStore())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	h.Create(rr, req)
	if rr.Code != http.StatusUnauthorized { t.Errorf("expected 401, got %d", rr.Code) }

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	h.Get(rr, req)
	if rr.Code != http.StatusUnauthorized { t.Errorf("expected 401, got %d", rr.Code) }

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/f1", nil)
	req.SetPathValue("id", "f1")
	h.Delete(rr, req)
	if rr.Code != http.StatusUnauthorized { t.Errorf("expected 401, got %d", rr.Code) }
}

func TestStripeWebhookNotConfigured(t *testing.T) {
	h := NewStripeWebhookHandler(nil, newMockBoltStore(), slog.Default())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	h.ServeHTTP(rr, req)
	t.Logf("restore status=%d", rr.Code)
}

func TestCertificateListAndTopology(t *testing.T) {
	h := NewCertificateHandler(newMockStore(), newMockBoltStore())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.List(rr, req)
	if rr.Code != http.StatusUnauthorized { t.Errorf("expected 401, got %d", rr.Code) }

	th := &TopologyHandler{store: newMockStore()}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/", nil)
	th.Validate(rr, req)
	if rr.Code != http.StatusUnauthorized { t.Errorf("expected 401, got %d", rr.Code) }
}

func TestAppStartStopRestartNoRuntime(t *testing.T) {
	c := testCore()
	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "T", Status: "running"})
	h := NewAppHandler(s, c)

	for _, method := range []string{"start", "stop", "restart"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/"+method, nil)
		req.SetPathValue("id", "a1")
		req = withClaims(req, "u1", "t1", "owner", "u@e.com")
		switch method {
		case "start": h.Start(rr, req)
		case "stop": h.Stop(rr, req)
		case "restart": h.Restart(rr, req)
		}
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s expected 503, got %d", method, rr.Code)
		}
	}
}

func TestUserSessionsEdge(t *testing.T) {
	h := &SessionHandler{}
	s, err := h.GetUserSessions("u1")
	if err != nil { t.Fatalf("unexpected err: %v", err) }
	if s != nil { t.Errorf("expected nil, got %v", s) }

	bolt := newMockBoltStore()
	bolt.Set("user_sessions", "u1:j1", SessionTrackingInfo{UserID: "u1", JTI: "j1"}, 0)
	bolt.Set("user_sessions", "u1:j2", SessionTrackingInfo{UserID: "u1", JTI: "j2"}, 0)
	h.bolt = bolt
	s, err = h.GetUserSessions("u1")
	if err != nil { t.Fatalf("unexpected err: %v", err) }
	if len(s) != 2 { t.Errorf("expected 2, got %d", len(s)) }

	// Revoke all (empty)
	h2 := &SessionHandler{bolt: bolt}
	err = h2.revokeAllUserSessions(context.Background(), "u1")
	if err != nil { t.Fatalf("unexpected err: %v", err) }
}
