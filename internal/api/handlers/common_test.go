package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// Domains
	domains       map[string]*core.Domain // keyed by ID
	domainsByFQDN map[string]*core.Domain // keyed by FQDN
	domainsByApp  map[string][]core.Domain // keyed by appID

	// Projects
	projects    map[string][]core.Project // keyed by tenantID
	projectsByID map[string]*core.Project // keyed by project ID

	// Deployments
	latestDeployments map[string]*core.Deployment // keyed by appID
	nextDeployVersion map[string]int              // keyed by appID
	deploymentsByApp  map[string][]core.Deployment // keyed by appID

	// Roles
	roles map[string][]core.Role // keyed by tenantID

	// Audit logs
	auditLogs      map[string][]core.AuditEntry // keyed by tenantID
	auditLogsTotal map[string]int               // keyed by tenantID

	// Secrets
	secrets map[string][]core.Secret // keyed by tenantID

	// Invitations
	invitations    map[string][]core.Invitation // keyed by tenantID
	allTenantsList []core.Tenant

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
	errCreateDomain           error
	errGetDomainByFQDN        error
	errListDomainsByApp       error
	errListAllDomains         error
	errDeleteDomain           error
	errListProjectsByTenant   error
	errListRoles              error
	errListAuditLogs          error
	errUpdateApp              error
	errGetProject             error
	errGetLatestDeployment    error
	errGetNextDeployVersion   error
	errListDeploymentsByApp   error
	errCreateProject          error
	errDeleteProject          error
	errCreateDeployment       error
	errCreateSecret           error
	errCreateSecretVersion    error
	errListSecretsByTenant    error
	errCreateInvite           error
	errListInvitesByTenant    error
	errListAllTenants         error

	// Capture calls for assertions.
	lastLoginUserID string
	deletedAppID    string
	deletedDomainID string
	deletedProjectID string
	createdProject  *core.Project
	createdApp      *core.Application
	createdDomain   *core.Domain
	updatedStatus   map[string]string
	updatedUser     *core.User
	updatedPassword string
	updatedApp      *core.Application
}

func newMockStore() *mockStore {
	return &mockStore{
		users:          make(map[string]*core.User),
		usersByEmail:   make(map[string]*core.User),
		memberships:    make(map[string]*core.TeamMember),
		apps:           make(map[string]*core.Application),
		tenants:        make(map[string]*core.Tenant),
		domains:        make(map[string]*core.Domain),
		domainsByFQDN:  make(map[string]*core.Domain),
		domainsByApp:   make(map[string][]core.Domain),
		projects:          make(map[string][]core.Project),
		projectsByID:      make(map[string]*core.Project),
		latestDeployments: make(map[string]*core.Deployment),
		nextDeployVersion: make(map[string]int),
		deploymentsByApp:  make(map[string][]core.Deployment),
		roles:             make(map[string][]core.Role),
		auditLogs:      make(map[string][]core.AuditEntry),
		auditLogsTotal: make(map[string]int),
		secrets:        make(map[string][]core.Secret),
		invitations:    make(map[string][]core.Invitation),
		updatedStatus:  make(map[string]string),
	}
}

// addDomain seeds a domain into the mock store.
func (m *mockStore) addDomain(d *core.Domain) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domains[d.ID] = d
	m.domainsByFQDN[d.FQDN] = d
	m.domainsByApp[d.AppID] = append(m.domainsByApp[d.AppID], *d)
}

// addProject seeds a project for a tenant into the mock store.
func (m *mockStore) addProject(tenantID string, p core.Project) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projects[tenantID] = append(m.projects[tenantID], p)
}

// addProjectByID seeds a project into the mock store keyed by ID.
func (m *mockStore) addProjectByID(p *core.Project) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projectsByID[p.ID] = p
}

// addRole seeds a role for a tenant into the mock store.
func (m *mockStore) addRole(tenantID string, r core.Role) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roles[tenantID] = append(m.roles[tenantID], r)
}

// addAuditLog seeds an audit log entry for a tenant into the mock store.
func (m *mockStore) addAuditLog(tenantID string, entry core.AuditEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditLogs[tenantID] = append(m.auditLogs[tenantID], entry)
}

// addTenant seeds a tenant into the mock store.
func (m *mockStore) addTenant(t *core.Tenant) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tenants[t.ID] = t
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

func (m *mockStore) UpdateApp(_ context.Context, app *core.Application) error {
	if m.errUpdateApp != nil {
		return m.errUpdateApp
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.apps[app.ID] = app
	m.updatedApp = app
	return nil
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

func (m *mockStore) CreateProject(_ context.Context, p *core.Project) error {
	if m.errCreateProject != nil {
		return m.errCreateProject
	}
	if p.ID == "" {
		p.ID = core.GenerateID()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projectsByID[p.ID] = p
	m.projects[p.TenantID] = append(m.projects[p.TenantID], *p)
	m.createdProject = p
	return nil
}

func (m *mockStore) GetProject(_ context.Context, id string) (*core.Project, error) {
	if m.errGetProject != nil {
		return nil, m.errGetProject
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projectsByID[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	return p, nil
}

func (m *mockStore) ListProjectsByTenant(_ context.Context, tenantID string) ([]core.Project, error) {
	if m.errListProjectsByTenant != nil {
		return nil, m.errListProjectsByTenant
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.projects[tenantID], nil
}

func (m *mockStore) DeleteProject(_ context.Context, id string) error {
	if m.errDeleteProject != nil {
		return m.errDeleteProject
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletedProjectID = id
	delete(m.projectsByID, id)
	return nil
}

func (m *mockStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	if m.errCreateTenantWithDefaults != nil {
		return "", m.errCreateTenantWithDefaults
	}
	id := core.GenerateID()
	return id, nil
}

// ─── DeploymentStore implementation ──────────────────────────────────────────

func (m *mockStore) CreateDeployment(_ context.Context, d *core.Deployment) error {
	if m.errCreateDeployment != nil {
		return m.errCreateDeployment
	}
	if d.ID == "" {
		d.ID = core.GenerateID()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deploymentsByApp[d.AppID] = append(m.deploymentsByApp[d.AppID], *d)
	return nil
}

func (m *mockStore) GetLatestDeployment(_ context.Context, appID string) (*core.Deployment, error) {
	if m.errGetLatestDeployment != nil {
		return nil, m.errGetLatestDeployment
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.latestDeployments[appID]
	if !ok {
		return nil, core.ErrNotFound
	}
	return d, nil
}

func (m *mockStore) ListDeploymentsByApp(_ context.Context, appID string, _ int) ([]core.Deployment, error) {
	if m.errListDeploymentsByApp != nil {
		return nil, m.errListDeploymentsByApp
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deploymentsByApp[appID], nil
}

func (m *mockStore) GetNextDeployVersion(_ context.Context, appID string) (int, error) {
	if m.errGetNextDeployVersion != nil {
		return 0, m.errGetNextDeployVersion
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.nextDeployVersion[appID]
	if !ok {
		return 1, nil
	}
	return v, nil
}

// ─── DomainStore implementation ──────────────────────────────────────────────

func (m *mockStore) CreateDomain(_ context.Context, domain *core.Domain) error {
	if m.errCreateDomain != nil {
		return m.errCreateDomain
	}
	if domain.ID == "" {
		domain.ID = core.GenerateID()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domains[domain.ID] = domain
	m.domainsByFQDN[domain.FQDN] = domain
	m.domainsByApp[domain.AppID] = append(m.domainsByApp[domain.AppID], *domain)
	m.createdDomain = domain
	return nil
}

func (m *mockStore) GetDomainByFQDN(_ context.Context, fqdn string) (*core.Domain, error) {
	if m.errGetDomainByFQDN != nil {
		return nil, m.errGetDomainByFQDN
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.domainsByFQDN[fqdn]
	if !ok {
		return nil, core.ErrNotFound
	}
	return d, nil
}

func (m *mockStore) ListDomainsByApp(_ context.Context, appID string) ([]core.Domain, error) {
	if m.errListDomainsByApp != nil {
		return nil, m.errListDomainsByApp
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.domainsByApp[appID], nil
}

func (m *mockStore) DeleteDomain(_ context.Context, id string) error {
	if m.errDeleteDomain != nil {
		return m.errDeleteDomain
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.domains[id]
	if !ok {
		return core.ErrNotFound
	}
	delete(m.domains, id)
	delete(m.domainsByFQDN, d.FQDN)
	m.deletedDomainID = id
	return nil
}

func (m *mockStore) ListAllDomains(_ context.Context) ([]core.Domain, error) {
	if m.errListAllDomains != nil {
		return nil, m.errListAllDomains
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []core.Domain
	for _, d := range m.domains {
		result = append(result, *d)
	}
	return result, nil
}

// ─── RoleStore implementation ────────────────────────────────────────────────

func (m *mockStore) GetRole(_ context.Context, _ string) (*core.Role, error) {
	panic("mockStore.GetRole not implemented")
}

func (m *mockStore) ListRoles(_ context.Context, tenantID string) ([]core.Role, error) {
	if m.errListRoles != nil {
		return nil, m.errListRoles
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.roles[tenantID], nil
}

// ─── AuditStore implementation ───────────────────────────────────────────────

func (m *mockStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error {
	return nil // silently accept audit logs in tests
}

func (m *mockStore) ListAuditLogs(_ context.Context, tenantID string, _, _ int) ([]core.AuditEntry, int, error) {
	if m.errListAuditLogs != nil {
		return nil, 0, m.errListAuditLogs
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := m.auditLogs[tenantID]
	total := m.auditLogsTotal[tenantID]
	if total == 0 {
		total = len(entries)
	}
	return entries, total, nil
}

// ─── SecretStore implementation ───────────────────────────────────────────────

func (m *mockStore) CreateSecret(_ context.Context, secret *core.Secret) error {
	if m.errCreateSecret != nil {
		return m.errCreateSecret
	}
	if secret.ID == "" {
		secret.ID = core.GenerateID()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secrets[secret.TenantID] = append(m.secrets[secret.TenantID], *secret)
	return nil
}

func (m *mockStore) CreateSecretVersion(_ context.Context, version *core.SecretVersion) error {
	if m.errCreateSecretVersion != nil {
		return m.errCreateSecretVersion
	}
	if version.ID == "" {
		version.ID = core.GenerateID()
	}
	return nil
}

func (m *mockStore) ListSecretsByTenant(_ context.Context, tenantID string) ([]core.Secret, error) {
	if m.errListSecretsByTenant != nil {
		return nil, m.errListSecretsByTenant
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.secrets[tenantID], nil
}

// ─── InviteStore implementation ──────────────────────────────────────────────

func (m *mockStore) CreateInvite(_ context.Context, invite *core.Invitation) error {
	if m.errCreateInvite != nil {
		return m.errCreateInvite
	}
	if invite.ID == "" {
		invite.ID = core.GenerateID()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invitations[invite.TenantID] = append(m.invitations[invite.TenantID], *invite)
	return nil
}

func (m *mockStore) ListInvitesByTenant(_ context.Context, tenantID string) ([]core.Invitation, error) {
	if m.errListInvitesByTenant != nil {
		return nil, m.errListInvitesByTenant
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.invitations[tenantID], nil
}

func (m *mockStore) ListAllTenants(_ context.Context, limit, offset int) ([]core.Tenant, int, error) {
	if m.errListAllTenants != nil {
		return nil, 0, m.errListAllTenants
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	total := len(m.allTenantsList)
	end := offset + limit
	if end > total {
		end = total
	}
	if offset > total {
		return nil, total, nil
	}
	return m.allTenantsList[offset:end], total, nil
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

// addDeployment seeds a deployment into the mock store for an app.
func (m *mockStore) addDeployment(appID string, d core.Deployment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deploymentsByApp[appID] = append(m.deploymentsByApp[appID], d)
}

// ─── Mock Container Runtime ─────────────────────────────────────────────────

// mockContainerRuntime implements core.ContainerRuntime for testing.
type mockContainerRuntime struct {
	containers []core.ContainerInfo
	pingErr    error
	listErr    error
	logsData   string
	logsErr    error
}

func (m *mockContainerRuntime) Ping() error { return m.pingErr }

func (m *mockContainerRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "container-123", nil
}

func (m *mockContainerRuntime) Stop(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *mockContainerRuntime) Remove(_ context.Context, _ string, _ bool) error {
	return nil
}

func (m *mockContainerRuntime) Restart(_ context.Context, _ string) error {
	return nil
}

func (m *mockContainerRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	if m.logsErr != nil {
		return nil, m.logsErr
	}
	return io.NopCloser(strings.NewReader(m.logsData)), nil
}

func (m *mockContainerRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.containers, nil
}

// ─── Mock Bolt Storer ────────────────────────────────────────────────────────

// mockBoltStore implements core.BoltStorer for testing.
type mockBoltStore struct {
	mu   sync.Mutex
	data map[string]map[string][]byte // bucket -> key -> json bytes
}

func newMockBoltStore() *mockBoltStore {
	return &mockBoltStore{data: make(map[string]map[string][]byte)}
}

func (m *mockBoltStore) Set(bucket, key string, value any, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[bucket] == nil {
		m.data[bucket] = make(map[string][]byte)
	}
	b, _ := json.Marshal(value)
	m.data[bucket][key] = b
	return nil
}

func (m *mockBoltStore) Get(bucket, key string, dest any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	bkt, ok := m.data[bucket]
	if !ok {
		return fmt.Errorf("key not found")
	}
	raw, ok := bkt[key]
	if !ok {
		return fmt.Errorf("key not found")
	}
	return json.Unmarshal(raw, dest)
}

func (m *mockBoltStore) Delete(bucket, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if bkt, ok := m.data[bucket]; ok {
		delete(bkt, key)
	}
	return nil
}

func (m *mockBoltStore) Close() error { return nil }

// ─── Mock Notification Sender ────────────────────────────────────────────────

// mockNotificationSender implements core.NotificationSender for testing.
type mockNotificationSender struct {
	lastNotification *core.Notification
	sendErr          error
}

func (m *mockNotificationSender) Send(_ context.Context, n core.Notification) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.lastNotification = &n
	return nil
}
