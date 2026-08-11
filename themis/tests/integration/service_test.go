// Package integration contains end-to-end tests that exercise Themis against a
// real PostgreSQL started with testcontainers.
package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mathif92/olympus/themis/pkg"
	"github.com/mathif92/olympus/themis/pkg/database"
)

const docAllowS3 = `{"Version":"2012-10-17","Statement":[{"Sid":"ListAndGet","Effect":"Allow","Action":["s3:GetObject","s3:ListBucket"],"Resource":["arn:aws:s3:::assets/*"]}]}`

const docDenySensitive = `{"Version":"2012-10-17","Statement":[{"Sid":"DenySecret","Effect":"Deny","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::assets/secrets/*"]}]}`

// startPostgres boots a real Postgres, applies the goose migrations, and
// returns a ready database.Client plus a cleanup func.
func startPostgres(t *testing.T) (*database.Client, func()) {
	t.Helper()

	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("olympus_themis"),
		postgres.WithUsername("olympus"),
		postgres.WithPassword("olympus_secret"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	url, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}

	client, err := database.NewClient(database.Config{
		PostgresURL: url,
		PoolMax:     10,
		PoolMin:     2,
		PoolTimeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("new database client: %v", err)
	}

	dir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	if err := database.Migrate(client.DB, dir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	stop := func() {
		client.Close()
		_ = pg.Terminate(context.Background())
	}
	return client, stop
}

func newStore(t *testing.T) (*pkg.ThemisStore, func()) {
	t.Helper()
	client, stop := startPostgres(t)
	return pkg.NewThemisStore(client, pkg.NewJWT("integration-test-secret")), stop
}

func seedProject(t *testing.T, store *pkg.ThemisStore, account, project string) {
	t.Helper()
	ctx := context.Background()
	if err := store.EnsureAccount(ctx, database.Account{ID: account, DisplayName: account, Email: account + "@t.dev", Plan: "pro"}); err != nil {
		t.Fatalf("ensure account: %v", err)
	}
	if err := store.CreateProject(ctx, account, database.Project{Name: project}); err != nil {
		t.Fatalf("create project: %v", err)
	}
}

func TestTenantIsolation(t *testing.T) {
	store, stop := newStore(t)
	defer stop()
	ctx := context.Background()

	seedProject(t, store, "tenant-a", "platform")
	seedProject(t, store, "tenant-b", "platform")

	if _, err := store.CreateUser(ctx, "tenant-a", "platform", pkg.UserInput{Name: "svc-a"}); err != nil {
		t.Fatalf("create user a: %v", err)
	}

	// Tenant B cannot see A's user in the same project name.
	if _, err := store.GetUser(ctx, "tenant-b", "platform", "svc-a"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound for tenant b, got %v", err)
	}
	// And B cannot create a policy on A's project implicitly (resolveProject fails).
	if _, err := store.CreatePolicy(ctx, "tenant-b", "platform", pkg.PolicyInput{Name: "p", Document: docAllowS3}); err != nil {
		t.Fatalf("unexpected error creating policy in b: %v", err)
	}
	if _, err := store.CreateUser(ctx, "tenant-b", "platform", pkg.UserInput{Name: "svc-a"}); err != nil {
		t.Fatalf("tenant b cannot reuse a's name in its own project: %v", err)
	}
}

func TestUsersGroupsRolesPoliciesCRUD(t *testing.T) {
	store, stop := newStore(t)
	defer stop()
	ctx := context.Background()

	seedProject(t, store, "acme", "infra")

	u, err := store.CreateUser(ctx, "acme", "infra", pkg.UserInput{Name: "deploy", Path: "/ci/"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.Path != "/ci/" {
		t.Fatalf("expected path /ci/, got %q", u.Path)
	}
	if _, err := store.CreateGroup(ctx, "acme", "infra", pkg.GroupInput{Name: "devops"}); err != nil {
		t.Fatalf("create group: %v", err)
	}
	r, err := store.CreateRole(ctx, "acme", "infra", pkg.RoleInput{Name: "readonly"})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if r.ID == "" {
		t.Fatal("expected role to have an id")
	}
	p, err := store.CreatePolicy(ctx, "acme", "infra", pkg.PolicyInput{Name: "s3-read", Document: docAllowS3})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if p.Version != 1 {
		t.Fatalf("expected version 1, got %d", p.Version)
	}

	// Invalid policy documents are rejected.
	if _, err := store.CreatePolicy(ctx, "acme", "infra", pkg.PolicyInput{Name: "bad", Document: `{"Statement":[]}`}); err == nil {
		t.Fatal("expected invalid policy document to be rejected")
	}

	// Membership + attachments.
	if err := store.GroupAddMember(ctx, "acme", "infra", "devops", "deploy"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := store.AttachPolicy(ctx, "acme", "infra", database.PrincipalUser, "deploy", "s3-read"); err != nil {
		t.Fatalf("attach user policy: %v", err)
	}
	if _, err := store.AttachPolicy(ctx, "acme", "infra", database.PrincipalGroup, "devops", "s3-read"); err != nil {
		t.Fatalf("attach group policy: %v", err)
	}
	if _, err := store.AttachPolicy(ctx, "acme", "infra", database.PrincipalRole, "readonly", "s3-read"); err != nil {
		t.Fatalf("attach role policy: %v", err)
	}

	members, err := store.GroupMembers(ctx, "acme", "infra", "devops")
	if err != nil || len(members) != 1 || members[0].UserName != "deploy" {
		t.Fatalf("unexpected members: %v %v", members, err)
	}

	atts, err := store.ListAttachments(ctx, "acme", "infra", "", "")
	if err != nil || len(atts) != 3 {
		t.Fatalf("expected 3 attachments, got %d (%v)", len(atts), err)
	}

	// Cleanup cascades: deleting the group removes the membership.
	if err := store.DeleteGroup(ctx, "acme", "infra", "devops"); err != nil {
		t.Fatalf("delete group: %v", err)
	}
	if _, err := store.GroupMembers(ctx, "acme", "infra", "devops"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound after group delete, got %v", err)
	}
	if err := store.DeleteUser(ctx, "acme", "infra", "deploy"); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if err := store.DeleteRole(ctx, "acme", "infra", "readonly"); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	if err := store.DeletePolicy(ctx, "acme", "infra", "s3-read"); err != nil {
		t.Fatalf("delete policy: %v", err)
	}
	if _, err := store.GetPolicy(ctx, "acme", "infra", "s3-read"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound after policy delete, got %v", err)
	}
}

func TestAccessKeyAndTokenFlow(t *testing.T) {
	store, stop := newStore(t)
	defer stop()
	ctx := context.Background()

	seedProject(t, store, "acme", "platform")
	if _, err := store.CreateUser(ctx, "acme", "platform", pkg.UserInput{Name: "svc"}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	key, err := store.CreateAccessKey(ctx, "acme", "platform", "svc")
	if err != nil {
		t.Fatalf("create access key: %v", err)
	}
	if key.Secret == "" || len(key.ID) != 20 || key.ID[:4] != "AKIA" {
		t.Fatalf("unexpected key shape: %+v", key)
	}

	// The stored secret hash must not equal the secret.
	var storedHash string
	if err := store.DB.QueryRow(ctx, `SELECT secret_hash FROM access_keys WHERE id = $1`, key.ID).Scan(&storedHash); err != nil {
		t.Fatalf("query secret hash: %v", err)
	}
	if storedHash == key.Secret {
		t.Fatal("secret must never be stored in plaintext")
	}

	// Authentication succeeds with the right secret, fails otherwise.
	if _, err := store.Authenticate(ctx, key.ID, key.Secret); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if _, err := store.Authenticate(ctx, key.ID, "wrong-secret"); err != pkg.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	// Deactivating the key blocks authentication.
	if _, err := store.SetAccessKeyStatus(ctx, "acme", "platform", "svc", key.ID, "inactive"); err != nil {
		t.Fatalf("deactivate key: %v", err)
	}
	if _, err := store.Authenticate(ctx, key.ID, key.Secret); err != pkg.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for inactive key, got %v", err)
	}
	if _, err := store.SetAccessKeyStatus(ctx, "acme", "platform", "svc", key.ID, "active"); err != nil {
		t.Fatalf("reactivate key: %v", err)
	}

	// Mint and verify a token.
	token, claims, err := store.MintToken(ctx, key.ID, key.Secret)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	if claims.Subject != "svc" || claims.PrincipalType != database.PrincipalUser {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	verified, err := store.JWT.Verify(token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if verified.Subject != "svc" {
		t.Fatalf("unexpected verified subject: %+v", verified)
	}

	// Tokens are tamper-proof.
	if _, err := store.JWT.Verify(token[:len(token)-2] + "xx"); err == nil {
		t.Fatal("expected tampered token to fail verification")
	}
}

func TestPolicyEvaluation(t *testing.T) {
	store, stop := newStore(t)
	defer stop()
	ctx := context.Background()

	seedProject(t, store, "acme", "assets")
	if _, err := store.CreateUser(ctx, "acme", "assets", pkg.UserInput{Name: "reader"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := store.CreatePolicy(ctx, "acme", "assets", pkg.PolicyInput{Name: "allow-s3", Document: docAllowS3}); err != nil {
		t.Fatalf("create allow policy: %v", err)
	}
	if _, err := store.CreatePolicy(ctx, "acme", "assets", pkg.PolicyInput{Name: "deny-secrets", Document: docDenySensitive}); err != nil {
		t.Fatalf("create deny policy: %v", err)
	}

	// No policies attached -> implicit deny.
	dec, err := store.Authorize(ctx, "acme", "assets", database.PrincipalUser, "reader", "s3:GetObject", "arn:aws:s3:::assets/logo.png")
	if err != nil {
		t.Fatalf("authorize implicit deny: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected implicit deny with no attachments")
	}

	// Attach allow policy -> allow on matching action/resource.
	if _, err := store.AttachPolicy(ctx, "acme", "assets", database.PrincipalUser, "reader", "allow-s3"); err != nil {
		t.Fatalf("attach allow: %v", err)
	}
	dec, err = store.Authorize(ctx, "acme", "assets", database.PrincipalUser, "reader", "s3:GetObject", "arn:aws:s3:::assets/logo.png")
	if err != nil {
		t.Fatalf("authorize allow: %v", err)
	}
	if !dec.Allowed {
		t.Fatalf("expected allow, got %+v", dec)
	}

	// Wildcard namespace action does not match unrelated actions.
	dec, err = store.Authorize(ctx, "acme", "assets", database.PrincipalUser, "reader", "iam:CreateUser", "arn:aws:s3:::assets/x")
	if err != nil {
		t.Fatalf("authorize non-match: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected deny for non-matching action")
	}

	// Explicit deny overrides allow.
	if _, err := store.AttachPolicy(ctx, "acme", "assets", database.PrincipalUser, "reader", "deny-secrets"); err != nil {
		t.Fatalf("attach deny: %v", err)
	}
	dec, err = store.Authorize(ctx, "acme", "assets", database.PrincipalUser, "reader", "s3:GetObject", "arn:aws:s3:::assets/secrets/db-password")
	if err != nil {
		t.Fatalf("authorize deny: %v", err)
	}
	if dec.Allowed {
		t.Fatalf("expected explicit deny to override allow, got %+v", dec)
	}
	if len(dec.Matched) != 2 {
		t.Fatalf("expected 2 matched statements, got %v", dec.Matched)
	}

	// Group inheritance: attaching the allow policy to a group grants members.
	if _, err := store.CreateGroup(ctx, "acme", "assets", pkg.GroupInput{Name: "viewers"}); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := store.CreateUser(ctx, "acme", "assets", pkg.UserInput{Name: "member"}); err != nil {
		t.Fatalf("create member user: %v", err)
	}
	if err := store.GroupAddMember(ctx, "acme", "assets", "viewers", "member"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := store.AttachPolicy(ctx, "acme", "assets", database.PrincipalGroup, "viewers", "allow-s3"); err != nil {
		t.Fatalf("attach group policy: %v", err)
	}
	dec, err = store.Authorize(ctx, "acme", "assets", database.PrincipalUser, "member", "s3:GetObject", "arn:aws:s3:::assets/logo.png")
	if err != nil {
		t.Fatalf("authorize via group: %v", err)
	}
	if !dec.Allowed {
		t.Fatalf("expected member to inherit group policy, got %+v", dec)
	}

	// Detaching removes the grant.
	if err := store.DetachPolicy(ctx, "acme", "assets", database.PrincipalUser, "reader", "allow-s3"); err != nil {
		t.Fatalf("detach: %v", err)
	}
	dec, _ = store.Authorize(ctx, "acme", "assets", database.PrincipalUser, "reader", "s3:GetObject", "arn:aws:s3:::assets/logo.png")
	if dec.Allowed {
		t.Fatal("expected deny after detach (deny policy still attached)")
	}
}
