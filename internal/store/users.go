package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// LocalIssuer marks an account that signs in with a password rather than
// through an identity provider. It occupies the oidc_iss column so that one
// uniqueness rule covers both kinds of account; no real issuer can collide with
// it, because a real issuer is an absolute URL.
const LocalIssuer = "local"

// Role is what a person may do in Headboard. It is Headboard's concept, not
// Headscale's — Headscale has no notion of an admin versus a member.
type Role string

const (
	// RoleOwner is the first identity to log in. Cannot be demoted by
	// anyone but another owner, so the tailnet cannot be locked out.
	RoleOwner Role = "owner"

	// RoleAdmin can do everything except change owners.
	RoleAdmin Role = "admin"

	// RoleNetworkAdmin manages the policy and routes but not people.
	RoleNetworkAdmin Role = "network-admin"

	// RoleAuditor reads everything, changes nothing.
	RoleAuditor Role = "auditor"

	// RoleMember sees only their own devices.
	RoleMember Role = "member"
)

// Rank orders roles for comparison. A higher rank includes every capability of
// the ranks below it, except that network-admin and auditor are deliberately
// narrow rather than "admin minus a bit".
func (r Role) Rank() int {
	switch r {
	case RoleOwner:
		return 50
	case RoleAdmin:
		return 40
	case RoleNetworkAdmin:
		return 30
	case RoleAuditor:
		return 20
	case RoleMember:
		return 10
	default:
		return 0
	}
}

// Valid reports whether the role is one Headboard knows.
func (r Role) Valid() bool { return r.Rank() > 0 }

// User is a Headboard account: an identity plus the role and the Headscale user
// it maps to.
type User struct {
	ID   int64 `json:"id"`
	Role Role  `json:"role"`

	OIDCIssuer  string `json:"-"`
	OIDCSubject string `json:"-"`

	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl,omitempty"`

	// HeadscaleUserID is nil until the identity is linked to a Headscale
	// user. An unlinked member can log in but has no devices to show, which
	// is why the admin link screen exists.
	HeadscaleUserID *int64 `json:"headscaleUserId,omitempty"`

	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`

	// PasswordHash is never serialised. Empty for accounts that sign in
	// through an identity provider.
	PasswordHash string `json:"-"`
}

// Local reports whether this account signs in with a password.
func (u User) Local() bool { return u.OIDCIssuer == LocalIssuer }

// Linked reports whether this identity maps to a Headscale user.
func (u User) Linked() bool { return u.HeadscaleUserID != nil }

const userColumns = `
	id, oidc_iss, oidc_sub, email, display_name, avatar_url,
	headscale_user_id, role, created_at, last_login_at, password_hash`

// scanner is the shared shape of *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanUser(row scanner) (User, error) {
	var u User

	err := row.Scan(
		&u.ID, &u.OIDCIssuer, &u.OIDCSubject, &u.Email, &u.DisplayName,
		&u.AvatarURL, &u.HeadscaleUserID, &u.Role,
		intoTime(&u.CreatedAt), intoNullTime(&u.LastLoginAt), &u.PasswordHash,
	)
	if err != nil {
		return User{}, err
	}

	return u, nil
}

// UserByOIDC looks an identity up by issuer and subject.
func (s *Store) UserByOIDC(ctx context.Context, iss, sub string) (User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT`+userColumns+` FROM users WHERE oidc_iss = ? AND oidc_sub = ?`, iss, sub)

	return scanUser(row)
}

// UserByID looks an account up by its Headboard id.
func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT`+userColumns+` FROM users WHERE id = ?`, id)

	return scanUser(row)
}

// ListUsers returns every Headboard account, newest last.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT`+userColumns+` FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var out []User

	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}

		out = append(out, u)
	}

	return out, rows.Err()
}

// UpsertLogin records a successful OIDC login, creating the account on first
// sight.
//
// The first identity to log in becomes owner. That is done in the same
// statement as the insert, so two simultaneous first logins cannot both win the
// role: the second sees a non-empty table.
func (s *Store) UpsertLogin(ctx context.Context, u User) (User, error) {
	now := nowText()

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO users (oidc_iss, oidc_sub, email, display_name, avatar_url,
		                   headscale_user_id, role, last_login_at)
		VALUES (?, ?, ?, ?, ?, ?,
		        CASE WHEN EXISTS (SELECT 1 FROM users) THEN ? ELSE ? END,
		        ?)
		ON CONFLICT (oidc_iss, oidc_sub) DO UPDATE SET
			email         = excluded.email,
			display_name  = excluded.display_name,
			avatar_url    = excluded.avatar_url,
			last_login_at = excluded.last_login_at
		RETURNING`+userColumns,
		u.OIDCIssuer, u.OIDCSubject, u.Email, u.DisplayName, u.AvatarURL,
		u.HeadscaleUserID, string(RoleMember), string(RoleOwner), now,
	)

	return scanUser(row)
}

// ErrLinkTaken is returned when a Headscale user is already claimed.
var ErrLinkTaken = errors.New("that headscale user is already linked to another account")

// LinkHeadscaleUser points a Headboard account at a Headscale user. Passing nil
// unlinks it.
func (s *Store) LinkHeadscaleUser(ctx context.Context, id int64, headscaleUserID *int64) (User, error) {
	row := s.db.QueryRowContext(ctx,
		`UPDATE users SET headscale_user_id = ? WHERE id = ? RETURNING`+userColumns,
		headscaleUserID, id)

	u, err := scanUser(row)
	if isUniqueViolation(err) {
		return User{}, ErrLinkTaken
	}

	return u, err
}

// SetRole changes an account's role.
func (s *Store) SetRole(ctx context.Context, id int64, role Role) (User, error) {
	if !role.Valid() {
		return User{}, fmt.Errorf("unknown role %q", role)
	}

	row := s.db.QueryRowContext(ctx,
		`UPDATE users SET role = ? WHERE id = ? RETURNING`+userColumns, string(role), id)

	return scanUser(row)
}

// CountOwners reports how many owners exist, so the last one cannot be demoted
// or deleted into a locked-out tailnet.
func (s *Store) CountOwners(ctx context.Context) (int, error) {
	var n int

	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM users WHERE role = ?`, string(RoleOwner)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting owners: %w", err)
	}

	return n, nil
}

// DeleteUser removes a Headboard account. The Headscale user is untouched.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting user %d: %w", id, err)
	}

	return nil
}

// CountUsers reports how many accounts exist. Used at startup to decide whether
// this is a first run.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int

	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}

	return n, nil
}

// UserByLocalEmail looks up a password account.
func (s *Store) UserByLocalEmail(ctx context.Context, email string) (User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT`+userColumns+` FROM users WHERE oidc_iss = ? AND oidc_sub = ?`,
		LocalIssuer, normaliseEmail(email))

	return scanUser(row)
}

// CreateLocalOwner creates the first password account, as owner.
//
// It refuses if any account already exists: this runs unattended at startup,
// and quietly minting an owner into a populated database would be a way in that
// nobody asked for.
func (s *Store) CreateLocalOwner(ctx context.Context, email, hash string) (User, error) {
	n, err := s.CountUsers(ctx)
	if err != nil {
		return User{}, err
	}

	if n > 0 {
		return User{}, errors.New("accounts already exist")
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO users (oidc_iss, oidc_sub, password_hash, email, display_name, role)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING`+userColumns,
		LocalIssuer, normaliseEmail(email), hash, email, "Administrator", string(RoleOwner))

	return scanUser(row)
}

// SetPassword replaces an account's password hash. Passing an empty hash would
// leave an account that cannot be signed into, so it is rejected.
func (s *Store) SetPassword(ctx context.Context, id int64, hash string) error {
	if hash == "" {
		return errors.New("refusing to set an empty password")
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		return fmt.Errorf("setting password: %w", err)
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	return nil
}

// LocalOwner returns the password account with the lowest id, which is the one
// the reset path acts on.
func (s *Store) LocalOwner(ctx context.Context) (User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT`+userColumns+` FROM users WHERE oidc_iss = ? ORDER BY id LIMIT 1`, LocalIssuer)

	return scanUser(row)
}

// TouchLogin records a successful password login.
func (s *Store) TouchLogin(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET last_login_at = ? WHERE id = ?`, nowText(), id)
	if err != nil {
		return fmt.Errorf("recording login: %w", err)
	}

	return nil
}

// normaliseEmail is what makes the local identity stable: the address is the
// subject, so it must not vary by case or stray whitespace.
func normaliseEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

var _ = sql.ErrNoRows
