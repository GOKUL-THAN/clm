package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"github.com/m2p/clm/pkg/satelliteauth"
)

// These are integration tests: they need a real PostgreSQL with the CLM schema
// applied, because the bugs they guard against are NULL-handling bugs that only
// a real driver reproduces. A mock would have happily returned zero values and
// hidden every one of them.
//
// Run with deploy/run-tests.sh, which provisions a scratch database. Without
// CLM_TEST_DB_DSN the whole package is skipped rather than failed, so `go test`
// stays usable where no database is available.

var testDB *DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("CLM_TEST_DB_DSN")
	if dsn == "" {
		// Exiting 0 keeps `go test ./...` usable without a database, but the
		// package would otherwise report a silent "ok" having run nothing — a
		// false green. Say so loudly, and fail outright when CI demands
		// coverage by setting CLM_TEST_DB_REQUIRED.
		if os.Getenv("CLM_TEST_DB_REQUIRED") != "" {
			panic("CLM_TEST_DB_REQUIRED is set but CLM_TEST_DB_DSN is empty: " +
				"the repository integration tests cannot run")
		}
		println("WARNING: CLM_TEST_DB_DSN not set — PostgreSQL integration tests will be skipped")
		code := m.Run()
		os.Exit(code)
	}

	raw, err := sql.Open("postgres", dsn)
	if err != nil {
		panic("open test database: " + err.Error())
	}
	if err := raw.Ping(); err != nil {
		panic("ping test database: " + err.Error())
	}
	testDB = &DB{DB: raw}

	code := m.Run()
	_ = raw.Close()
	os.Exit(code)
}

func skipIfNoDB(t *testing.T) {
	t.Helper()
	if testDB == nil {
		t.Skip("CLM_TEST_DB_DSN not set")
	}
}

// seedTenant inserts a tenant and removes it (and anything referencing it) when
// the test finishes.
func seedTenant(t *testing.T, ctx context.Context, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	provisionCtx := core.WithTenantProvisioning(ctx)
	if err := testDB.WithTenantProvisioningContext(provisionCtx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tenants (id, name, slug, plan, status, max_certs, max_satellites,
			    max_users, sso_enabled, data_region)
			 VALUES ($1, $2, $3, 'free', 'active', 100, 2, 5, false, 'ap-south-1')`,
			id, name, name)
		return err
	}); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		platformCtx := core.WithPlatformAccess(context.Background())
		_ = testDB.WithPlatformContext(platformCtx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM satellite_commands WHERE tenant_id = $1`, id)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM satellites WHERE tenant_id = $1`, id)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM cloud_account_idempotency WHERE tenant_id = $1`, id)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM cloud_accounts WHERE tenant_id = $1`, id)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM users WHERE tenant_id = $1`, id)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM teams WHERE tenant_id = $1`, id)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, id)
			return err
		})
	})
	return id
}

func TestUserRepositoryEnforcesTenantScope(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantA := seedTenant(t, ctx, "user-scope-a-"+uuid.NewString()[:8])
	tenantB := seedTenant(t, ctx, "user-scope-b-"+uuid.NewString()[:8])
	ctxA := core.WithTenantID(ctx, tenantA)
	ctxB := core.WithTenantID(ctx, tenantB)
	repo := NewUserRepository(testDB)
	userA := &models.User{
		ID: uuid.New(), TenantID: tenantA, Email: uuid.NewString() + "@example.invalid",
		Name: "Tenant A User", Role: models.RoleViewer, Status: "active",
		PasswordHash: "test-only", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	userB := &models.User{
		ID: uuid.New(), TenantID: tenantB, Email: uuid.NewString() + "@example.invalid",
		Name: "Tenant B User", Role: models.RoleViewer, Status: "active",
		PasswordHash: "test-only", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	for _, row := range []struct {
		ctx  context.Context
		user *models.User
	}{{ctxA, userA}, {ctxB, userB}} {
		if err := repo.Create(row.ctx, row.user); err != nil {
			t.Fatalf("create tenant user: %v", err)
		}
	}

	if _, err := repo.GetByIDForTenant(ctxA, tenantA, userA.ID); err != nil {
		t.Fatalf("read user in owning tenant: %v", err)
	}
	if _, err := repo.GetByIDForTenant(ctxA, tenantB, userA.ID); err == nil {
		t.Fatal("cross-tenant user lookup was accepted without a platform context")
	}

	users, err := repo.ListByTenant(ctxA, tenantA)
	if err != nil {
		t.Fatalf("list users in tenant A: %v", err)
	}
	if len(users) != 1 || users[0].ID != userA.ID || users[0].TenantID != tenantA {
		t.Fatalf("tenant A user list = %+v, want only user %s", users, userA.ID)
	}

	crossTenantUpdate := *userA
	crossTenantUpdate.Name = "Unauthorized cross-tenant update"
	if err := repo.UpdateForTenant(ctxA, tenantB, &crossTenantUpdate); err == nil {
		t.Fatal("cross-tenant user update was accepted without a platform context")
	}
	if err := repo.DeleteForTenant(ctxA, tenantB, userA.ID); err == nil {
		t.Fatal("cross-tenant user delete was accepted without a platform context")
	}

	stillOwned, err := repo.GetByIDForTenant(ctxA, tenantA, userA.ID)
	if err != nil {
		t.Fatalf("read user after rejected cross-tenant operations: %v", err)
	}
	if stillOwned.Name != userA.Name {
		t.Fatalf("cross-tenant update changed user name to %q", stillOwned.Name)
	}

	userA.Name = "Updated Tenant A User"
	userA.UpdatedAt = time.Now().UTC()
	if err := repo.UpdateForTenant(ctxA, tenantA, userA); err != nil {
		t.Fatalf("update user in owning tenant: %v", err)
	}
	updated, err := repo.GetByIDForTenant(ctxA, tenantA, userA.ID)
	if err != nil || updated.Name != userA.Name {
		t.Fatalf("read updated tenant A user = %+v, error = %v", updated, err)
	}
	if err := repo.DeleteForTenant(ctxA, tenantA, userA.ID); err != nil {
		t.Fatalf("delete user in owning tenant: %v", err)
	}
	if _, err := repo.GetByIDForTenant(ctxA, tenantA, userA.ID); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("read deleted user error = %v, want ErrUserNotFound", err)
	}
}

// TestUserRepo_GetByEmail_NullPasswordHash guards the bug that mattered most:
// password_hash is nullable (an SSO-provisioned user has no local password) but
// models.User.PasswordHash is a plain string. Before COALESCE, reading such a
// user failed with "converting NULL to string is unsupported" — and because the
// login path reported every repository error as invalid credentials, the real
// cause was invisible.
func TestUserRepo_GetByEmail_NullPasswordHash(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	repo := NewUserRepository(testDB)

	tenantID := seedTenant(t, ctx, "null-pw-"+uuid.NewString()[:8])
	email := "sso-only-" + uuid.NewString()[:8] + "@aiforgers.tech"

	// password_hash deliberately omitted, so it is NULL.
	if err := testDB.WithTenantContext(core.WithTenantID(ctx, tenantID), tenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(core.WithTenantID(ctx, tenantID),
			`INSERT INTO users (id, tenant_id, email, name, role, status, mfa_enabled)
			 VALUES ($1, $2, $3, 'SSO Only', 'viewer', 'active', false)`,
			uuid.New(), tenantID, email)
		return err
	}); err != nil {
		t.Fatalf("insert passwordless user: %v", err)
	}

	user, err := repo.GetByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetByEmail on a NULL password_hash must succeed, got: %v", err)
	}
	if user == nil {
		t.Fatal("expected a user, got nil")
	}
	if user.PasswordHash != "" {
		t.Errorf("expected empty PasswordHash, got %q", user.PasswordHash)
	}
	if user.Email != email {
		t.Errorf("email = %q, want %q", user.Email, email)
	}
}

// TestUserRepo_GetByEmail_NotFound asserts a genuine miss is reported as
// core.ErrNotFound and yields no partially-populated struct.
func TestUserRepo_GetByEmail_NotFound(t *testing.T) {
	skipIfNoDB(t)
	repo := NewUserRepository(testDB)

	user, err := repo.GetByEmail(context.Background(), "definitely-absent@example.invalid")
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("expected core.ErrNotFound, got %v", err)
	}
	if user != nil {
		t.Errorf("expected nil user alongside error, got %+v", user)
	}
}

// TestTenantRepo_GetByID_NullSSOProvider guards the original bug: sso_provider is
// nullable, List never selected it so listing worked, and only the single-row
// read failed — which surfaced as a 404 for a tenant that plainly existed.
func TestTenantRepo_GetByID_NullSSOProvider(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	platformCtx := core.WithPlatformAccess(ctx)
	repo := NewTenantRepository(testDB)

	tenantID := seedTenant(t, ctx, "null-sso-"+uuid.NewString()[:8])

	// Make the nullable column explicitly NULL.
	if err := testDB.WithPlatformContext(core.WithPlatformAccess(ctx), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tenants SET sso_provider = NULL WHERE id = $1`, tenantID)
		return err
	}); err != nil {
		t.Fatalf("null out sso_provider: %v", err)
	}

	tenant, err := repo.GetByID(platformCtx, tenantID)
	if err != nil {
		t.Fatalf("GetByID on a NULL sso_provider must succeed, got: %v", err)
	}
	if tenant.SSOProvider != "" {
		t.Errorf("expected empty SSOProvider, got %q", tenant.SSOProvider)
	}

	// List must agree with the single-row read — the two drifted before.
	all, err := repo.List(platformCtx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found bool
	for _, x := range all {
		if x.ID == tenantID {
			found = true
		}
	}
	if !found {
		t.Error("tenant present via GetByID but missing from List")
	}
}

func TestTenantRepo_GetByID_NotFound(t *testing.T) {
	skipIfNoDB(t)
	repo := NewTenantRepository(testDB)

	tenant, err := repo.GetByID(core.WithPlatformAccess(context.Background()), uuid.New())
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("expected core.ErrNotFound, got %v", err)
	}
	if tenant != nil {
		t.Errorf("expected nil tenant alongside error, got %+v", tenant)
	}
}

// TestTenantRepo_CountDependents_And_Delete covers the delete guard: a tenant
// owning rows must not be removable, and the reported counts are what the API
// shows the operator.
func TestTenantRepo_CountDependents_And_Delete(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	platformCtx := core.WithPlatformAccess(ctx)
	repo := NewTenantRepository(testDB)

	tenantID := seedTenant(t, ctx, "deps-"+uuid.NewString()[:8])

	// Empty tenant: no dependents, delete succeeds.
	deps, err := repo.CountDependents(platformCtx, tenantID)
	if err != nil {
		t.Fatalf("CountDependents: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected no dependents on a fresh tenant, got %v", deps)
	}

	// Attach a user, and the tenant must now report exactly that.
	if err := testDB.WithTenantContext(core.WithTenantID(ctx, tenantID), tenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(core.WithTenantID(ctx, tenantID),
			`INSERT INTO users (id, tenant_id, email, name, role, status, mfa_enabled, password_hash)
			 VALUES ($1, $2, $3, 'Dep User', 'viewer', 'active', false, 'x')`,
			uuid.New(), tenantID, "dep-"+uuid.NewString()[:8]+"@aiforgers.tech")
		return err
	}); err != nil {
		t.Fatalf("insert dependent user: %v", err)
	}

	deps, err = repo.CountDependents(platformCtx, tenantID)
	if err != nil {
		t.Fatalf("CountDependents: %v", err)
	}
	if deps["users"] != 1 {
		t.Errorf("expected users=1 in dependents, got %v", deps)
	}

	auditID := uuid.New()
	if err := testDB.WithTenantContext(core.WithTenantID(ctx, tenantID), tenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(core.WithTenantID(ctx, tenantID),
			`INSERT INTO audit_log (id, tenant_id, action, resource_type) VALUES ($1, $2, 'test', 'tenant')`,
			auditID, tenantID)
		return err
	}); err != nil {
		t.Fatalf("insert immutable audit record: %v", err)
	}
	if err := repo.DeleteCascade(platformCtx, tenantID); err == nil {
		t.Fatal("DeleteCascade removed a tenant with immutable audit records")
	}
	if err := testDB.WithPlatformContext(core.WithPlatformAccess(ctx), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM audit_log WHERE id = $1`, auditID)
		return err
	}); err != nil {
		t.Fatalf("clean up immutable audit fixture: %v", err)
	}

	// A plain delete must fail while the foreign key still points at it.
	if err := repo.Delete(platformCtx, tenantID); err == nil {
		t.Error("Delete should fail while dependents exist")
	}

	// Cascade removes the dependents and the tenant together.
	if err := repo.DeleteCascade(platformCtx, tenantID); err != nil {
		t.Fatalf("DeleteCascade: %v", err)
	}
	if _, err := repo.GetByID(platformCtx, tenantID); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("tenant should be gone, got %v", err)
	}

	var remaining int
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM users WHERE tenant_id = $1`, tenantID).Scan(&remaining); err != nil {
		t.Fatalf("count remaining users: %v", err)
	}
	if remaining != 0 {
		t.Errorf("cascade left %d orphaned users", remaining)
	}
}

func TestTenantRepo_DeleteMissing(t *testing.T) {
	skipIfNoDB(t)
	err := NewTenantRepository(testDB).Delete(core.WithPlatformAccess(context.Background()), uuid.New())
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("expected core.ErrNotFound deleting an absent tenant, got %v", err)
	}
}

func TestSatelliteRepo_GetByID_NotFound(t *testing.T) {
	skipIfNoDB(t)
	sat, err := NewSatelliteRepository(testDB).GetByID(context.Background(), uuid.New())
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("expected core.ErrNotFound, got %v", err)
	}
	if sat != nil {
		t.Errorf("expected nil satellite alongside error, got %+v", sat)
	}
}

// TestTenantRepo_UpdateRoundTrip guards plan-limit rewriting, which is what
// silently changed a tenant's limits when its seeded values did not match the
// plan tier.
func TestTenantRepo_UpdateRoundTrip(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	platformCtx := core.WithPlatformAccess(ctx)
	repo := NewTenantRepository(testDB)

	tenantID := seedTenant(t, ctx, "upd-"+uuid.NewString()[:8])
	tenant, err := repo.GetByID(platformCtx, tenantID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	tenant.Name = "Renamed Org"
	tenant.Plan = "enterprise"
	tenant.MaxCerts = 100000
	tenant.UpdatedAt = time.Now()
	if err := repo.Update(platformCtx, tenant); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(platformCtx, tenantID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "Renamed Org" || got.Plan != "enterprise" || got.MaxCerts != 100000 {
		t.Errorf("update did not round-trip: %+v", got)
	}
}

func TestControlPlaneAuditEventsAreAppendOnly(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantID := uuid.New()
	provisionCtx := core.WithTenantProvisioning(ctx)
	if err := testDB.WithTenantProvisioningContext(provisionCtx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tenants (id, name, slug, plan, status, max_certs, max_satellites, max_users, sso_enabled, data_region)
			 VALUES ($1, $2, $3, 'free', 'active', 100, 2, 5, false, 'ap-south-1')`,
			tenantID, "audit-event-tenant", "audit-event-tenant")
		return err
	}); err != nil {
		t.Fatalf("seed audit tenant: %v", err)
	}
	id := uuid.New()
	tenantCtx := core.WithTenantID(ctx, tenantID)
	if err := testDB.WithTenantContext(tenantCtx, tenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(tenantCtx, `INSERT INTO control_plane_audit_events (id,tenant_id,request_id,actor_id,actor_role,action,resource_type,outcome,source_ip) VALUES ($1,$2,'request',$3,'tenant_admin','POST /api/v1/users','users','success','127.0.0.1')`, id, tenantID, uuid.New())
		return err
	}); err != nil {
		t.Fatalf("insert audit event: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, `UPDATE control_plane_audit_events SET outcome='failure' WHERE id=$1`, id); err == nil {
		t.Fatal("append-only trigger allowed UPDATE")
	}
	if _, err := testDB.ExecContext(ctx, `DELETE FROM control_plane_audit_events WHERE id=$1`, id); err == nil {
		t.Fatal("append-only trigger allowed DELETE")
	}
}

func TestCloudAccountIdempotencyCascadesOnDelete(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, ctx, "cloud-"+uuid.NewString()[:8])
	accountID := uuid.New()
	tenantCtx := core.WithTenantID(ctx, tenantID)
	if err := testDB.WithTenantContext(tenantCtx, tenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(tenantCtx, `INSERT INTO cloud_accounts (id,tenant_id,name,provider,encryption_version,credential_nonce,credential_ciphertext) VALUES ($1,$2,'primary','aws',1,$3,$4)`, accountID, tenantID, []byte("123456789012"), []byte("ciphertext"))
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(tenantCtx, `INSERT INTO cloud_account_idempotency (tenant_id,idempotency_key,request_hash,cloud_account_id) VALUES ($1,'request-key','hash',$2)`, tenantID, accountID)
		return err
	}); err != nil {
		t.Fatalf("insert cloud account/idempotency: %v", err)
	}
	if err := testDB.WithTenantContext(core.WithTenantID(ctx, tenantID), tenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(core.WithTenantID(ctx, tenantID), `DELETE FROM cloud_accounts WHERE tenant_id=$1 AND id=$2`, tenantID, accountID)
		return err
	}); err != nil {
		t.Fatalf("delete cloud account: %v", err)
	}
	var remaining int
	if err := testDB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_account_idempotency WHERE tenant_id=$1`, tenantID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("idempotency delete cascade left %d rows", remaining)
	}
}

func TestCloudAccountCreateIdempotentConcurrentReplay(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, ctx, "cloud-race-"+uuid.NewString()[:8])
	ctx = core.WithTenantID(ctx, tenantID)
	repo := NewCloudAccountRepository(testDB)
	type result struct {
		existing *models.EncryptedCloudCredentials
		hash     string
		err      error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			now := time.Now().UTC()
			account := &models.EncryptedCloudCredentials{
				CloudAccount:      models.CloudAccount{ID: uuid.New(), TenantID: tenantID, Name: "primary", Provider: "aws", CredentialVersion: 1, CreatedAt: now, UpdatedAt: now},
				EncryptionVersion: 1, Nonce: []byte("123456789012"), Ciphertext: []byte("ciphertext"),
			}
			existing, hash, err := repo.CreateIdempotent(ctx, account, "same-key", "same-hash")
			results <- result{existing: existing, hash: hash, err: err}
		}()
	}
	wg.Wait()
	close(results)
	created, replayed := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("CreateIdempotent() error = %v", result.err)
		}
		if result.existing == nil {
			created++
		} else if result.hash == "same-hash" {
			replayed++
		}
	}
	if created != 1 || replayed != 1 {
		t.Fatalf("created=%d replayed=%d, want one each", created, replayed)
	}
	var count int
	if err := testDB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_accounts WHERE tenant_id=$1`, tenantID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent replay created %d accounts, want 1", count)
	}
}

func seedSatellite(t *testing.T, ctx context.Context, tenantID uuid.UUID) (*SatelliteRepository, *models.Satellite) {
	t.Helper()
	repo := NewSatelliteRepository(testDB)
	now := time.Now().UTC()
	sat := &models.Satellite{ID: uuid.New(), TenantID: tenantID, Name: "sat-" + uuid.NewString()[:8], Type: models.SatelliteLocal, Status: models.SatelliteRegistering, ConnectorMode: models.ConnectorDirect, SecretsBackend: "vault", CreatedAt: now, UpdatedAt: now}
	if err := repo.Create(core.WithTenantID(ctx, tenantID), sat); err != nil {
		t.Fatalf("create satellite: %v", err)
	}
	return repo, sat
}

func TestSatelliteCredentialHashVerifyAndBinding(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, ctx, "sat-auth-"+uuid.NewString()[:8])
	ctx = core.WithTenantID(ctx, tenantID)
	repo, sat := seedSatellite(t, ctx, tenantID)
	plaintext, encoded, err := satelliteauth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetCredentialForTenant(ctx, tenantID, sat.ID, encoded); err != nil {
		t.Fatal(err)
	}
	identity, err := repo.AuthenticateSatellite(ctx, sat.ID, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if identity.TenantID != tenantID || identity.SatelliteID != sat.ID {
		t.Fatalf("identity=%+v", identity)
	}
	if _, err := repo.AuthenticateSatellite(ctx, sat.ID, "wrong"); !errors.Is(err, ErrSatelliteAuth) {
		t.Fatalf("wrong credential error=%v", err)
	}
	if _, err := repo.AuthenticateSatellite(ctx, uuid.New(), plaintext); !errors.Is(err, ErrSatelliteAuth) {
		t.Fatalf("wrong route satellite error=%v", err)
	}
	if err := repo.RevokeCredentialForTenant(ctx, tenantID, sat.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AuthenticateSatellite(ctx, sat.ID, plaintext); !errors.Is(err, ErrSatelliteAuth) {
		t.Fatalf("revoked credential error=%v", err)
	}
}

func TestCommandIdempotencyClaimLeaseAndResult(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, ctx, "cmd-"+uuid.NewString()[:8])
	ctx = core.WithTenantID(ctx, tenantID)
	repo, sat := seedSatellite(t, ctx, tenantID)
	command := &models.SatelliteCommand{ID: uuid.New(), TenantID: tenantID, SatelliteID: sat.ID, IdempotencyKey: "stable-key", Type: "discover", Payload: `{}`, Status: models.CommandQueued, MaxAttempts: 3}
	created, wasCreated, err := repo.QueueCommandIdempotent(ctx, tenantID, command)
	if err != nil || !wasCreated {
		t.Fatalf("first queue created=%v err=%v", wasCreated, err)
	}
	replay, replayCreated, err := repo.QueueCommandIdempotent(ctx, tenantID, &models.SatelliteCommand{ID: uuid.New(), SatelliteID: sat.ID, IdempotencyKey: "stable-key", Type: "discover", Payload: `{}`})
	if err != nil || replayCreated || replay.ID != created.ID {
		t.Fatalf("replay=%+v created=%v err=%v", replay, replayCreated, err)
	}
	if _, _, err := repo.QueueCommandIdempotent(ctx, tenantID, &models.SatelliteCommand{ID: uuid.New(), SatelliteID: sat.ID, IdempotencyKey: "stable-key", Type: "deploy_cert", Payload: `{"target":"other"}`}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("mismatched idempotency replay error=%v", err)
	}

	var wg sync.WaitGroup
	claims := make(chan []*models.SatelliteCommand, 2)
	for _, owner := range []string{"worker-a", "worker-b"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			got, e := repo.ClaimCommands(ctx, sat.ID, owner, 1, time.Minute)
			if e != nil {
				t.Errorf("claim: %v", e)
			}
			claims <- got
		}(owner)
	}
	wg.Wait()
	close(claims)
	var claimed *models.SatelliteCommand
	count := 0
	for batch := range claims {
		count += len(batch)
		if len(batch) == 1 {
			claimed = batch[0]
		}
	}
	if count != 1 || claimed == nil || claimed.LeaseToken == nil {
		t.Fatalf("exclusive claims=%d command=%+v", count, claimed)
	}
	if err := repo.AcknowledgeCommand(ctx, sat.ID, claimed.ID, uuid.New(), time.Minute); !errors.Is(err, ErrInvalidLease) {
		t.Fatalf("wrong ack token error=%v", err)
	}
	if err := repo.AcknowledgeCommand(ctx, sat.ID, claimed.ID, *claimed.LeaseToken, time.Minute); err != nil {
		t.Fatal(err)
	}
	completed, err := repo.CompleteCommand(ctx, sat.ID, claimed.ID, *claimed.LeaseToken, models.CommandSucceeded, "ok", "", false, 0)
	if err != nil || completed.Status != models.CommandSucceeded {
		t.Fatalf("complete=%+v err=%v", completed, err)
	}
	duplicate, err := repo.CompleteCommand(ctx, sat.ID, claimed.ID, *claimed.LeaseToken, models.CommandSucceeded, "ok", "", false, 0)
	if err != nil || duplicate.Status != models.CommandSucceeded {
		t.Fatalf("duplicate result=%+v err=%v", duplicate, err)
	}
}

func TestCommandLeaseExpiryAndDeadLetter(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, ctx, "lease-"+uuid.NewString()[:8])
	ctx = core.WithTenantID(ctx, tenantID)
	repo, sat := seedSatellite(t, ctx, tenantID)
	cmd := &models.SatelliteCommand{SatelliteID: sat.ID, IdempotencyKey: "expires", Type: "discover", Payload: `{}`, MaxAttempts: 2}
	if err := repo.QueueCommandForTenant(ctx, tenantID, cmd); err != nil {
		t.Fatal(err)
	}
	first, err := repo.ClaimCommands(ctx, sat.ID, "first", 1, 20*time.Millisecond)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim=%d err=%v", len(first), err)
	}
	time.Sleep(35 * time.Millisecond)
	if _, err := repo.CompleteCommand(ctx, sat.ID, first[0].ID, *first[0].LeaseToken, models.CommandSucceeded, "late", "", false, 0); !errors.Is(err, ErrInvalidLease) {
		t.Fatalf("expired lease result error=%v", err)
	}
	second, err := repo.ClaimCommands(ctx, sat.ID, "second", 1, time.Minute)
	if err != nil || len(second) != 1 || second[0].Attempts != 2 {
		t.Fatalf("expired claim=%+v err=%v", second, err)
	}
	dead := &models.SatelliteCommand{SatelliteID: sat.ID, IdempotencyKey: "dead", Type: "discover", Payload: `{}`, MaxAttempts: 1}
	if err := repo.QueueCommandForTenant(ctx, tenantID, dead); err != nil {
		t.Fatal(err)
	}
	leased, err := repo.ClaimCommands(ctx, sat.ID, "dead-worker", 1, 20*time.Millisecond)
	if err != nil || len(leased) != 1 {
		t.Fatalf("dead claim=%+v err=%v", leased, err)
	}
	time.Sleep(35 * time.Millisecond)
	if _, err := repo.ClaimCommands(ctx, sat.ID, "next", 10, time.Minute); err != nil {
		t.Fatal(err)
	}
	var status models.CommandStatus
	if err := testDB.QueryRowContext(ctx, `SELECT status FROM satellite_commands WHERE id=$1`, dead.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != models.CommandDeadLetter {
		t.Fatalf("status=%s want dead_letter", status)
	}
}

func TestRetryableCommandResultRequeuesAndGetsNewAttempt(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, ctx, "retry-"+uuid.NewString()[:8])
	ctx = core.WithTenantID(ctx, tenantID)
	repo, sat := seedSatellite(t, ctx, tenantID)
	cmd := &models.SatelliteCommand{SatelliteID: sat.ID, IdempotencyKey: "retryable", Type: "discover", Payload: `{}`, MaxAttempts: 2}
	if err := repo.QueueCommandForTenant(ctx, tenantID, cmd); err != nil {
		t.Fatal(err)
	}
	first, err := repo.ClaimCommands(ctx, sat.ID, "first", 1, time.Minute)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim=%+v err=%v", first, err)
	}
	requeued, err := repo.CompleteCommand(ctx, sat.ID, first[0].ID, *first[0].LeaseToken, models.CommandFailed, "", "temporary", true, time.Millisecond)
	if err != nil || requeued.Status != models.CommandQueued || requeued.TerminalAt != nil {
		t.Fatalf("requeued=%+v err=%v", requeued, err)
	}
	time.Sleep(5 * time.Millisecond)
	second, err := repo.ClaimCommands(ctx, sat.ID, "second", 1, time.Minute)
	if err != nil || len(second) != 1 || second[0].Attempts != 2 || second[0].ExecutionGeneration != 0 || second[0].LeaseToken == nil || *second[0].LeaseToken == *first[0].LeaseToken {
		t.Fatalf("second claim=%+v err=%v", second, err)
	}
	dead, err := repo.CompleteCommand(ctx, sat.ID, second[0].ID, *second[0].LeaseToken, models.CommandFailed, "", "still failing", true, time.Millisecond)
	if err != nil || dead.Status != models.CommandDeadLetter || dead.TerminalAt == nil {
		t.Fatalf("dead-letter result=%+v err=%v", dead, err)
	}
	if err := repo.RetryCommandForTenant(ctx, tenantID, sat.ID, dead.ID); err != nil {
		t.Fatalf("operator retry: %v", err)
	}
	third, err := repo.ClaimCommands(ctx, sat.ID, "third", 1, time.Minute)
	if err != nil || len(third) != 1 || third[0].Attempts != 1 || third[0].ExecutionGeneration != 1 {
		t.Fatalf("operator retry claim=%+v err=%v", third, err)
	}
}
