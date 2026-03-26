package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// testPostgresDriver is a minimal driver registered under "postgres" for testing NewPostgres.
type testPostgresDriver struct {
	openFunc func(name string) (driver.Conn, error)
}

func (d *testPostgresDriver) Open(name string) (driver.Conn, error) {
	if d.openFunc != nil {
		return d.openFunc(name)
	}
	return &testConn{}, nil
}

type testConn struct {
	execErr error
	pingErr error
}

func (c *testConn) Prepare(query string) (driver.Stmt, error) {
	return &testStmt{conn: c}, nil
}

func (c *testConn) Close() error { return nil }

func (c *testConn) Begin() (driver.Tx, error) { return &testTx{}, nil }

func (c *testConn) Ping(ctx context.Context) error { return c.pingErr }

type testStmt struct {
	conn *testConn
}

func (s *testStmt) Close() error { return nil }

func (s *testStmt) NumInput() int { return -1 }

func (s *testStmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.conn.execErr != nil {
		return nil, s.conn.execErr
	}
	return testResult{}, nil
}

func (s *testStmt) Query(args []driver.Value) (driver.Rows, error) {
	return nil, errors.New("not implemented")
}

type testTx struct{}

func (t *testTx) Commit() error   { return nil }
func (t *testTx) Rollback() error { return nil }

type testResult struct{}

func (r testResult) LastInsertId() (int64, error) { return 0, nil }
func (r testResult) RowsAffected() (int64, error) { return 0, nil }

var (
	registerOnce sync.Once
	testPgDriver *testPostgresDriver
)

func registerTestPostgresDriver() {
	registerOnce.Do(func() {
		testPgDriver = &testPostgresDriver{}
		sql.Register("postgres", testPgDriver)
	})
}

// newMockPostgres creates a PostgresDB with a sqlmock-backed *sql.DB.
func newMockPostgres(t *testing.T) (*PostgresDB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	pg := &PostgresDB{db: db, dsn: "mock"}
	t.Cleanup(func() { db.Close() })
	return pg, mock
}

// newMockPostgresWithPing creates a PostgresDB with ping monitoring enabled.
func newMockPostgresWithPing(t *testing.T) (*PostgresDB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	pg := &PostgresDB{db: db, dsn: "mock"}
	t.Cleanup(func() { db.Close() })
	return pg, mock
}

// =====================================================
// Ping / Close
// =====================================================

func TestPostgresDB_Ping(t *testing.T) {
	pg, mock := newMockPostgresWithPing(t)
	mock.ExpectPing()
	if err := pg.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

func TestPostgresDB_Ping_Error(t *testing.T) {
	pg, mock := newMockPostgresWithPing(t)
	mock.ExpectPing().WillReturnError(errors.New("connection refused"))
	if err := pg.Ping(context.Background()); err == nil {
		t.Fatal("expected error from Ping()")
	}
}

func TestPostgresDB_Close(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	pg := &PostgresDB{db: db, dsn: "mock"}
	mock.ExpectClose()
	if err := pg.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestPostgresDB_Close_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	pg := &PostgresDB{db: db, dsn: "mock"}
	mock.ExpectClose().WillReturnError(errors.New("close failed"))
	if err := pg.Close(); err == nil {
		t.Fatal("expected error from Close()")
	}
}

// =====================================================
// migrate()
// =====================================================

func TestPostgresDB_Migrate_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS tenants").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := pg.migrate(); err != nil {
		t.Fatalf("migrate() error = %v", err)
	}
}

func TestPostgresDB_Migrate_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS tenants").
		WillReturnError(errors.New("syntax error"))
	if err := pg.migrate(); err == nil {
		t.Fatal("expected error from migrate()")
	}
}

// =====================================================
// Tenant CRUD
// =====================================================

func TestPostgresDB_CreateTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	tenant := &core.Tenant{
		ID: "t1", Name: "Acme", Slug: "acme", PlanID: "free",
		OwnerID: "u1", Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectExec("INSERT INTO tenants").
		WithArgs(tenant.ID, tenant.Name, tenant.Slug, tenant.PlanID, tenant.OwnerID, tenant.Status, tenant.CreatedAt, tenant.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateTenant(context.Background(), tenant); err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
}

func TestPostgresDB_CreateTenant_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	tenant := &core.Tenant{
		ID: "t1", Name: "Acme", Slug: "acme", PlanID: "free",
		OwnerID: "u1", Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectExec("INSERT INTO tenants").
		WithArgs(tenant.ID, tenant.Name, tenant.Slug, tenant.PlanID, tenant.OwnerID, tenant.Status, tenant.CreatedAt, tenant.UpdatedAt).
		WillReturnError(errors.New("duplicate key"))

	if err := pg.CreateTenant(context.Background(), tenant); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_GetTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "slug", "avatar_url", "plan_id", "owner_id", "status", "limits_json", "metadata_json", "created_at", "updated_at"}).
		AddRow("t1", "Acme", "acme", "", "free", "u1", "active", "{}", "{}", now, now)
	mock.ExpectQuery("SELECT .+ FROM tenants WHERE id = \\$1").
		WithArgs("t1").
		WillReturnRows(rows)

	tenant, err := pg.GetTenant(context.Background(), "t1")
	if err != nil {
		t.Fatalf("GetTenant() error = %v", err)
	}
	if tenant.ID != "t1" || tenant.Name != "Acme" {
		t.Fatalf("unexpected tenant: %+v", tenant)
	}
}

func TestPostgresDB_GetTenant_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM tenants WHERE id = \\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := pg.GetTenant(context.Background(), "missing")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetTenant_DBError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM tenants WHERE id = \\$1").
		WithArgs("t1").
		WillReturnError(errors.New("connection lost"))

	_, err := pg.GetTenant(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_GetTenantBySlug_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "slug", "avatar_url", "plan_id", "owner_id", "status", "created_at", "updated_at"}).
		AddRow("t1", "Acme", "acme", "", "free", "u1", "active", now, now)
	mock.ExpectQuery("SELECT .+ FROM tenants WHERE slug = \\$1").
		WithArgs("acme").
		WillReturnRows(rows)

	tenant, err := pg.GetTenantBySlug(context.Background(), "acme")
	if err != nil {
		t.Fatalf("GetTenantBySlug() error = %v", err)
	}
	if tenant.Slug != "acme" {
		t.Fatalf("unexpected slug: %s", tenant.Slug)
	}
}

func TestPostgresDB_GetTenantBySlug_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM tenants WHERE slug = \\$1").
		WithArgs("nope").
		WillReturnError(sql.ErrNoRows)

	_, err := pg.GetTenantBySlug(context.Background(), "nope")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetTenantBySlug_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM tenants WHERE slug = \\$1").
		WithArgs("acme").
		WillReturnError(errors.New("db error"))

	_, err := pg.GetTenantBySlug(context.Background(), "acme")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_UpdateTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	tenant := &core.Tenant{ID: "t1", Name: "Updated", Slug: "updated", PlanID: "pro", Status: "active"}
	mock.ExpectExec("UPDATE tenants SET").
		WithArgs(tenant.Name, tenant.Slug, tenant.PlanID, tenant.Status, sqlmock.AnyArg(), tenant.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pg.UpdateTenant(context.Background(), tenant); err != nil {
		t.Fatalf("UpdateTenant() error = %v", err)
	}
}

func TestPostgresDB_UpdateTenant_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	tenant := &core.Tenant{ID: "t1", Name: "Updated", Slug: "updated", PlanID: "pro", Status: "active"}
	mock.ExpectExec("UPDATE tenants SET").
		WithArgs(tenant.Name, tenant.Slug, tenant.PlanID, tenant.Status, sqlmock.AnyArg(), tenant.ID).
		WillReturnError(errors.New("update failed"))

	if err := pg.UpdateTenant(context.Background(), tenant); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_DeleteTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM tenants WHERE id = \\$1").
		WithArgs("t1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pg.DeleteTenant(context.Background(), "t1"); err != nil {
		t.Fatalf("DeleteTenant() error = %v", err)
	}
}

func TestPostgresDB_DeleteTenant_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM tenants WHERE id = \\$1").
		WithArgs("t1").
		WillReturnError(errors.New("fk violation"))

	if err := pg.DeleteTenant(context.Background(), "t1"); err == nil {
		t.Fatal("expected error")
	}
}

// =====================================================
// User CRUD
// =====================================================

func TestPostgresDB_CreateUser_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	u := &core.User{ID: "u1", Email: "a@b.com", PasswordHash: "hash", Name: "Alice", AvatarURL: "", Status: "active"}
	mock.ExpectExec("INSERT INTO users").
		WithArgs(u.ID, u.Email, u.PasswordHash, u.Name, u.AvatarURL, u.Status).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
}

func TestPostgresDB_CreateUser_GeneratesID(t *testing.T) {
	pg, mock := newMockPostgres(t)
	u := &core.User{Email: "a@b.com", PasswordHash: "hash", Name: "Alice", Status: "active"}
	mock.ExpectExec("INSERT INTO users").
		WithArgs(sqlmock.AnyArg(), u.Email, u.PasswordHash, u.Name, u.AvatarURL, u.Status).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if u.ID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestPostgresDB_CreateUser_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	u := &core.User{ID: "u1", Email: "a@b.com", PasswordHash: "hash", Name: "Alice", Status: "active"}
	mock.ExpectExec("INSERT INTO users").
		WithArgs(u.ID, u.Email, u.PasswordHash, u.Name, u.AvatarURL, u.Status).
		WillReturnError(errors.New("duplicate"))

	if err := pg.CreateUser(context.Background(), u); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_GetUser_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	loginAt := now.Add(-time.Hour)
	rows := sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "avatar_url", "status", "totp_enabled", "last_login_at", "created_at", "updated_at"}).
		AddRow("u1", "a@b.com", "hash", "Alice", "", "active", false, &loginAt, now, now)
	mock.ExpectQuery("SELECT .+ FROM users WHERE id = \\$1").
		WithArgs("u1").
		WillReturnRows(rows)

	user, err := pg.GetUser(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if user.Email != "a@b.com" {
		t.Fatalf("unexpected email: %s", user.Email)
	}
}

func TestPostgresDB_GetUser_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM users WHERE id = \\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := pg.GetUser(context.Background(), "missing")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetUser_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM users WHERE id = \\$1").
		WithArgs("u1").
		WillReturnError(errors.New("db down"))

	_, err := pg.GetUser(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_GetUserByEmail_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "avatar_url", "status", "totp_enabled", "last_login_at", "created_at", "updated_at"}).
		AddRow("u1", "a@b.com", "hash", "Alice", "", "active", false, nil, now, now)
	mock.ExpectQuery("SELECT .+ FROM users WHERE email = \\$1").
		WithArgs("a@b.com").
		WillReturnRows(rows)

	user, err := pg.GetUserByEmail(context.Background(), "a@b.com")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if user.ID != "u1" {
		t.Fatalf("unexpected ID: %s", user.ID)
	}
}

func TestPostgresDB_GetUserByEmail_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM users WHERE email = \\$1").
		WithArgs("nope@nope.com").
		WillReturnError(sql.ErrNoRows)

	_, err := pg.GetUserByEmail(context.Background(), "nope@nope.com")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetUserByEmail_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM users WHERE email = \\$1").
		WithArgs("a@b.com").
		WillReturnError(errors.New("timeout"))

	_, err := pg.GetUserByEmail(context.Background(), "a@b.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_UpdateUser_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	u := &core.User{ID: "u1", Email: "a@b.com", Name: "Bob", AvatarURL: "", Status: "active"}
	mock.ExpectExec("UPDATE users SET").
		WithArgs(u.Email, u.Name, u.AvatarURL, u.Status, u.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pg.UpdateUser(context.Background(), u); err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}
}

func TestPostgresDB_UpdateUser_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	u := &core.User{ID: "u1", Email: "a@b.com", Name: "Bob", Status: "active"}
	mock.ExpectExec("UPDATE users SET").
		WithArgs(u.Email, u.Name, u.AvatarURL, u.Status, u.ID).
		WillReturnError(errors.New("update error"))

	if err := pg.UpdateUser(context.Background(), u); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_UpdatePassword_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE users SET password_hash").
		WithArgs("newhash", "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pg.UpdatePassword(context.Background(), "u1", "newhash"); err != nil {
		t.Fatalf("UpdatePassword() error = %v", err)
	}
}

func TestPostgresDB_UpdatePassword_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE users SET password_hash").
		WithArgs("newhash", "u1").
		WillReturnError(errors.New("fail"))

	if err := pg.UpdatePassword(context.Background(), "u1", "newhash"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_UpdateLastLogin_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE users SET last_login_at").
		WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pg.UpdateLastLogin(context.Background(), "u1"); err != nil {
		t.Fatalf("UpdateLastLogin() error = %v", err)
	}
}

func TestPostgresDB_UpdateLastLogin_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE users SET last_login_at").
		WithArgs("u1").
		WillReturnError(errors.New("timeout"))

	if err := pg.UpdateLastLogin(context.Background(), "u1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CountUsers_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"count"}).AddRow(42)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(rows)

	count, err := pg.CountUsers(context.Background())
	if err != nil {
		t.Fatalf("CountUsers() error = %v", err)
	}
	if count != 42 {
		t.Fatalf("expected 42, got %d", count)
	}
}

func TestPostgresDB_CountUsers_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT COUNT").WillReturnError(errors.New("db error"))

	_, err := pg.CountUsers(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CreateUserWithMembership_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(sqlmock.AnyArg(), "a@b.com", "hash", "Alice", "active").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO team_members").
		WithArgs(sqlmock.AnyArg(), "t1", sqlmock.AnyArg(), "role1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE tenants SET owner_id").
		WithArgs(sqlmock.AnyArg(), "t1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	userID, err := pg.CreateUserWithMembership(context.Background(), "a@b.com", "hash", "Alice", "active", "t1", "role1")
	if err != nil {
		t.Fatalf("CreateUserWithMembership() error = %v", err)
	}
	if userID == "" {
		t.Fatal("expected non-empty userID")
	}
}

func TestPostgresDB_CreateUserWithMembership_BeginError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin().WillReturnError(errors.New("begin fail"))

	_, err := pg.CreateUserWithMembership(context.Background(), "a@b.com", "hash", "Alice", "active", "t1", "role1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CreateUserWithMembership_InsertUserError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(sqlmock.AnyArg(), "a@b.com", "hash", "Alice", "active").
		WillReturnError(errors.New("dup email"))
	mock.ExpectRollback()

	_, err := pg.CreateUserWithMembership(context.Background(), "a@b.com", "hash", "Alice", "active", "t1", "role1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CreateUserWithMembership_InsertMemberError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(sqlmock.AnyArg(), "a@b.com", "hash", "Alice", "active").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO team_members").
		WithArgs(sqlmock.AnyArg(), "t1", sqlmock.AnyArg(), "role1").
		WillReturnError(errors.New("fk violation"))
	mock.ExpectRollback()

	_, err := pg.CreateUserWithMembership(context.Background(), "a@b.com", "hash", "Alice", "active", "t1", "role1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CreateUserWithMembership_UpdateTenantError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(sqlmock.AnyArg(), "a@b.com", "hash", "Alice", "active").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO team_members").
		WithArgs(sqlmock.AnyArg(), "t1", sqlmock.AnyArg(), "role1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE tenants SET owner_id").
		WithArgs(sqlmock.AnyArg(), "t1").
		WillReturnError(errors.New("update fail"))
	mock.ExpectRollback()

	_, err := pg.CreateUserWithMembership(context.Background(), "a@b.com", "hash", "Alice", "active", "t1", "role1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CreateUserWithMembership_CommitError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(sqlmock.AnyArg(), "a@b.com", "hash", "Alice", "active").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO team_members").
		WithArgs(sqlmock.AnyArg(), "t1", sqlmock.AnyArg(), "role1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE tenants SET owner_id").
		WithArgs(sqlmock.AnyArg(), "t1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	_, err := pg.CreateUserWithMembership(context.Background(), "a@b.com", "hash", "Alice", "active", "t1", "role1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// =====================================================
// App CRUD
// =====================================================

func TestPostgresDB_CreateApp_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	app := &core.Application{
		ID: "a1", ProjectID: "p1", TenantID: "t1", Name: "myapp", Type: "web",
		SourceType: "git", SourceURL: "https://github.com/x/y", Branch: "main",
		Dockerfile: "Dockerfile", BuildPack: "", EnvVarsEnc: "", LabelsJSON: "{}",
		Replicas: 1, Status: "created", ServerID: "s1",
	}
	mock.ExpectExec("INSERT INTO applications").
		WithArgs(app.ID, app.ProjectID, app.TenantID, app.Name, app.Type, app.SourceType,
			app.SourceURL, app.Branch, app.Dockerfile, app.BuildPack, app.EnvVarsEnc,
			app.LabelsJSON, app.Replicas, app.Status, app.ServerID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateApp(context.Background(), app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
}

func TestPostgresDB_CreateApp_GeneratesID(t *testing.T) {
	pg, mock := newMockPostgres(t)
	app := &core.Application{
		ProjectID: "p1", TenantID: "t1", Name: "myapp", Type: "web",
		SourceType: "git", SourceURL: "https://github.com/x/y", Branch: "main",
		Replicas: 1, Status: "created",
	}
	mock.ExpectExec("INSERT INTO applications").
		WithArgs(sqlmock.AnyArg(), app.ProjectID, app.TenantID, app.Name, app.Type, app.SourceType,
			app.SourceURL, app.Branch, app.Dockerfile, app.BuildPack, app.EnvVarsEnc,
			app.LabelsJSON, app.Replicas, app.Status, app.ServerID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateApp(context.Background(), app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	if app.ID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestPostgresDB_CreateApp_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	app := &core.Application{ID: "a1", ProjectID: "p1", TenantID: "t1", Name: "myapp", Type: "web",
		SourceType: "git", SourceURL: "", Branch: "main", Replicas: 1, Status: "created"}
	mock.ExpectExec("INSERT INTO applications").
		WithArgs(app.ID, app.ProjectID, app.TenantID, app.Name, app.Type, app.SourceType,
			app.SourceURL, app.Branch, app.Dockerfile, app.BuildPack, app.EnvVarsEnc,
			app.LabelsJSON, app.Replicas, app.Status, app.ServerID).
		WillReturnError(errors.New("insert failed"))

	if err := pg.CreateApp(context.Background(), app); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_GetApp_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "project_id", "tenant_id", "name", "type", "source_type", "source_url", "branch",
		"dockerfile", "build_pack", "env_vars_enc", "labels_json", "replicas", "status", "server_id",
		"created_at", "updated_at",
	}).AddRow("a1", "p1", "t1", "myapp", "web", "git", "https://github.com/x/y", "main",
		"Dockerfile", "", "", "{}", 1, "running", "s1", now, now)

	mock.ExpectQuery("SELECT .+ FROM applications WHERE id = \\$1").
		WithArgs("a1").
		WillReturnRows(rows)

	app, err := pg.GetApp(context.Background(), "a1")
	if err != nil {
		t.Fatalf("GetApp() error = %v", err)
	}
	if app.Name != "myapp" || app.Status != "running" {
		t.Fatalf("unexpected app: %+v", app)
	}
}

func TestPostgresDB_GetApp_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM applications WHERE id = \\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := pg.GetApp(context.Background(), "missing")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetApp_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM applications WHERE id = \\$1").
		WithArgs("a1").
		WillReturnError(errors.New("scan fail"))

	_, err := pg.GetApp(context.Background(), "a1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_UpdateApp_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	app := &core.Application{
		ID: "a1", Name: "updated", SourceURL: "https://x.com", Branch: "dev",
		Dockerfile: "Dockerfile", EnvVarsEnc: "enc", LabelsJSON: "{}", Replicas: 2, Status: "running",
	}
	mock.ExpectExec("UPDATE applications SET").
		WithArgs(app.Name, app.SourceURL, app.Branch, app.Dockerfile,
			app.EnvVarsEnc, app.LabelsJSON, app.Replicas, app.Status, app.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pg.UpdateApp(context.Background(), app); err != nil {
		t.Fatalf("UpdateApp() error = %v", err)
	}
}

func TestPostgresDB_UpdateApp_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	app := &core.Application{ID: "a1", Name: "updated", Branch: "dev", Replicas: 2, Status: "running"}
	mock.ExpectExec("UPDATE applications SET").
		WithArgs(app.Name, app.SourceURL, app.Branch, app.Dockerfile,
			app.EnvVarsEnc, app.LabelsJSON, app.Replicas, app.Status, app.ID).
		WillReturnError(errors.New("update fail"))

	if err := pg.UpdateApp(context.Background(), app); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAppsByTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()

	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery("SELECT COUNT").
		WithArgs("t1").
		WillReturnRows(countRows)

	appRows := sqlmock.NewRows([]string{
		"id", "project_id", "tenant_id", "name", "type", "source_type", "source_url", "branch",
		"status", "replicas", "created_at", "updated_at",
	}).
		AddRow("a1", "p1", "t1", "app1", "web", "git", "", "main", "running", 1, now, now).
		AddRow("a2", "p1", "t1", "app2", "web", "image", "", "main", "stopped", 1, now, now)
	mock.ExpectQuery("SELECT .+ FROM applications WHERE tenant_id").
		WithArgs("t1", 10, 0).
		WillReturnRows(appRows)

	apps, total, err := pg.ListAppsByTenant(context.Background(), "t1", 10, 0)
	if err != nil {
		t.Fatalf("ListAppsByTenant() error = %v", err)
	}
	if total != 2 || len(apps) != 2 {
		t.Fatalf("expected 2 apps, got total=%d len=%d", total, len(apps))
	}
}

func TestPostgresDB_ListAppsByTenant_CountError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT COUNT").
		WithArgs("t1").
		WillReturnError(errors.New("count fail"))

	_, _, err := pg.ListAppsByTenant(context.Background(), "t1", 10, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAppsByTenant_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT").WithArgs("t1").WillReturnRows(countRows)
	mock.ExpectQuery("SELECT .+ FROM applications WHERE tenant_id").
		WithArgs("t1", 10, 0).
		WillReturnError(errors.New("query fail"))

	_, _, err := pg.ListAppsByTenant(context.Background(), "t1", 10, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAppsByTenant_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT").WithArgs("t1").WillReturnRows(countRows)

	// Return wrong number of columns to cause scan error
	appRows := sqlmock.NewRows([]string{"id", "project_id"}).
		AddRow("a1", "p1")
	mock.ExpectQuery("SELECT .+ FROM applications WHERE tenant_id").
		WithArgs("t1", 10, 0).
		WillReturnRows(appRows)

	_, _, err := pg.ListAppsByTenant(context.Background(), "t1", 10, 0)
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestPostgresDB_ListAppsByProject_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	appRows := sqlmock.NewRows([]string{
		"id", "project_id", "tenant_id", "name", "type", "source_type", "source_url", "branch",
		"status", "replicas", "created_at", "updated_at",
	}).
		AddRow("a1", "p1", "t1", "app1", "web", "git", "", "main", "running", 1, now, now)
	mock.ExpectQuery("SELECT .+ FROM applications WHERE project_id = \\$1").
		WithArgs("p1").
		WillReturnRows(appRows)

	apps, err := pg.ListAppsByProject(context.Background(), "p1")
	if err != nil {
		t.Fatalf("ListAppsByProject() error = %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
}

func TestPostgresDB_ListAppsByProject_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM applications WHERE project_id = \\$1").
		WithArgs("p1").
		WillReturnError(errors.New("query fail"))

	_, err := pg.ListAppsByProject(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAppsByProject_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	appRows := sqlmock.NewRows([]string{"id"}).AddRow("a1")
	mock.ExpectQuery("SELECT .+ FROM applications WHERE project_id = \\$1").
		WithArgs("p1").
		WillReturnRows(appRows)

	_, err := pg.ListAppsByProject(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestPostgresDB_UpdateAppStatus_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE applications SET status").
		WithArgs("running", "a1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pg.UpdateAppStatus(context.Background(), "a1", "running"); err != nil {
		t.Fatalf("UpdateAppStatus() error = %v", err)
	}
}

func TestPostgresDB_UpdateAppStatus_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE applications SET status").
		WithArgs("running", "a1").
		WillReturnError(errors.New("fail"))

	if err := pg.UpdateAppStatus(context.Background(), "a1", "running"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_DeleteApp_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM applications WHERE id = \\$1").
		WithArgs("a1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pg.DeleteApp(context.Background(), "a1"); err != nil {
		t.Fatalf("DeleteApp() error = %v", err)
	}
}

func TestPostgresDB_DeleteApp_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM applications WHERE id = \\$1").
		WithArgs("a1").
		WillReturnError(errors.New("delete fail"))

	if err := pg.DeleteApp(context.Background(), "a1"); err == nil {
		t.Fatal("expected error")
	}
}

// =====================================================
// Deployment CRUD
// =====================================================

func TestPostgresDB_CreateDeployment_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	startedAt := time.Now()
	d := &core.Deployment{
		ID: "d1", AppID: "a1", Version: 1, Image: "img:v1", ContainerID: "c1",
		Status: "running", BuildLog: "ok", CommitSHA: "abc", CommitMessage: "init",
		TriggeredBy: "manual", Strategy: "recreate", StartedAt: &startedAt,
	}
	mock.ExpectExec("INSERT INTO deployments").
		WithArgs(d.ID, d.AppID, d.Version, d.Image, d.ContainerID, d.Status, d.BuildLog,
			d.CommitSHA, d.CommitMessage, d.TriggeredBy, d.Strategy, d.StartedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateDeployment(context.Background(), d); err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}
}

func TestPostgresDB_CreateDeployment_GeneratesID(t *testing.T) {
	pg, mock := newMockPostgres(t)
	d := &core.Deployment{
		AppID: "a1", Version: 1, Image: "img:v1", Status: "pending",
		TriggeredBy: "manual", Strategy: "recreate",
	}
	mock.ExpectExec("INSERT INTO deployments").
		WithArgs(sqlmock.AnyArg(), d.AppID, d.Version, d.Image, d.ContainerID, d.Status, d.BuildLog,
			d.CommitSHA, d.CommitMessage, d.TriggeredBy, d.Strategy, d.StartedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateDeployment(context.Background(), d); err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}
	if d.ID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestPostgresDB_CreateDeployment_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	d := &core.Deployment{ID: "d1", AppID: "a1", Version: 1, Status: "pending",
		TriggeredBy: "manual", Strategy: "recreate"}
	mock.ExpectExec("INSERT INTO deployments").
		WithArgs(d.ID, d.AppID, d.Version, d.Image, d.ContainerID, d.Status, d.BuildLog,
			d.CommitSHA, d.CommitMessage, d.TriggeredBy, d.Strategy, d.StartedAt).
		WillReturnError(errors.New("insert fail"))

	if err := pg.CreateDeployment(context.Background(), d); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_GetLatestDeployment_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	startedAt := now.Add(-time.Minute)
	finishedAt := now
	rows := sqlmock.NewRows([]string{
		"id", "app_id", "version", "image", "container_id", "status",
		"commit_sha", "commit_message", "triggered_by", "strategy",
		"started_at", "finished_at", "created_at",
	}).AddRow("d1", "a1", 3, "img:v3", "c1", "running",
		"abc", "deploy v3", "manual", "recreate",
		&startedAt, &finishedAt, now)

	mock.ExpectQuery("SELECT .+ FROM deployments WHERE app_id = \\$1 ORDER BY version DESC LIMIT 1").
		WithArgs("a1").
		WillReturnRows(rows)

	dep, err := pg.GetLatestDeployment(context.Background(), "a1")
	if err != nil {
		t.Fatalf("GetLatestDeployment() error = %v", err)
	}
	if dep.Version != 3 {
		t.Fatalf("expected version 3, got %d", dep.Version)
	}
}

func TestPostgresDB_GetLatestDeployment_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM deployments WHERE app_id = \\$1").
		WithArgs("nope").
		WillReturnError(sql.ErrNoRows)

	_, err := pg.GetLatestDeployment(context.Background(), "nope")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetLatestDeployment_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM deployments WHERE app_id = \\$1").
		WithArgs("a1").
		WillReturnError(errors.New("db error"))

	_, err := pg.GetLatestDeployment(context.Background(), "a1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListDeploymentsByApp_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "app_id", "version", "image", "container_id", "status",
		"commit_sha", "commit_message", "triggered_by", "strategy",
		"started_at", "finished_at", "created_at",
	}).
		AddRow("d2", "a1", 2, "img:v2", "", "running", "", "", "manual", "recreate", nil, nil, now).
		AddRow("d1", "a1", 1, "img:v1", "", "stopped", "", "", "manual", "recreate", nil, nil, now)

	mock.ExpectQuery("SELECT .+ FROM deployments WHERE app_id = \\$1 ORDER BY version DESC LIMIT \\$2").
		WithArgs("a1", 10).
		WillReturnRows(rows)

	deps, err := pg.ListDeploymentsByApp(context.Background(), "a1", 10)
	if err != nil {
		t.Fatalf("ListDeploymentsByApp() error = %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(deps))
	}
}

func TestPostgresDB_ListDeploymentsByApp_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM deployments WHERE app_id = \\$1").
		WithArgs("a1", 10).
		WillReturnError(errors.New("fail"))

	_, err := pg.ListDeploymentsByApp(context.Background(), "a1", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListDeploymentsByApp_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow("d1")
	mock.ExpectQuery("SELECT .+ FROM deployments WHERE app_id = \\$1").
		WithArgs("a1", 10).
		WillReturnRows(rows)

	_, err := pg.ListDeploymentsByApp(context.Background(), "a1", 10)
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestPostgresDB_GetNextDeployVersion_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"max"}).AddRow(5)
	mock.ExpectQuery("SELECT MAX").
		WithArgs("a1").
		WillReturnRows(rows)

	v, err := pg.GetNextDeployVersion(context.Background(), "a1")
	if err != nil {
		t.Fatalf("GetNextDeployVersion() error = %v", err)
	}
	if v != 6 {
		t.Fatalf("expected 6, got %d", v)
	}
}

func TestPostgresDB_GetNextDeployVersion_NoDeployments(t *testing.T) {
	pg, mock := newMockPostgres(t)
	// NULL result when no deployments exist
	rows := sqlmock.NewRows([]string{"max"}).AddRow(nil)
	mock.ExpectQuery("SELECT MAX").
		WithArgs("a1").
		WillReturnRows(rows)

	v, err := pg.GetNextDeployVersion(context.Background(), "a1")
	if err != nil {
		t.Fatalf("GetNextDeployVersion() error = %v", err)
	}
	if v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}
}

func TestPostgresDB_GetNextDeployVersion_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT MAX").
		WithArgs("a1").
		WillReturnError(errors.New("db error"))

	v, err := pg.GetNextDeployVersion(context.Background(), "a1")
	if err == nil {
		t.Fatal("expected error")
	}
	if v != 1 {
		t.Fatalf("expected default 1 on error, got %d", v)
	}
}

// =====================================================
// Domain CRUD
// =====================================================

func TestPostgresDB_CreateDomain_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	d := &core.Domain{
		ID: "d1", AppID: "a1", FQDN: "example.com", Type: "custom",
		DNSProvider: "cloudflare", DNSSynced: false, Verified: false,
	}
	mock.ExpectExec("INSERT INTO domains").
		WithArgs(d.ID, d.AppID, d.FQDN, d.Type, d.DNSProvider, d.DNSSynced, d.Verified).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateDomain(context.Background(), d); err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}
}

func TestPostgresDB_CreateDomain_GeneratesID(t *testing.T) {
	pg, mock := newMockPostgres(t)
	d := &core.Domain{AppID: "a1", FQDN: "example.com", Type: "custom"}
	mock.ExpectExec("INSERT INTO domains").
		WithArgs(sqlmock.AnyArg(), d.AppID, d.FQDN, d.Type, d.DNSProvider, d.DNSSynced, d.Verified).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateDomain(context.Background(), d); err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}
	if d.ID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestPostgresDB_CreateDomain_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	d := &core.Domain{ID: "d1", AppID: "a1", FQDN: "example.com", Type: "custom"}
	mock.ExpectExec("INSERT INTO domains").
		WithArgs(d.ID, d.AppID, d.FQDN, d.Type, d.DNSProvider, d.DNSSynced, d.Verified).
		WillReturnError(errors.New("dup fqdn"))

	if err := pg.CreateDomain(context.Background(), d); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_GetDomainByFQDN_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "app_id", "fqdn", "type", "dns_provider", "dns_synced", "verified", "created_at"}).
		AddRow("d1", "a1", "example.com", "custom", "cloudflare", true, true, now)
	mock.ExpectQuery("SELECT .+ FROM domains WHERE fqdn = \\$1").
		WithArgs("example.com").
		WillReturnRows(rows)

	dom, err := pg.GetDomainByFQDN(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("GetDomainByFQDN() error = %v", err)
	}
	if dom.FQDN != "example.com" {
		t.Fatalf("unexpected FQDN: %s", dom.FQDN)
	}
}

func TestPostgresDB_GetDomainByFQDN_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM domains WHERE fqdn = \\$1").
		WithArgs("nope.com").
		WillReturnError(sql.ErrNoRows)

	_, err := pg.GetDomainByFQDN(context.Background(), "nope.com")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetDomainByFQDN_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM domains WHERE fqdn = \\$1").
		WithArgs("example.com").
		WillReturnError(errors.New("db error"))

	_, err := pg.GetDomainByFQDN(context.Background(), "example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListDomainsByApp_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "app_id", "fqdn", "type", "dns_provider", "dns_synced", "verified", "created_at"}).
		AddRow("d1", "a1", "example.com", "custom", "cloudflare", true, true, now).
		AddRow("d2", "a1", "api.example.com", "custom", "", false, false, now)
	mock.ExpectQuery("SELECT .+ FROM domains WHERE app_id = \\$1").
		WithArgs("a1").
		WillReturnRows(rows)

	domains, err := pg.ListDomainsByApp(context.Background(), "a1")
	if err != nil {
		t.Fatalf("ListDomainsByApp() error = %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(domains))
	}
}

func TestPostgresDB_ListDomainsByApp_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM domains WHERE app_id = \\$1").
		WithArgs("a1").
		WillReturnError(errors.New("fail"))

	_, err := pg.ListDomainsByApp(context.Background(), "a1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListDomainsByApp_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow("d1")
	mock.ExpectQuery("SELECT .+ FROM domains WHERE app_id = \\$1").
		WithArgs("a1").
		WillReturnRows(rows)

	_, err := pg.ListDomainsByApp(context.Background(), "a1")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestPostgresDB_DeleteDomain_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM domains WHERE id = \\$1").
		WithArgs("d1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pg.DeleteDomain(context.Background(), "d1"); err != nil {
		t.Fatalf("DeleteDomain() error = %v", err)
	}
}

func TestPostgresDB_DeleteDomain_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM domains WHERE id = \\$1").
		WithArgs("d1").
		WillReturnError(errors.New("delete fail"))

	if err := pg.DeleteDomain(context.Background(), "d1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAllDomains_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "app_id", "fqdn", "type", "dns_provider", "dns_synced", "verified", "created_at"}).
		AddRow("d1", "a1", "example.com", "custom", "", false, false, now)
	mock.ExpectQuery("SELECT .+ FROM domains ORDER BY created_at").
		WillReturnRows(rows)

	domains, err := pg.ListAllDomains(context.Background())
	if err != nil {
		t.Fatalf("ListAllDomains() error = %v", err)
	}
	if len(domains) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(domains))
	}
}

func TestPostgresDB_ListAllDomains_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM domains ORDER BY created_at").
		WillReturnError(errors.New("fail"))

	_, err := pg.ListAllDomains(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAllDomains_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow("d1")
	mock.ExpectQuery("SELECT .+ FROM domains ORDER BY created_at").
		WillReturnRows(rows)

	_, err := pg.ListAllDomains(context.Background())
	if err == nil {
		t.Fatal("expected scan error")
	}
}

// =====================================================
// Project CRUD
// =====================================================

func TestPostgresDB_CreateProject_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	proj := &core.Project{
		ID: "p1", TenantID: "t1", Name: "myproj", Description: "desc", Environment: "production",
	}
	mock.ExpectExec("INSERT INTO projects").
		WithArgs(proj.ID, proj.TenantID, proj.Name, proj.Description, proj.Environment).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateProject(context.Background(), proj); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
}

func TestPostgresDB_CreateProject_GeneratesID(t *testing.T) {
	pg, mock := newMockPostgres(t)
	proj := &core.Project{TenantID: "t1", Name: "myproj", Description: "desc", Environment: "production"}
	mock.ExpectExec("INSERT INTO projects").
		WithArgs(sqlmock.AnyArg(), proj.TenantID, proj.Name, proj.Description, proj.Environment).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateProject(context.Background(), proj); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if proj.ID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestPostgresDB_CreateProject_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	proj := &core.Project{ID: "p1", TenantID: "t1", Name: "myproj", Description: "desc", Environment: "production"}
	mock.ExpectExec("INSERT INTO projects").
		WithArgs(proj.ID, proj.TenantID, proj.Name, proj.Description, proj.Environment).
		WillReturnError(errors.New("dup"))

	if err := pg.CreateProject(context.Background(), proj); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_GetProject_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "tenant_id", "name", "description", "environment", "created_at", "updated_at"}).
		AddRow("p1", "t1", "myproj", "desc", "production", now, now)
	mock.ExpectQuery("SELECT .+ FROM projects WHERE id = \\$1").
		WithArgs("p1").
		WillReturnRows(rows)

	proj, err := pg.GetProject(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if proj.Name != "myproj" {
		t.Fatalf("unexpected name: %s", proj.Name)
	}
}

func TestPostgresDB_GetProject_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM projects WHERE id = \\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := pg.GetProject(context.Background(), "missing")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetProject_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM projects WHERE id = \\$1").
		WithArgs("p1").
		WillReturnError(errors.New("db error"))

	_, err := pg.GetProject(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListProjectsByTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "tenant_id", "name", "description", "environment", "created_at", "updated_at"}).
		AddRow("p1", "t1", "proj1", "desc1", "production", now, now).
		AddRow("p2", "t1", "proj2", "desc2", "staging", now, now)
	mock.ExpectQuery("SELECT .+ FROM projects WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnRows(rows)

	projects, err := pg.ListProjectsByTenant(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ListProjectsByTenant() error = %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestPostgresDB_ListProjectsByTenant_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM projects WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnError(errors.New("fail"))

	_, err := pg.ListProjectsByTenant(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListProjectsByTenant_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow("p1")
	mock.ExpectQuery("SELECT .+ FROM projects WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnRows(rows)

	_, err := pg.ListProjectsByTenant(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestPostgresDB_DeleteProject_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM projects WHERE id = \\$1").
		WithArgs("p1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := pg.DeleteProject(context.Background(), "p1"); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
}

func TestPostgresDB_DeleteProject_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM projects WHERE id = \\$1").
		WithArgs("p1").
		WillReturnError(errors.New("fk"))

	if err := pg.DeleteProject(context.Background(), "p1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CreateTenantWithDefaults_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO tenants").
		WithArgs(sqlmock.AnyArg(), "Acme", "acme").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO projects").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tenantID, err := pg.CreateTenantWithDefaults(context.Background(), "Acme", "acme")
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults() error = %v", err)
	}
	if tenantID == "" {
		t.Fatal("expected non-empty tenantID")
	}
}

func TestPostgresDB_CreateTenantWithDefaults_BeginError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin().WillReturnError(errors.New("begin fail"))

	_, err := pg.CreateTenantWithDefaults(context.Background(), "Acme", "acme")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CreateTenantWithDefaults_InsertTenantError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO tenants").
		WithArgs(sqlmock.AnyArg(), "Acme", "acme").
		WillReturnError(errors.New("dup slug"))
	mock.ExpectRollback()

	_, err := pg.CreateTenantWithDefaults(context.Background(), "Acme", "acme")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CreateTenantWithDefaults_InsertProjectError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO tenants").
		WithArgs(sqlmock.AnyArg(), "Acme", "acme").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO projects").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("insert project fail"))
	mock.ExpectRollback()

	_, err := pg.CreateTenantWithDefaults(context.Background(), "Acme", "acme")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CreateTenantWithDefaults_CommitError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO tenants").
		WithArgs(sqlmock.AnyArg(), "Acme", "acme").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO projects").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit fail"))

	_, err := pg.CreateTenantWithDefaults(context.Background(), "Acme", "acme")
	if err == nil {
		t.Fatal("expected error")
	}
}

// =====================================================
// Role + TeamMember
// =====================================================

func TestPostgresDB_GetRole_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "tenant_id", "name", "description", "permissions_json", "is_builtin", "created_at"}).
		AddRow("r1", "t1", "admin", "Admin role", `["*"]`, true, now)
	mock.ExpectQuery("SELECT .+ FROM roles WHERE id = \\$1").
		WithArgs("r1").
		WillReturnRows(rows)

	role, err := pg.GetRole(context.Background(), "r1")
	if err != nil {
		t.Fatalf("GetRole() error = %v", err)
	}
	if role.Name != "admin" {
		t.Fatalf("unexpected name: %s", role.Name)
	}
}

func TestPostgresDB_GetRole_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM roles WHERE id = \\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := pg.GetRole(context.Background(), "missing")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetRole_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM roles WHERE id = \\$1").
		WithArgs("r1").
		WillReturnError(errors.New("db error"))

	_, err := pg.GetRole(context.Background(), "r1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_GetUserMembership_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "tenant_id", "user_id", "role_id", "status", "created_at"}).
		AddRow("tm1", "t1", "u1", "r1", "active", now)
	mock.ExpectQuery("SELECT .+ FROM team_members WHERE user_id = \\$1").
		WithArgs("u1").
		WillReturnRows(rows)

	tm, err := pg.GetUserMembership(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetUserMembership() error = %v", err)
	}
	if tm.TenantID != "t1" {
		t.Fatalf("unexpected tenantID: %s", tm.TenantID)
	}
}

func TestPostgresDB_GetUserMembership_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM team_members WHERE user_id = \\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := pg.GetUserMembership(context.Background(), "missing")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetUserMembership_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM team_members WHERE user_id = \\$1").
		WithArgs("u1").
		WillReturnError(errors.New("db error"))

	_, err := pg.GetUserMembership(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListRoles_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "tenant_id", "name", "description", "permissions_json", "is_builtin", "created_at"}).
		AddRow("r1", "", "admin", "Admin", `["*"]`, true, now).
		AddRow("r2", "t1", "viewer", "View only", `["read"]`, false, now)
	mock.ExpectQuery("SELECT .+ FROM roles WHERE tenant_id = \\$1 OR is_builtin = TRUE").
		WithArgs("t1").
		WillReturnRows(rows)

	roles, err := pg.ListRoles(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}
}

func TestPostgresDB_ListRoles_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM roles WHERE tenant_id = \\$1 OR is_builtin = TRUE").
		WithArgs("t1").
		WillReturnError(errors.New("fail"))

	_, err := pg.ListRoles(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListRoles_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow("r1")
	mock.ExpectQuery("SELECT .+ FROM roles WHERE tenant_id = \\$1 OR is_builtin = TRUE").
		WithArgs("t1").
		WillReturnRows(rows)

	_, err := pg.ListRoles(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

// =====================================================
// Audit Log
// =====================================================

func TestPostgresDB_CreateAuditLog_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	entry := &core.AuditEntry{
		TenantID: "t1", UserID: "u1", Action: "create", ResourceType: "app",
		ResourceID: "a1", DetailsJSON: "{}", IPAddress: "127.0.0.1", UserAgent: "test",
	}
	mock.ExpectExec("INSERT INTO audit_log").
		WithArgs(entry.TenantID, entry.UserID, entry.Action, entry.ResourceType,
			entry.ResourceID, entry.DetailsJSON, entry.IPAddress, entry.UserAgent).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateAuditLog(context.Background(), entry); err != nil {
		t.Fatalf("CreateAuditLog() error = %v", err)
	}
}

func TestPostgresDB_CreateAuditLog_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	entry := &core.AuditEntry{
		TenantID: "t1", UserID: "u1", Action: "create", ResourceType: "app",
		ResourceID: "a1", DetailsJSON: "{}", IPAddress: "127.0.0.1", UserAgent: "test",
	}
	mock.ExpectExec("INSERT INTO audit_log").
		WithArgs(entry.TenantID, entry.UserID, entry.Action, entry.ResourceType,
			entry.ResourceID, entry.DetailsJSON, entry.IPAddress, entry.UserAgent).
		WillReturnError(errors.New("insert fail"))

	if err := pg.CreateAuditLog(context.Background(), entry); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAuditLogs_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()

	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT").WithArgs("t1").WillReturnRows(countRows)

	logRows := sqlmock.NewRows([]string{
		"id", "tenant_id", "user_id", "action", "resource_type", "resource_id",
		"details_json", "ip_address", "user_agent", "created_at",
	}).AddRow(int64(1), "t1", "u1", "create", "app", "a1", "{}", "127.0.0.1", "test", now)
	mock.ExpectQuery("SELECT .+ FROM audit_log WHERE tenant_id = \\$1").
		WithArgs("t1", 10, 0).
		WillReturnRows(logRows)

	entries, total, err := pg.ListAuditLogs(context.Background(), "t1", 10, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected 1 entry, got total=%d len=%d", total, len(entries))
	}
}

func TestPostgresDB_ListAuditLogs_CountError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT COUNT").WithArgs("t1").WillReturnError(errors.New("count fail"))

	_, _, err := pg.ListAuditLogs(context.Background(), "t1", 10, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAuditLogs_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT").WithArgs("t1").WillReturnRows(countRows)
	mock.ExpectQuery("SELECT .+ FROM audit_log WHERE tenant_id = \\$1").
		WithArgs("t1", 10, 0).
		WillReturnError(errors.New("query fail"))

	_, _, err := pg.ListAuditLogs(context.Background(), "t1", 10, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAuditLogs_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT").WithArgs("t1").WillReturnRows(countRows)
	logRows := sqlmock.NewRows([]string{"id"}).AddRow(1)
	mock.ExpectQuery("SELECT .+ FROM audit_log WHERE tenant_id = \\$1").
		WithArgs("t1", 10, 0).
		WillReturnRows(logRows)

	_, _, err := pg.ListAuditLogs(context.Background(), "t1", 10, 0)
	if err == nil {
		t.Fatal("expected scan error")
	}
}

// =====================================================
// Secret CRUD
// =====================================================

func TestPostgresDB_CreateSecret_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	secret := &core.Secret{
		ID: "s1", TenantID: "t1", ProjectID: "p1", AppID: "a1",
		Name: "DB_PASSWORD", Type: "env", Description: "Database password",
		Scope: "app", CurrentVersion: 1,
	}
	mock.ExpectExec("INSERT INTO secrets").
		WithArgs(secret.ID, secret.TenantID, secret.ProjectID, secret.AppID,
			secret.Name, secret.Type, secret.Description, secret.Scope, secret.CurrentVersion).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateSecret(context.Background(), secret); err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}
}

func TestPostgresDB_CreateSecret_GeneratesID(t *testing.T) {
	pg, mock := newMockPostgres(t)
	secret := &core.Secret{
		TenantID: "t1", Name: "SECRET", Type: "env", Scope: "app", CurrentVersion: 1,
	}
	mock.ExpectExec("INSERT INTO secrets").
		WithArgs(sqlmock.AnyArg(), secret.TenantID, secret.ProjectID, secret.AppID,
			secret.Name, secret.Type, secret.Description, secret.Scope, secret.CurrentVersion).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateSecret(context.Background(), secret); err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}
	if secret.ID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestPostgresDB_CreateSecret_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	secret := &core.Secret{ID: "s1", TenantID: "t1", Name: "SECRET", Type: "env", Scope: "app", CurrentVersion: 1}
	mock.ExpectExec("INSERT INTO secrets").
		WithArgs(secret.ID, secret.TenantID, secret.ProjectID, secret.AppID,
			secret.Name, secret.Type, secret.Description, secret.Scope, secret.CurrentVersion).
		WillReturnError(errors.New("insert fail"))

	if err := pg.CreateSecret(context.Background(), secret); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_CreateSecretVersion_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	version := &core.SecretVersion{
		ID: "sv1", SecretID: "s1", Version: 1, ValueEnc: "encrypted", CreatedBy: "u1",
	}
	mock.ExpectExec("INSERT INTO secret_versions").
		WithArgs(version.ID, version.SecretID, version.Version, version.ValueEnc, version.CreatedBy).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateSecretVersion(context.Background(), version); err != nil {
		t.Fatalf("CreateSecretVersion() error = %v", err)
	}
}

func TestPostgresDB_CreateSecretVersion_GeneratesID(t *testing.T) {
	pg, mock := newMockPostgres(t)
	version := &core.SecretVersion{SecretID: "s1", Version: 1, ValueEnc: "enc", CreatedBy: "u1"}
	mock.ExpectExec("INSERT INTO secret_versions").
		WithArgs(sqlmock.AnyArg(), version.SecretID, version.Version, version.ValueEnc, version.CreatedBy).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateSecretVersion(context.Background(), version); err != nil {
		t.Fatalf("CreateSecretVersion() error = %v", err)
	}
	if version.ID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestPostgresDB_CreateSecretVersion_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	version := &core.SecretVersion{ID: "sv1", SecretID: "s1", Version: 1, ValueEnc: "enc", CreatedBy: "u1"}
	mock.ExpectExec("INSERT INTO secret_versions").
		WithArgs(version.ID, version.SecretID, version.Version, version.ValueEnc, version.CreatedBy).
		WillReturnError(errors.New("dup version"))

	if err := pg.CreateSecretVersion(context.Background(), version); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListSecretsByTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "tenant_id", "project_id", "app_id", "name", "type",
		"description", "scope", "current_version", "created_at", "updated_at",
	}).
		AddRow("s1", "t1", "p1", "a1", "DB_PASSWORD", "env", "desc", "app", 1, now, now).
		AddRow("s2", "t1", "", "", "API_KEY", "env", "", "tenant", 1, now, now)
	mock.ExpectQuery("SELECT .+ FROM secrets WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnRows(rows)

	secrets, err := pg.ListSecretsByTenant(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ListSecretsByTenant() error = %v", err)
	}
	if len(secrets) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(secrets))
	}
}

func TestPostgresDB_ListSecretsByTenant_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM secrets WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnError(errors.New("fail"))

	_, err := pg.ListSecretsByTenant(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListSecretsByTenant_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow("s1")
	mock.ExpectQuery("SELECT .+ FROM secrets WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnRows(rows)

	_, err := pg.ListSecretsByTenant(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

// =====================================================
// Invite CRUD
// =====================================================

func TestPostgresDB_CreateInvite_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	inv := &core.Invitation{
		ID: "i1", TenantID: "t1", Email: "a@b.com", RoleID: "r1",
		InvitedBy: "u1", TokenHash: "hash123", ExpiresAt: time.Now().Add(24 * time.Hour),
		Status: "pending",
	}
	mock.ExpectExec("INSERT INTO invitations").
		WithArgs(inv.ID, inv.TenantID, inv.Email, inv.RoleID,
			inv.InvitedBy, inv.TokenHash, inv.ExpiresAt, inv.Status).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateInvite(context.Background(), inv); err != nil {
		t.Fatalf("CreateInvite() error = %v", err)
	}
}

func TestPostgresDB_CreateInvite_GeneratesID(t *testing.T) {
	pg, mock := newMockPostgres(t)
	inv := &core.Invitation{
		TenantID: "t1", Email: "a@b.com", RoleID: "r1",
		InvitedBy: "u1", TokenHash: "hash123", ExpiresAt: time.Now().Add(24 * time.Hour),
		Status: "pending",
	}
	mock.ExpectExec("INSERT INTO invitations").
		WithArgs(sqlmock.AnyArg(), inv.TenantID, inv.Email, inv.RoleID,
			inv.InvitedBy, inv.TokenHash, inv.ExpiresAt, inv.Status).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := pg.CreateInvite(context.Background(), inv); err != nil {
		t.Fatalf("CreateInvite() error = %v", err)
	}
	if inv.ID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestPostgresDB_CreateInvite_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	inv := &core.Invitation{
		ID: "i1", TenantID: "t1", Email: "a@b.com", RoleID: "r1",
		InvitedBy: "u1", TokenHash: "hash123", ExpiresAt: time.Now().Add(24 * time.Hour),
		Status: "pending",
	}
	mock.ExpectExec("INSERT INTO invitations").
		WithArgs(inv.ID, inv.TenantID, inv.Email, inv.RoleID,
			inv.InvitedBy, inv.TokenHash, inv.ExpiresAt, inv.Status).
		WillReturnError(errors.New("dup token"))

	if err := pg.CreateInvite(context.Background(), inv); err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListInvitesByTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	expires := now.Add(24 * time.Hour)
	rows := sqlmock.NewRows([]string{
		"id", "tenant_id", "email", "role_id", "invited_by", "token_hash",
		"expires_at", "accepted_at", "status", "created_at",
	}).
		AddRow("i1", "t1", "a@b.com", "r1", "u1", "hash", expires, nil, "pending", now)
	mock.ExpectQuery("SELECT .+ FROM invitations WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnRows(rows)

	invites, err := pg.ListInvitesByTenant(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ListInvitesByTenant() error = %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(invites))
	}
}

func TestPostgresDB_ListInvitesByTenant_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM invitations WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnError(errors.New("fail"))

	_, err := pg.ListInvitesByTenant(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListInvitesByTenant_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow("i1")
	mock.ExpectQuery("SELECT .+ FROM invitations WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnRows(rows)

	_, err := pg.ListInvitesByTenant(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

// =====================================================
// ListAllTenants
// =====================================================

func TestPostgresDB_ListAllTenants_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()

	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(countRows)

	tenantRows := sqlmock.NewRows([]string{
		"id", "name", "slug", "avatar_url", "plan_id", "owner_id",
		"status", "limits_json", "metadata_json", "created_at", "updated_at",
	}).
		AddRow("t1", "Acme", "acme", "", "free", "u1", "active", "{}", "{}", now, now).
		AddRow("t2", "Beta", "beta", "", "pro", "u2", "active", "{}", "{}", now, now)
	mock.ExpectQuery("SELECT .+ FROM tenants ORDER BY created_at DESC").
		WithArgs(10, 0).
		WillReturnRows(tenantRows)

	tenants, total, err := pg.ListAllTenants(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("ListAllTenants() error = %v", err)
	}
	if total != 2 || len(tenants) != 2 {
		t.Fatalf("expected 2 tenants, got total=%d len=%d", total, len(tenants))
	}
}

func TestPostgresDB_ListAllTenants_CountError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT COUNT").WillReturnError(errors.New("count fail"))

	_, _, err := pg.ListAllTenants(context.Background(), 10, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAllTenants_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(countRows)
	mock.ExpectQuery("SELECT .+ FROM tenants ORDER BY created_at DESC").
		WithArgs(10, 0).
		WillReturnError(errors.New("query fail"))

	_, _, err := pg.ListAllTenants(context.Background(), 10, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPostgresDB_ListAllTenants_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(countRows)
	tenantRows := sqlmock.NewRows([]string{"id"}).AddRow("t1")
	mock.ExpectQuery("SELECT .+ FROM tenants ORDER BY created_at DESC").
		WithArgs(10, 0).
		WillReturnRows(tenantRows)

	_, _, err := pg.ListAllTenants(context.Background(), 10, 0)
	if err == nil {
		t.Fatal("expected scan error")
	}
}

// =====================================================
// NewPostgres constructor (integration-level test with mock)
// =====================================================

func TestNewPostgres_Success(t *testing.T) {
	registerTestPostgresDriver()
	testPgDriver.openFunc = func(name string) (driver.Conn, error) {
		return &testConn{}, nil
	}
	t.Cleanup(func() { testPgDriver.openFunc = nil })

	pg, err := NewPostgres("test-success-dsn")
	if err != nil {
		t.Fatalf("NewPostgres() error = %v", err)
	}
	pg.Close()
}

func TestNewPostgres_PingFail(t *testing.T) {
	registerTestPostgresDriver()
	testPgDriver.openFunc = func(name string) (driver.Conn, error) {
		return &testConn{pingErr: errors.New("ping failed")}, nil
	}
	t.Cleanup(func() { testPgDriver.openFunc = nil })

	_, err := NewPostgres("test-ping-fail-dsn")
	if err == nil {
		t.Fatal("expected ping error from NewPostgres")
	}
}

func TestNewPostgres_MigrateError(t *testing.T) {
	registerTestPostgresDriver()
	testPgDriver.openFunc = func(name string) (driver.Conn, error) {
		return &testConn{execErr: errors.New("migrate fail")}, nil
	}
	t.Cleanup(func() { testPgDriver.openFunc = nil })

	_, err := NewPostgres("test-migrate-fail-dsn")
	if err == nil {
		t.Fatal("expected migrate error")
	}
}

func TestNewPostgres_PingError(t *testing.T) {
	// Test via the struct directly since controlling ping failure
	// through the registered driver is complex.
	pg, mock := newMockPostgresWithPing(t)
	mock.ExpectPing().WillReturnError(errors.New("connection refused"))
	if err := pg.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error")
	}
}

// =====================================================
// Interface compliance (compile-time check)
// =====================================================

func TestPostgresDB_ImplementsStore(t *testing.T) {
	// Already checked by var _ core.Store = (*PostgresDB)(nil) in postgres.go,
	// but this test ensures it holds at runtime too.
	var store core.Store = &PostgresDB{}
	if store == nil {
		t.Fatal("PostgresDB should implement core.Store")
	}
}

// =====================================================
// Empty results for list methods
// =====================================================

func TestPostgresDB_ListAppsByTenant_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery("SELECT COUNT").WithArgs("t1").WillReturnRows(countRows)

	appRows := sqlmock.NewRows([]string{
		"id", "project_id", "tenant_id", "name", "type", "source_type", "source_url", "branch",
		"status", "replicas", "created_at", "updated_at",
	})
	mock.ExpectQuery("SELECT .+ FROM applications WHERE tenant_id").
		WithArgs("t1", 10, 0).
		WillReturnRows(appRows)

	apps, total, err := pg.ListAppsByTenant(context.Background(), "t1", 10, 0)
	if err != nil {
		t.Fatalf("ListAppsByTenant() error = %v", err)
	}
	if total != 0 || len(apps) != 0 {
		t.Fatalf("expected empty, got total=%d len=%d", total, len(apps))
	}
}

func TestPostgresDB_ListAppsByProject_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	appRows := sqlmock.NewRows([]string{
		"id", "project_id", "tenant_id", "name", "type", "source_type", "source_url", "branch",
		"status", "replicas", "created_at", "updated_at",
	})
	mock.ExpectQuery("SELECT .+ FROM applications WHERE project_id = \\$1").
		WithArgs("p1").
		WillReturnRows(appRows)

	apps, err := pg.ListAppsByProject(context.Background(), "p1")
	if err != nil {
		t.Fatalf("ListAppsByProject() error = %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("expected empty, got %d", len(apps))
	}
}

func TestPostgresDB_ListDeploymentsByApp_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{
		"id", "app_id", "version", "image", "container_id", "status",
		"commit_sha", "commit_message", "triggered_by", "strategy",
		"started_at", "finished_at", "created_at",
	})
	mock.ExpectQuery("SELECT .+ FROM deployments WHERE app_id = \\$1").
		WithArgs("a1", 10).
		WillReturnRows(rows)

	deps, err := pg.ListDeploymentsByApp(context.Background(), "a1", 10)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("expected empty, got %d", len(deps))
	}
}

func TestPostgresDB_ListDomainsByApp_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id", "app_id", "fqdn", "type", "dns_provider", "dns_synced", "verified", "created_at"})
	mock.ExpectQuery("SELECT .+ FROM domains WHERE app_id = \\$1").
		WithArgs("a1").
		WillReturnRows(rows)

	domains, err := pg.ListDomainsByApp(context.Background(), "a1")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(domains) != 0 {
		t.Fatalf("expected empty, got %d", len(domains))
	}
}

func TestPostgresDB_ListAllDomains_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id", "app_id", "fqdn", "type", "dns_provider", "dns_synced", "verified", "created_at"})
	mock.ExpectQuery("SELECT .+ FROM domains ORDER BY created_at").
		WillReturnRows(rows)

	domains, err := pg.ListAllDomains(context.Background())
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(domains) != 0 {
		t.Fatalf("expected empty, got %d", len(domains))
	}
}

func TestPostgresDB_ListProjectsByTenant_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id", "tenant_id", "name", "description", "environment", "created_at", "updated_at"})
	mock.ExpectQuery("SELECT .+ FROM projects WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnRows(rows)

	projects, err := pg.ListProjectsByTenant(context.Background(), "t1")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected empty, got %d", len(projects))
	}
}

func TestPostgresDB_ListRoles_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{"id", "tenant_id", "name", "description", "permissions_json", "is_builtin", "created_at"})
	mock.ExpectQuery("SELECT .+ FROM roles WHERE tenant_id = \\$1 OR is_builtin = TRUE").
		WithArgs("t1").
		WillReturnRows(rows)

	roles, err := pg.ListRoles(context.Background(), "t1")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("expected empty, got %d", len(roles))
	}
}

func TestPostgresDB_ListAuditLogs_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery("SELECT COUNT").WithArgs("t1").WillReturnRows(countRows)

	logRows := sqlmock.NewRows([]string{
		"id", "tenant_id", "user_id", "action", "resource_type", "resource_id",
		"details_json", "ip_address", "user_agent", "created_at",
	})
	mock.ExpectQuery("SELECT .+ FROM audit_log WHERE tenant_id = \\$1").
		WithArgs("t1", 10, 0).
		WillReturnRows(logRows)

	entries, total, err := pg.ListAuditLogs(context.Background(), "t1", 10, 0)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if total != 0 || len(entries) != 0 {
		t.Fatalf("expected empty, got total=%d len=%d", total, len(entries))
	}
}

func TestPostgresDB_ListSecretsByTenant_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{
		"id", "tenant_id", "project_id", "app_id", "name", "type",
		"description", "scope", "current_version", "created_at", "updated_at",
	})
	mock.ExpectQuery("SELECT .+ FROM secrets WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnRows(rows)

	secrets, err := pg.ListSecretsByTenant(context.Background(), "t1")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("expected empty, got %d", len(secrets))
	}
}

func TestPostgresDB_ListInvitesByTenant_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rows := sqlmock.NewRows([]string{
		"id", "tenant_id", "email", "role_id", "invited_by", "token_hash",
		"expires_at", "accepted_at", "status", "created_at",
	})
	mock.ExpectQuery("SELECT .+ FROM invitations WHERE tenant_id = \\$1").
		WithArgs("t1").
		WillReturnRows(rows)

	invites, err := pg.ListInvitesByTenant(context.Background(), "t1")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(invites) != 0 {
		t.Fatalf("expected empty, got %d", len(invites))
	}
}

func TestPostgresDB_ListAllTenants_Empty(t *testing.T) {
	pg, mock := newMockPostgres(t)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(countRows)

	tenantRows := sqlmock.NewRows([]string{
		"id", "name", "slug", "avatar_url", "plan_id", "owner_id",
		"status", "limits_json", "metadata_json", "created_at", "updated_at",
	})
	mock.ExpectQuery("SELECT .+ FROM tenants ORDER BY created_at DESC").
		WithArgs(10, 0).
		WillReturnRows(tenantRows)

	tenants, total, err := pg.ListAllTenants(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if total != 0 || len(tenants) != 0 {
		t.Fatalf("expected empty, got total=%d len=%d", total, len(tenants))
	}
}
