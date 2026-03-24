package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Test constants ──────────────────────────────────────────────────────────

const testJWTSecret = "test-secret-key-for-jwt-signing-min32"

// ─── Mock Store ──────────────────────────────────────────────────────────────

// mockStore implements core.Store for testing. Only the methods actually called
// by the handlers under test are given real implementations; the rest panic so
// we notice immediately if a handler reaches an unexpected code path.
type mockStore struct {
	mu sync.Mutex

	// Configurable behaviour per-test.
	users       map[string]*core.User       // keyed by ID
	usersByEmail map[string]*core.User      // keyed by email
	memberships map[string]*core.TeamMember // keyed by userID
	apps        map[string]*core.Application
	appList     []core.Application
	appTotal    int

	// Tenants
	tenants map[string]*core.Tenant

	// Error overrides — if non-nil the corresponding method returns this error.
	errGetUserByEmail         error
	errGetUser                error
	errGetUserMembership      error
	errUpdateLastLogin        error
	errCreateTenantWithDefaults error
	errCreateUserWithMembership error
	errCreateApp              error
	errGetApp                 error
	errDeleteApp              error
	errUpdateAppStatus        error
	errListAppsByTenant       error
	errUpdateUser             error
	errUpdatePassword         error

	// Capture calls for assertions.
	lastLoginUserID string
	deletedAppID    string
	createdApp      *core.Application
	updatedStatus   map[string]string
	updatedUser     *core.User
	updatedPassword string
}

func newMockStore() *mockStore {
	return &mockStore{
		users:        make(map[string]*core.User),
		usersByEmail: make(map[string]*core.User),
		memberships:  make(map[string]*core.TeamMember),
		apps:         make(map[string]*core.Application),
		tenants:      make(map[string]*core.Tenant),
		updatedStatus: make(map[string]string),
	}
}

// addUser is a test helper that seeds a user into the mock store.
func (m *mockStore) addUser(u *core.User, membership *core.TeamMember) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = u
	m.usersByEmail[u.Email] = u
	if membership != nil {
		m.memberships[u.ID] = membership
	}
}

// addApp seeds an application into the mock store.
func (m *mockStore) addApp(app *core.Application) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.apps[app.ID] = app
}

// ─── UserStore implementation ────────────────────────────────────────────────

func (m *mockStore) GetUserByEmail(_ context.Context, email string) (*core.User, error) {
	if m.errGetUserByEmail != nil {
		return nil, m.errGetUserByEmail
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.usersByEmail[email]
	if !ok {
		return nil, core.ErrNotFound
	}
	return u, nil
}

func (m *mockStore) GetUser(_ context.Context, id string) (*core.User, error) {
	if m.errGetUser != nil {
		return nil, m.errGetUser
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	return u, nil
}

func (m *mockStore) GetUserMembership(_ context.Context, userID string) (*core.TeamMember, error) {
	if m.errGetUserMembership != nil {
		return nil, m.errGetUserMembership
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tm, ok := m.memberships[userID]
	if !ok {
		return nil, core.ErrNotFound
	}
	return tm, nil
}

func (m *mockStore) UpdateLastLogin(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastLoginUserID = userID
	return m.errUpdateLastLogin
}

func (m *mockStore) CreateUserWithMembership(_ context.Context, email, passwordHash, name, status, tenantID, roleID string) (string, error) {
	if m.errCreateUserWithMembership != nil {
		return "", m.errCreateUserWithMembership
	}
	id := core.GenerateID()
	u := &core.User{ID: id, Email: email, PasswordHash: passwordHash, Name: name, Status: status}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[id] = u
	m.usersByEmail[email] = u
	m.memberships[id] = &core.TeamMember{UserID: id, TenantID: tenantID, RoleID: roleID}
	return id, nil
}

func (m *mockStore) CreateUser(_ context.Context, _ *core.User) error {
	panic("mockStore.CreateUser not implemented")
}
func (m *mockStore) UpdateUser(_ context.Context, user *core.User) error {
	if m.errUpdateUser != nil {
		return m.errUpdateUser
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedUser = user
	m.users[user.ID] = user
	return nil
}
func (m *mockStore) UpdatePassword(_ context.Context, userID, passwordHash string) error {
	if m.errUpdatePassword != nil {
		return m.errUpdatePassword
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedPassword = passwordHash
	return nil
}
func (m *mockStore) CountUsers(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.users), nil
}

// ─── AppStore implementation ─────────────────────────────────────────────────

func (m *mockStore) CreateApp(_ context.Context, app *core.Application) error {
	if m.errCreateApp != nil {
		return m.errCreateApp
	}
	if app.ID == "" {
		app.ID = core.GenerateID()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.apps[app.ID] = app
	m.createdApp = app
	return nil
}

func (m *mockStore) GetApp(_ context.Context, id string) (*core.Application, error) {
	if m.errGetApp != nil {
		return nil, m.errGetApp
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.apps[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	return a, nil
}

func (m *mockStore) UpdateApp(_ context.Context, _ *core.Application) error {
	panic("mockStore.UpdateApp not implemented")
}

func (m *mockStore) ListAppsByTenant(_ context.Context, _ string, limit, offset int) ([]core.Application, int, error) {
	if m.errListAppsByTenant != nil {
		return nil, 0, m.errListAppsByTenant
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	total := m.appTotal
	if total == 0 {
		total = len(m.appList)
	}
	end := offset + limit
	if end > len(m.appList) {
		end = len(m.appList)
	}
	if offset > len(m.appList) {
		return nil, total, nil
	}
	return m.appList[offset:end], total, nil
}

func (m *mockStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	panic("mockStore.ListAppsByProject not implemented")
}

func (m *mockStore) UpdateAppStatus(_ context.Context, id, status string) error {
	if m.errUpdateAppStatus != nil {
		return m.errUpdateAppStatus
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedStatus[id] = status
	return nil
}

func (m *mockStore) DeleteApp(_ context.Context, id string) error {
	if m.errDeleteApp != nil {
		return m.errDeleteApp
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletedAppID = id
	delete(m.apps, id)
	return nil
}

// ─── TenantStore implementation ──────────────────────────────────────────────

func (m *mockStore) CreateTenant(_ context.Context, t *core.Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tenants[t.ID] = t
	return nil
}

func (m *mockStore) GetTenant(_ context.Context, id string) (*core.Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tenants[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	return t, nil
}

func (m *mockStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	panic("mockStore.GetTenantBySlug not implemented")
}

func (m *mockStore) UpdateTenant(_ context.Context, _ *core.Tenant) error {
	panic("mockStore.UpdateTenant not implemented")
}

func (m *mockStore) DeleteTenant(_ context.Context, _ string) error {
	panic("mockStore.DeleteTenant not implemented")
}

// ─── ProjectStore implementation ─────────────────────────────────────────────

func (m *mockStore) CreateProject(_ context.Context, _ *core.Project) error {
	panic("mockStore.CreateProject not implemented")
}

func (m *mockStore) GetProject(_ context.Context, _ string) (*core.Project, error) {
	panic("mockStore.GetProject not implemented")
}

func (m *mockStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	panic("mockStore.ListProjectsByTenant not implemented")
}

func (m *mockStore) DeleteProject(_ context.Context, _ string) error {
	panic("mockStore.DeleteProject not implemented")
}

func (m *mockStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	if m.errCreateTenantWithDefaults != nil {
		return "", m.errCreateTenantWithDefaults
	}
	id := core.GenerateID()
	return id, nil
}

// ─── DeploymentStore implementation ──────────────────────────────────────────

func (m *mockStore) CreateDeployment(_ context.Context, _ *core.Deployment) error {
	panic("mockStore.CreateDeployment not implemented")
}

func (m *mockStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	panic("mockStore.GetLatestDeployment not implemented")
}

func (m *mockStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	panic("mockStore.ListDeploymentsByApp not implemented")
}

func (m *mockStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) {
	panic("mockStore.GetNextDeployVersion not implemented")
}

// ─── DomainStore implementation ──────────────────────────────────────────────

func (m *mockStore) CreateDomain(_ context.Context, _ *core.Domain) error {
	panic("mockStore.CreateDomain not implemented")
}

func (m *mockStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	panic("mockStore.GetDomainByFQDN not implemented")
}

func (m *mockStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	panic("mockStore.ListDomainsByApp not implemented")
}

func (m *mockStore) DeleteDomain(_ context.Context, _ string) error {
	panic("mockStore.DeleteDomain not implemented")
}

func (m *mockStore) ListAllDomains(_ context.Context) ([]core.Domain, error) {
	panic("mockStore.ListAllDomains not implemented")
}

// ─── RoleStore implementation ────────────────────────────────────────────────

func (m *mockStore) GetRole(_ context.Context, _ string) (*core.Role, error) {
	panic("mockStore.GetRole not implemented")
}

func (m *mockStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) {
	panic("mockStore.ListRoles not implemented")
}

// ─── AuditStore implementation ───────────────────────────────────────────────

func (m *mockStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error {
	return nil // silently accept audit logs in tests
}

func (m *mockStore) ListAuditLogs(_ context.Context, _ string, _, _ int) ([]core.AuditEntry, int, error) {
	return nil, 0, nil
}

// ─── Store top-level methods ─────────────────────────────────────────────────

func (m *mockStore) Close() error                    { return nil }
func (m *mockStore) Ping(_ context.Context) error    { return nil }

// ─── Test Helpers ────────────────────────────────────────────────────────────

// testJWT returns a JWTService configured with the test secret.
func testJWT() *auth.JWTService {
	return auth.NewJWTService(testJWTSecret)
}

// testAuthModule returns an auth.Module wired with the test JWT and mock store.
// The module is NOT fully initialized (no DB init, no first-run); only the
// JWT() method works, which is all the handlers call.
func testAuthModule(store core.Store) *auth.Module {
	return auth.NewTestModule(testJWTSecret, store)
}

// testCore returns a minimal *core.Core suitable for handler tests.
func testCore() *core.Core {
	return &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
	}
}

// seedTestUser creates a user in the mock store with a bcrypt-hashed password
// and a matching membership. Returns the user.
func seedTestUser(store *mockStore, id, email, password, tenantID, roleID string) *core.User {
	hash, err := auth.HashPassword(password)
	if err != nil {
		panic("seedTestUser: " + err.Error())
	}
	u := &core.User{
		ID:           id,
		Email:        email,
		PasswordHash: hash,
		Name:         "Test User",
		Status:       "active",
	}
	tm := &core.TeamMember{
		UserID:   id,
		TenantID: tenantID,
		RoleID:   roleID,
	}
	store.addUser(u, tm)
	return u
}

// generateTestToken creates a valid JWT access token for the given user params.
func generateTestToken(userID, tenantID, roleID, email string) string {
	jwt := testJWT()
	pair, err := jwt.GenerateTokenPair(userID, tenantID, roleID, email)
	if err != nil {
		panic("generateTestToken: " + err.Error())
	}
	return pair.AccessToken
}

// generateTestRefreshToken creates a valid JWT refresh token.
func generateTestRefreshToken(userID, tenantID, roleID, email string) string {
	jwt := testJWT()
	pair, err := jwt.GenerateTokenPair(userID, tenantID, roleID, email)
	if err != nil {
		panic("generateTestRefreshToken: " + err.Error())
	}
	return pair.RefreshToken
}

// authedRequest creates an http.Request with a valid Bearer token in the
// Authorization header. It also injects the claims into the request context
// (simulating what the RequireAuth middleware does).
func authedRequest(method, url string, body *httptest.ResponseRecorder) *http.Request {
	return nil // placeholder — use authedRequestWithClaims instead
}

// withClaims returns a new request with auth claims set in its context,
// simulating what the RequireAuth middleware does.
func withClaims(r *http.Request, userID, tenantID, roleID, email string) *http.Request {
	claims := &auth.Claims{
		UserID:   userID,
		TenantID: tenantID,
		RoleID:   roleID,
		Email:    email,
	}
	ctx := auth.ContextWithClaims(r.Context(), claims)
	return r.WithContext(ctx)
}
