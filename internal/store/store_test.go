package store_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

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
		-- 0001 also made audit_log; later migrations alter it.
		CREATE TABLE audit_log (id INTEGER PRIMARY KEY AUTOINCREMENT);
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

	files, _ := filepath.Glob("migrations/*.sql")
	if n != len(files) {
		t.Errorf("schema_migrations has %d rows after two opens, want one per file (%d)", n, len(files))
	}
}

// The activity page filters by category (a family of actions such as every
// key action) and by who did it; both must narrow the list, and paging must
// continue past the last row shown.
func TestListAuditFiltersByCategoryAndActor(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()

	for _, e := range []store.AuditEntry{
		{ActorLabel: "a@example.com", Action: "apikey.create", TargetType: "apikey"},
		{ActorLabel: "b@example.com", Action: "preauthkey.expire", TargetType: "preauthkey"},
		{ActorLabel: "a@example.com", Action: "device.rename", TargetType: "device"},
		{ActorLabel: "a@example.com", Action: "apikey.delete", TargetType: "apikey"},
	} {
		if err := st.Audit(ctx, e); err != nil {
			t.Fatalf("Audit: %v", err)
		}
	}

	keys, err := st.ListAudit(ctx, store.AuditFilter{Prefixes: []string{"apikey.", "preauthkey."}})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("key category = %d entries, want 3", len(keys))
	}

	mine, err := st.ListAudit(ctx, store.AuditFilter{Prefixes: []string{"apikey."}, Actor: "a@example.com"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(mine) != 2 || mine[0].Action != "apikey.delete" {
		t.Errorf("actor filter = %+v, want a's two apikey rows newest first", mine)
	}

	next, err := st.ListAudit(ctx, store.AuditFilter{Limit: 1, Before: mine[0].ID})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(next) != 1 || next[0].Action != "device.rename" {
		t.Errorf("page after the newest = %+v, want device.rename", next)
	}
}

// The label is what the activity page names the target by. It must survive
// the target itself: a deleted device has no name anywhere else.
func TestAuditKeepsTheTargetLabel(t *testing.T) {
	st := openStore(t)

	if err := st.Audit(t.Context(), store.AuditEntry{Action: "device.delete", TargetType: "device", TargetID: "4", TargetLabel: "web-1"}); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListAudit(t.Context(), store.AuditFilter{})
	if err != nil || len(got) != 1 || got[0].TargetLabel != "web-1" {
		t.Fatalf("entries = %+v, %v; want the label back", got, err)
	}
}

// Five wrong PINs lock sudo, counted in the database so a restart or a second
// tab does not reset the count; a right PIN or a new PIN clears it.
func TestPinFailuresLockAndClear(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()
	u, err := st.UpsertLogin(ctx, identity("https://idp.example", "pin", "pin@example.com"))
	if err != nil {
		t.Fatal(err)
	}

	if err := st.SetPin(ctx, u.ID, "hash"); err != nil {
		t.Fatal(err)
	}
	got, _ := st.UserByID(ctx, u.ID)
	if got.PinHash != "hash" {
		t.Fatalf("pin hash = %q", got.PinHash)
	}

	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	var locked *time.Time
	for i := 1; i <= 5; i++ {
		var n int
		n, locked, err = st.RecordPinFailure(ctx, u.ID, 5, 15*time.Minute, now)
		if err != nil || n != i {
			t.Fatalf("failure %d: n=%d err=%v", i, n, err)
		}
		if i < 5 && locked != nil {
			t.Fatalf("locked after %d failures", i)
		}
	}
	if locked == nil || !locked.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("locked until %v, want 15 minutes after the fifth failure", locked)
	}

	got, _ = st.UserByID(ctx, u.ID)
	if got.PinLockedUntil == nil {
		t.Error("lock not stored on the account")
	}

	if err := st.ResetPinFailures(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = st.UserByID(ctx, u.ID)
	if got.PinLockedUntil != nil || got.PinFailures != 0 {
		t.Errorf("after reset: failures=%d locked=%v", got.PinFailures, got.PinLockedUntil)
	}

	if err := st.ResetPin(ctx, u.ID, store.AuditEntry{ActorLabel: "cli", Action: "account.pin_reset", TargetType: "account"}); err != nil {
		t.Fatal(err)
	}
	if got, _ = st.UserByID(ctx, u.ID); got.PinHash != "" {
		t.Error("ResetPin left a hash")
	}
}

// The reset command must not create a database nor migrate one: it runs next
// to a live server, and a wrong path should fail loudly, not start empty.
func TestOpenExistingNeverCreatesOrMigrates(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.db")
	if _, err := store.OpenExisting(t.Context(), missing); err == nil {
		t.Error("opened a database that does not exist")
	}

	path := filepath.Join(t.TempDir(), "h.db")
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	again, err := store.OpenExisting(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenExisting on a migrated database: %v", err)
	}
	again.Close()

	stale := filepath.Join(t.TempDir(), "stale.db")
	db, _ := sql.Open("sqlite", stale)
	_, _ = db.Exec(`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT); INSERT INTO schema_migrations (name) VALUES ('0001_init.sql')`)
	db.Close()
	if _, err := store.OpenExisting(t.Context(), stale); err == nil {
		t.Error("opened a database whose schema is behind this binary")
	}
}

func TestBumpCredVersion(t *testing.T) {
	st := openStore(t)
	u, _ := st.UpsertLogin(t.Context(), identity("https://idp.example", "v", "v@example.com"))

	if err := st.BumpCredVersion(t.Context(), u.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := st.UserByID(t.Context(), u.ID)
	if got.CredVersion != u.CredVersion+1 {
		t.Errorf("cred version %d, want %d", got.CredVersion, u.CredVersion+1)
	}

	same, err := st.UsersByEmail(t.Context(), "V@example.com")
	if err != nil || len(same) != 1 || same[0].ID != u.ID {
		t.Errorf("UsersByEmail = %v, %v", same, err)
	}
}

func TestSudoGrants(t *testing.T) {
	st := openStore(t)
	u, _ := st.UpsertLogin(t.Context(), identity("https://idp.example", "s", "s@example.com"))
	now := time.Now()

	if err := st.GrantSudo(t.Context(), "tok", u.ID, now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if until, ok := st.SudoUntil(t.Context(), "tok", u.ID, now); !ok || until.Before(now) {
		t.Errorf("grant not found: %v %v", until, ok)
	}
	if _, ok := st.SudoUntil(t.Context(), "tok", u.ID+1, now); ok {
		t.Error("a grant answered for another account")
	}
	if _, ok := st.SudoUntil(t.Context(), "tok", u.ID, now.Add(6*time.Minute)); ok {
		t.Error("an expired grant still counts")
	}
	if err := st.EndSudo(t.Context(), "tok"); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.SudoUntil(t.Context(), "tok", u.ID, now); ok {
		t.Error("an ended grant still counts")
	}
}

// A reset changes the credential, signs the account out and records itself in
// one transaction: no crash between them can leave old sessions working.
func TestResetCredentialIsOneTransaction(t *testing.T) {
	st := openStore(t)
	u, err := st.CreateLocalOwner(t.Context(), "ops@example.com", "old")
	if err != nil {
		t.Fatal(err)
	}
	_ = st.SetPin(t.Context(), u.ID, "pin")
	entry := func(action string) store.AuditEntry {
		return store.AuditEntry{ActorLabel: "cli", Action: action, TargetType: "account", TargetLabel: u.Email}
	}

	if err := st.ResetPassword(t.Context(), u.ID, "new", entry("account.password_reset")); err != nil {
		t.Fatal(err)
	}
	if err := st.ResetPin(t.Context(), u.ID, entry("account.pin_reset")); err != nil {
		t.Fatal(err)
	}

	got, _ := st.UserByID(t.Context(), u.ID)
	if got.PasswordHash != "new" || got.PinHash != "" || got.CredVersion != u.CredVersion+2 {
		t.Errorf("after resets: hash=%q pin=%q version=%d", got.PasswordHash, got.PinHash, got.CredVersion)
	}
	if e, _ := st.ListAudit(t.Context(), store.AuditFilter{Prefixes: []string{"account."}}); len(e) != 2 {
		t.Errorf("audit rows = %d, want 2", len(e))
	}

	// A failed audit write must undo the reset with it.
	bad := entry("account.pin_reset")
	missing := int64(999)
	bad.ActorUserID = &missing
	if err := st.ResetPin(t.Context(), u.ID, bad); err == nil {
		t.Fatal("a reset with an unwritable audit row succeeded")
	}
	if after, _ := st.UserByID(t.Context(), u.ID); after.CredVersion != got.CredVersion {
		t.Error("the version moved although the reset failed")
	}
}
