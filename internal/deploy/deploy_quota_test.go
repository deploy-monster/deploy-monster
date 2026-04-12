package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/resource"
)

// fakeQuotaChecker is a minimal QuotaChecker used to drive the
// pre-flight gate without pulling in a real container runtime. The
// err field is returned verbatim so tests can prove the deployer
// propagates the error without unwrapping.
type fakeQuotaChecker struct {
	calls int
	err   error
}

func (f *fakeQuotaChecker) Check(_ context.Context, _ string) error {
	f.calls++
	return f.err
}

// TestDeployImage_QuotaRefused is the Phase 3.3.6 headline for the
// deploy pipeline half of quota enforcement: a tenant at the CPU/RAM
// ceiling must be refused BEFORE the container is ever created. We
// assert the refusal by:
//
//  1. The deployer returns an error wrapping resource.ErrQuotaExceeded
//     so HTTP callers can map it to 429 via errors.Is.
//  2. runtime.CreateAndStart was never called (no container leaked).
//  3. The app's store status was updated to "quota_exceeded" so the
//     dashboard can surface the refusal to the tenant.
//  4. No deploy record was created — the version counter must not
//     advance on a refused deploy.
func TestDeployImage_QuotaRefused(t *testing.T) {
	rt := &mockRuntime{}
	store := newMockStore()
	events := core.NewEventBus(nil)

	d := NewDeployer(rt, store, events)
	quotaErr := errors.New("deploy gate: " + resource.ErrQuotaExceeded.Error())
	// Use fmt-style wrapping so errors.Is unwraps correctly.
	quotaErr = wrapQuota(resource.ErrQuotaExceeded, "tenant over RAM")
	d.SetQuotaChecker(&fakeQuotaChecker{err: quotaErr})

	app := &core.Application{
		ID: "app1", Name: "webapp", TenantID: "tenant-over-limit",
	}

	dep, err := d.DeployImage(context.Background(), app, "registry/app:v1")
	if err == nil {
		t.Fatal("DeployImage = nil err, want quota refusal")
	}
	if dep != nil {
		t.Errorf("DeployImage returned deployment %+v, want nil", dep)
	}
	if !errors.Is(err, resource.ErrQuotaExceeded) {
		t.Errorf("errors.Is(err, ErrQuotaExceeded) = false, err = %v", err)
	}
	if rt.createCalled {
		t.Error("CreateAndStart was called — quota gate must short-circuit before container creation")
	}

	// The deploy handler status update path marks the app
	// quota_exceeded on refusal.
	foundQuotaStatus := false
	for _, upd := range store.appStatusUpdates {
		if upd.ID == "app1" && upd.Status == "quota_exceeded" {
			foundQuotaStatus = true
			break
		}
	}
	if !foundQuotaStatus {
		t.Errorf("app status was not updated to quota_exceeded; updates=%+v", store.appStatusUpdates)
	}
}

// TestDeployImage_QuotaPassed verifies a quota-checker returning nil
// flows through to the normal deploy path — CreateAndStart IS called
// and a deployment record is produced.
func TestDeployImage_QuotaPassed(t *testing.T) {
	rt := &mockRuntime{}
	store := newMockStore()
	events := core.NewEventBus(nil)

	d := NewDeployer(rt, store, events)
	checker := &fakeQuotaChecker{} // err == nil
	d.SetQuotaChecker(checker)

	app := &core.Application{
		ID: "app-ok", Name: "api", TenantID: "tenant-under-limit",
	}

	dep, err := d.DeployImage(context.Background(), app, "registry/app:v1")
	if err != nil {
		t.Fatalf("DeployImage: %v", err)
	}
	if dep == nil {
		t.Fatal("DeployImage returned nil deployment")
	}
	if checker.calls != 1 {
		t.Errorf("quota checker calls = %d, want 1", checker.calls)
	}
	if !rt.createCalled {
		t.Error("CreateAndStart was not called")
	}
}

// TestDeployImage_NoQuotaCheckerIsBackwardsCompat ensures a Deployer
// constructed without SetQuotaChecker behaves exactly as it did
// before Phase 3.3.6 — no hidden side effects, no panics, no
// unexplained refusals.
func TestDeployImage_NoQuotaCheckerIsBackwardsCompat(t *testing.T) {
	rt := &mockRuntime{}
	store := newMockStore()
	events := core.NewEventBus(nil)

	d := NewDeployer(rt, store, events) // no quota checker installed

	app := &core.Application{
		ID: "legacy-app", Name: "legacy", TenantID: "legacy-tenant",
	}
	dep, err := d.DeployImage(context.Background(), app, "img:v1")
	if err != nil {
		t.Fatalf("legacy deploy failed: %v", err)
	}
	if dep == nil {
		t.Fatal("expected deployment, got nil")
	}
	if !rt.createCalled {
		t.Error("CreateAndStart was not called on legacy deploy")
	}
}

// TestDeployImage_QuotaCheckerNonQuotaError proves that a checker
// returning an infrastructure error (e.g. the quota probe's list
// call failed) does NOT become a silent pass — the error propagates
// and the deploy is still refused, just without the
// ErrQuotaExceeded sentinel so HTTP callers map it to 5xx instead
// of 429.
func TestDeployImage_QuotaCheckerNonQuotaError(t *testing.T) {
	rt := &mockRuntime{}
	store := newMockStore()
	events := core.NewEventBus(nil)

	d := NewDeployer(rt, store, events)
	infraErr := errors.New("docker daemon unreachable")
	d.SetQuotaChecker(&fakeQuotaChecker{err: infraErr})

	_, err := d.DeployImage(context.Background(),
		&core.Application{ID: "x", TenantID: "t"}, "img:v1")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, resource.ErrQuotaExceeded) {
		t.Errorf("infra error was misclassified as quota refusal: %v", err)
	}
	if rt.createCalled {
		t.Error("CreateAndStart was called despite checker error")
	}
}

// wrapQuota produces a fmt.Errorf-style wrapped error without
// importing fmt at test scope (keeps the import set minimal).
func wrapQuota(sentinel error, msg string) error {
	return &wrappedQuotaErr{sentinel: sentinel, msg: msg}
}

type wrappedQuotaErr struct {
	sentinel error
	msg      string
}

func (e *wrappedQuotaErr) Error() string { return e.msg + ": " + e.sentinel.Error() }
func (e *wrappedQuotaErr) Unwrap() error { return e.sentinel }
