package store_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jemang/headboard/internal/store"
)

// Every test gets its own database file, so they neither see each other's rows
// nor need anything running. These used to skip unless DATABASE_URL pointed at
// a Postgres — which meant they mostly did not run at all.
func openStore(t *testing.T) *store.Store {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "headboard.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(st.Close)

	return st
}

func identity(iss, sub, email string) store.User {
	return store.User{OIDCIssuer: iss, OIDCSubject: sub, Email: email, DisplayName: email}
}

// Whoever sets Headboard up must end up able to administer it, and everyone
// after them must not.
func TestFirstLoginBecomesOwner(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()

	first, err := st.UpsertLogin(ctx, identity("https://idp.example", "one", "one@example.com"))
	if err != nil {
		t.Fatalf("first login: %v", err)
	}

	if first.Role != store.RoleOwner {
		t.Errorf("first login role = %s, want owner", first.Role)
	}
	if first.Admission != store.AdmissionActive {
		t.Errorf("first login admission = %s, want active", first.Admission)
	}

	second, err := st.UpsertLogin(ctx, identity("https://idp.example", "two", "two@example.com"))
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	if second.Role != store.RoleMember {
		t.Errorf("second login role = %s, want member", second.Role)
	}
	if second.Admission != store.AdmissionPending {
		t.Errorf("second login admission = %s, want pending", second.Admission)
	}
}

// Logging in again must not reset a role that an admin deliberately changed.
func TestRepeatLoginKeepsRole(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()

	u, err := st.UpsertLogin(ctx, identity("https://idp.example", "one", "one@example.com"))
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if _, err := st.UpsertLogin(ctx, identity("https://idp.example", "two", "two@example.com")); err != nil {
		t.Fatalf("second identity: %v", err)
	}

	promoted, err := st.SetRole(ctx, 2, store.RoleNetworkAdmin)
	if err != nil {
		t.Fatalf("SetRole: %v", err)
	}

	again, err := st.UpsertLogin(ctx, identity("https://idp.example", "two", "renamed@example.com"))
	if err != nil {
		t.Fatalf("repeat login: %v", err)
	}

	if again.Role != promoted.Role {
		t.Errorf("repeat login reset the role to %s, want %s", again.Role, promoted.Role)
	}

	// Profile fields should refresh, though.
	if again.Email != "renamed@example.com" {
		t.Errorf("repeat login did not refresh the email: %q", again.Email)
	}

	if u.ID == again.ID {
		t.Error("the second identity reused the first account's id")
	}
}

func TestAdmissionCanBeDecidedAndSurvivesLogin(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()

	if _, err := st.UpsertLogin(ctx, identity("https://idp.example", "owner", "owner@example.com")); err != nil {
		t.Fatalf("owner login: %v", err)
	}

	pending, err := st.UpsertLogin(ctx, identity("https://idp.example", "member", "member@example.com"))
	if err != nil {
		t.Fatalf("member login: %v", err)
	}

	approved, err := st.SetAdmission(ctx, pending.ID, store.AdmissionActive)
	if err != nil {
		t.Fatalf("SetAdmission: %v", err)
	}
	if !approved.Active() {
		t.Errorf("approved account is not active: %+v", approved)
	}

	again, err := st.UpsertLogin(ctx, identity("https://idp.example", "member", "renamed@example.com"))
	if err != nil {
		t.Fatalf("repeat login: %v", err)
	}
	if again.Admission != store.AdmissionActive {
		t.Errorf("repeat login admission = %s, want active", again.Admission)
	}

	if _, err := st.SetAdmission(ctx, pending.ID, store.AdmissionState("unknown")); err == nil {
		t.Error("SetAdmission accepted an unknown state")
	}
}

func TestAdmissionMigrationMovesOnlyLegacyMembersToPending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "headboard.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening legacy database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT);
		INSERT INTO schema_migrations (name) VALUES ('0001_init.sql');
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			oidc_iss TEXT NOT NULL,
			oidc_sub TEXT NOT NULL,
			password_hash TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			avatar_url TEXT NOT NULL DEFAULT '',
			headscale_user_id INTEGER,
			role TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_login_at TEXT
		);
	`)
	if err != nil {
		t.Fatalf("creating legacy schema: %v", err)
	}

	for i, role := range []store.Role{
		store.RoleOwner,
		store.RoleAdmin,
		store.RoleNetworkAdmin,
		store.RoleAuditor,
		store.RoleMember,
	} {
		if _, err := db.Exec(`INSERT INTO users (oidc_iss, oidc_sub, email, display_name, role, created_at)
			VALUES (?, ?, ?, ?, ?, '2026-08-01T00:00:00Z')`, "https://idp.example", role, role+"@example.com", role, role); err != nil {
			t.Fatalf("inserting %s: %v", role, err)
		}
		_ = i
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing legacy database: %v", err)
	}

	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("migrating legacy database: %v", err)
	}
	defer st.Close()

	accounts, err := st.ListUsers(t.Context())
	if err != nil {
		t.Fatalf("listing migrated accounts: %v", err)
	}
	for _, account := range accounts {
		want := store.AdmissionActive
		if account.Role == store.RoleMember {
			want = store.AdmissionPending
		}
		if account.Admission != want {
			t.Errorf("%s admission = %s, want %s", account.Role, account.Admission, want)
		}
	}
}

// Two identities pointing at one Headscale user would each see the other's
// devices as their own.
func TestHeadscaleLinkIsExclusive(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()

	if _, err := st.UpsertLogin(ctx, identity("https://idp.example", "one", "one@example.com")); err != nil {
		t.Fatalf("first login: %v", err)
	}

	if _, err := st.UpsertLogin(ctx, identity("https://idp.example", "two", "two@example.com")); err != nil {
		t.Fatalf("second login: %v", err)
	}

	hsUser := int64(42)

	if _, err := st.LinkHeadscaleUser(ctx, 1, &hsUser); err != nil {
		t.Fatalf("linking account 1: %v", err)
	}

	_, err := st.LinkHeadscaleUser(ctx, 2, &hsUser)
	if !errors.Is(err, store.ErrLinkTaken) {
		t.Fatalf("second link returned %v, want ErrLinkTaken", err)
	}

	// Unlinking must free the headscale user for someone else.
	if _, err := st.LinkHeadscaleUser(ctx, 1, nil); err != nil {
		t.Fatalf("unlinking: %v", err)
	}

	if _, err := st.LinkHeadscaleUser(ctx, 2, &hsUser); err != nil {
		t.Fatalf("relinking after unlink: %v", err)
	}
}

func TestCountOwners(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()

	if _, err := st.UpsertLogin(ctx, identity("https://idp.example", "one", "one@example.com")); err != nil {
		t.Fatalf("login: %v", err)
	}

	n, err := st.CountOwners(ctx)
	if err != nil {
		t.Fatalf("CountOwners: %v", err)
	}

	if n != 1 {
		t.Errorf("owners = %d, want 1", n)
	}
}

func TestAuditRoundTrip(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()

	u, err := st.UpsertLogin(ctx, identity("https://idp.example", "one", "one@example.com"))
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	err = st.Audit(ctx, store.AuditEntry{
		ActorUserID: &u.ID,
		ActorLabel:  u.Email,
		Action:      "node.rename",
		TargetType:  "node",
		TargetID:    "7",
		Before:      []byte(`{"name":"old"}`),
		After:       []byte(`{"name":"new"}`),
		IP:          "203.0.113.9",
	})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}

	entries, err := st.ListAudit(ctx, store.AuditFilter{TargetType: "node"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}

	// Byte-for-byte: the column is TEXT, so what went in comes back out.
	// Postgres reformatted this through JSONB and the assertion used to carry
	// its spacing.
	got := entries[0]
	if got.Action != "node.rename" || got.TargetID != "7" || string(got.After) != `{"name":"new"}` {
		t.Errorf("round trip lost detail: %+v", got)
	}

	if string(got.Before) != `{"name":"old"}` {
		t.Errorf("before = %s, want the document that was written", got.Before)
	}

	// Filters must actually filter, or the audit page lies by omission.
	other, err := st.ListAudit(ctx, store.AuditFilter{TargetType: "policy"})
	if err != nil {
		t.Fatalf("ListAudit(policy): %v", err)
	}

	if len(other) != 0 {
		t.Errorf("filtering by a different target returned %d entries", len(other))
	}
}

func TestPolicyRevisions(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()

	const body = `{ // kept verbatim
	  "acls": [],
	}`

	rev, err := st.SavePolicyRevision(ctx, store.PolicyRevision{
		SHA256: "abc123",
		Body:   body,
		Note:   "first",
	})
	if err != nil {
		t.Fatalf("SavePolicyRevision: %v", err)
	}

	got, err := st.PolicyRevision(ctx, rev.ID)
	if err != nil {
		t.Fatalf("PolicyRevision: %v", err)
	}

	// Rollback is only worth having if the stored text is byte-identical to
	// what was applied, comments included.
	if got.Body != body {
		t.Errorf("body changed in storage:\n got %q\nwant %q", got.Body, body)
	}

	list, err := st.ListPolicyRevisions(ctx, 10)
	if err != nil {
		t.Fatalf("ListPolicyRevisions: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("revisions = %d, want 1", len(list))
	}

	if list[0].Body != "" {
		t.Error("the list included revision bodies; it should not")
	}
}

// Open runs migrations, so calling it twice against the same database must be
// safe — every restart does exactly that.
func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "headboard.db")

	first, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	first.Close()

	again, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer again.Close()

	var n int
	if err := again.DB().QueryRowContext(t.Context(),
		`SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("counting migrations: %v", err)
	}

	if n != 2 {
		t.Errorf("schema_migrations has %d rows after two opens, want 2", n)
	}
}
