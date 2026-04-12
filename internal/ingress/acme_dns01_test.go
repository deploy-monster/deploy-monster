package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// fakeDNSProvider records every Create/Delete call so tests can
// assert the solver wrote and cleaned up the expected record shape.
type fakeDNSProvider struct {
	mu           sync.Mutex
	name         string
	createCalls  []core.DNSRecord
	deleteCalls  []core.DNSRecord
	createErr    error
	deleteErr    error
	createCalled atomic.Int32
	deleteCalled atomic.Int32
}

func (f *fakeDNSProvider) Name() string { return f.name }

func (f *fakeDNSProvider) CreateRecord(_ context.Context, r core.DNSRecord) error {
	f.createCalled.Add(1)
	f.mu.Lock()
	f.createCalls = append(f.createCalls, r)
	f.mu.Unlock()
	return f.createErr
}

func (f *fakeDNSProvider) UpdateRecord(_ context.Context, _ core.DNSRecord) error {
	return nil
}

func (f *fakeDNSProvider) DeleteRecord(_ context.Context, r core.DNSRecord) error {
	f.deleteCalled.Add(1)
	f.mu.Lock()
	f.deleteCalls = append(f.deleteCalls, r)
	f.mu.Unlock()
	return f.deleteErr
}

func (f *fakeDNSProvider) Verify(_ context.Context, _ string) (bool, error) {
	return true, nil
}

// fakeResolver serves canned TXT responses. Each call to LookupTXT
// consumes one response slice from the queue; once the queue is
// empty the last entry is repeated forever so tests don't have to
// pre-fill every tick of the propagation loop.
type fakeResolver struct {
	mu        sync.Mutex
	responses [][]string
	errs      []error
	calls     atomic.Int32
}

func (r *fakeResolver) LookupTXT(_ context.Context, _ string) ([]string, error) {
	r.calls.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	var vals []string
	var err error
	if len(r.responses) > 0 {
		vals = r.responses[0]
		if len(r.responses) > 1 {
			r.responses = r.responses[1:]
		}
	}
	if len(r.errs) > 0 {
		err = r.errs[0]
		if len(r.errs) > 1 {
			r.errs = r.errs[1:]
		}
	}
	return vals, err
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDNS01ChallengeRecord_RFC8555(t *testing.T) {
	// RFC 8555 §8.4 fixed vector: keyAuth = "token.thumbprint", the
	// record value must be the unpadded base64url of sha256(keyAuth).
	keyAuth := "evaGxfADs6pSRb2LAv9IZf17Dt3juxGJ-PCt92wr-oA.9jg46WB3rR_AHD-EBXdN7cBkH1WOu0tA3M9fm21mqTI"
	sum := sha256.Sum256([]byte(keyAuth))
	wantValue := base64.RawURLEncoding.EncodeToString(sum[:])

	fqdn, value := DNS01ChallengeRecord("example.com", keyAuth)
	if fqdn != "_acme-challenge.example.com" {
		t.Errorf("fqdn = %q, want _acme-challenge.example.com", fqdn)
	}
	if value != wantValue {
		t.Errorf("value = %q, want %q", value, wantValue)
	}
	// Value must be unpadded base64url (no '=').
	for _, c := range value {
		if c == '=' {
			t.Errorf("value %q contains padding, must be RawURLEncoding", value)
			break
		}
	}
}

func TestDNS01ChallengeRecord_TrailingDotStripped(t *testing.T) {
	fqdn, _ := DNS01ChallengeRecord("example.com.", "ka")
	if fqdn != "_acme-challenge.example.com" {
		t.Errorf("fqdn = %q, want trailing dot stripped", fqdn)
	}
}

func TestDNS01ChallengeRecord_AlreadyPrefixed(t *testing.T) {
	// Defensive: if someone passes the already-prefixed form we must
	// not end up with "_acme-challenge._acme-challenge.example.com".
	fqdn, _ := DNS01ChallengeRecord("_acme-challenge.example.com", "ka")
	if fqdn != "_acme-challenge.example.com" {
		t.Errorf("fqdn = %q, want single prefix", fqdn)
	}
}

func TestDNS01Solver_Present_HappyPath(t *testing.T) {
	prov := &fakeDNSProvider{name: "fake"}
	_, wantValue := DNS01ChallengeRecord("example.com", "k1")
	res := &fakeResolver{
		responses: [][]string{{wantValue}}, // visible immediately
	}

	s := NewDNS01Solver(prov, silentLogger())
	s.resolver = res
	s.pollInterval = 1 * time.Millisecond
	s.propagationTimeout = 200 * time.Millisecond

	if err := s.Present(context.Background(), "example.com", "tok", "k1"); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if prov.createCalled.Load() != 1 {
		t.Errorf("CreateRecord calls = %d, want 1", prov.createCalled.Load())
	}
	if prov.deleteCalled.Load() != 0 {
		t.Error("DeleteRecord called on happy path — should only run on propagation failure")
	}
	prov.mu.Lock()
	got := prov.createCalls[0]
	prov.mu.Unlock()
	if got.Type != "TXT" {
		t.Errorf("record type = %q, want TXT", got.Type)
	}
	if got.Name != "_acme-challenge.example.com" {
		t.Errorf("record name = %q, want _acme-challenge.example.com", got.Name)
	}
	if got.Value != wantValue {
		t.Errorf("record value = %q, want %q", got.Value, wantValue)
	}
	if got.TTL != 60 {
		t.Errorf("record ttl = %d, want 60", got.TTL)
	}
}

func TestDNS01Solver_Present_DelayedVisibility(t *testing.T) {
	// First two lookups see nothing, third sees the record. Present
	// should succeed once the value shows up.
	prov := &fakeDNSProvider{name: "fake"}
	_, wantValue := DNS01ChallengeRecord("example.com", "k2")
	res := &fakeResolver{
		responses: [][]string{
			{},
			{"unrelated-record"},
			{wantValue},
		},
	}

	s := NewDNS01Solver(prov, silentLogger())
	s.resolver = res
	s.pollInterval = 1 * time.Millisecond
	s.propagationTimeout = 500 * time.Millisecond

	if err := s.Present(context.Background(), "example.com", "tok", "k2"); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if res.calls.Load() < 3 {
		t.Errorf("resolver calls = %d, want at least 3", res.calls.Load())
	}
	if prov.deleteCalled.Load() != 0 {
		t.Error("DeleteRecord called despite eventual propagation success")
	}
}

func TestDNS01Solver_Present_QuotedValueStripped(t *testing.T) {
	// Some resolvers wrap TXT values in double quotes on the wire.
	prov := &fakeDNSProvider{name: "fake"}
	_, wantValue := DNS01ChallengeRecord("example.com", "k3")
	res := &fakeResolver{
		responses: [][]string{{`"` + wantValue + `"`}},
	}

	s := NewDNS01Solver(prov, silentLogger())
	s.resolver = res
	s.pollInterval = 1 * time.Millisecond
	s.propagationTimeout = 200 * time.Millisecond

	if err := s.Present(context.Background(), "example.com", "tok", "k3"); err != nil {
		t.Fatalf("Present with quoted resolver value: %v", err)
	}
}

func TestDNS01Solver_Present_PropagationTimeout_CleansUp(t *testing.T) {
	// Resolver never returns the expected value → propagation times
	// out → solver must attempt to delete the orphaned TXT record and
	// return the original propagation error (not the cleanup result).
	prov := &fakeDNSProvider{name: "fake"}
	res := &fakeResolver{
		responses: [][]string{{"something-else"}},
	}

	s := NewDNS01Solver(prov, silentLogger())
	s.resolver = res
	s.pollInterval = 2 * time.Millisecond
	s.propagationTimeout = 20 * time.Millisecond

	err := s.Present(context.Background(), "example.com", "tok", "k4")
	if err == nil {
		t.Fatal("Present returned nil, want propagation timeout error")
	}
	if prov.createCalled.Load() != 1 {
		t.Errorf("CreateRecord calls = %d, want 1", prov.createCalled.Load())
	}
	if prov.deleteCalled.Load() != 1 {
		t.Errorf("DeleteRecord calls = %d, want 1 (orphan cleanup)", prov.deleteCalled.Load())
	}
}

func TestDNS01Solver_Present_CreateError(t *testing.T) {
	createErr := errors.New("provider rejected record")
	prov := &fakeDNSProvider{name: "fake", createErr: createErr}
	s := NewDNS01Solver(prov, silentLogger())
	s.resolver = &fakeResolver{}

	err := s.Present(context.Background(), "example.com", "tok", "k5")
	if err == nil || !errors.Is(err, createErr) {
		t.Errorf("err = %v, want wrap of %v", err, createErr)
	}
	if prov.deleteCalled.Load() != 0 {
		t.Error("DeleteRecord called despite CreateRecord failing — nothing to clean up")
	}
}

func TestDNS01Solver_Present_ContextCancelled(t *testing.T) {
	prov := &fakeDNSProvider{name: "fake"}
	res := &fakeResolver{
		responses: [][]string{{"no-match"}},
	}

	s := NewDNS01Solver(prov, silentLogger())
	s.resolver = res
	s.pollInterval = 10 * time.Millisecond
	s.propagationTimeout = 5 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)

	err := s.Present(ctx, "example.com", "tok", "k6")
	if err == nil {
		t.Fatal("Present returned nil on cancelled ctx")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled in chain", err)
	}
}

func TestDNS01Solver_Present_NilProvider(t *testing.T) {
	s := NewDNS01Solver(nil, silentLogger())
	if err := s.Present(context.Background(), "example.com", "tok", "ka"); err == nil {
		t.Error("Present with nil provider returned nil, want error")
	}
}

func TestDNS01Solver_Present_EmptyDomain(t *testing.T) {
	s := NewDNS01Solver(&fakeDNSProvider{}, silentLogger())
	if err := s.Present(context.Background(), "", "tok", "ka"); err == nil {
		t.Error("Present with empty domain returned nil, want error")
	}
}

func TestDNS01Solver_Present_EmptyKeyAuth(t *testing.T) {
	s := NewDNS01Solver(&fakeDNSProvider{}, silentLogger())
	if err := s.Present(context.Background(), "example.com", "tok", ""); err == nil {
		t.Error("Present with empty keyAuth returned nil, want error")
	}
}

func TestDNS01Solver_CleanUp_Deletes(t *testing.T) {
	prov := &fakeDNSProvider{name: "fake"}
	s := NewDNS01Solver(prov, silentLogger())

	if err := s.CleanUp(context.Background(), "example.com", "tok", "k7"); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if prov.deleteCalled.Load() != 1 {
		t.Fatalf("DeleteRecord calls = %d, want 1", prov.deleteCalled.Load())
	}
	prov.mu.Lock()
	got := prov.deleteCalls[0]
	prov.mu.Unlock()
	_, wantValue := DNS01ChallengeRecord("example.com", "k7")
	if got.Name != "_acme-challenge.example.com" || got.Value != wantValue || got.Type != "TXT" {
		t.Errorf("delete record = %+v, want TXT _acme-challenge.example.com with matching value", got)
	}
}

func TestDNS01Solver_CleanUp_ProviderError(t *testing.T) {
	delErr := errors.New("provider ate the record")
	prov := &fakeDNSProvider{name: "fake", deleteErr: delErr}
	s := NewDNS01Solver(prov, silentLogger())

	err := s.CleanUp(context.Background(), "example.com", "tok", "k8")
	if err == nil || !errors.Is(err, delErr) {
		t.Errorf("err = %v, want wrap of %v", err, delErr)
	}
}

func TestDNS01Solver_CleanUp_NilProvider(t *testing.T) {
	s := NewDNS01Solver(nil, silentLogger())
	if err := s.CleanUp(context.Background(), "example.com", "tok", "ka"); err == nil {
		t.Error("CleanUp with nil provider returned nil, want error")
	}
}

func TestNewDNS01Solver_NilLoggerTolerated(t *testing.T) {
	s := NewDNS01Solver(&fakeDNSProvider{}, nil)
	if s.logger == nil {
		t.Error("NewDNS01Solver left nil logger in place")
	}
}
