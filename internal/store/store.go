// Package store is Headboard's own database: accounts, sessions, audit log,
// policy history and settings.
//
// It deliberately mirrors no tailnet state. Nodes, users, keys and the ACL
// policy are read live from Headscale on every request, so there is no cache to
// go stale and no second source of truth to reconcile.
//
// SQLite, for the same reason Headscale uses it: one writer, one instance, and
// a schema small enough that a second container would cost more to operate than
// it could ever return. The driver is pure Go, so the release binary stays
// static, and it is already in the dependency tree — Headscale itself pulls it.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store holds the database handle.
type Store struct {
	db *sql.DB
}

// Open connects, verifies the connection and applies pending migrations.
//
// The path may be a bare filename; the pragmas Headboard depends on are added
// here rather than being left to the caller, because two of them are off by
// default and silently change behaviour rather than failing.
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("no database path")
	}

	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// One connection. Writes are serialised rather than racing for the write
	// lock, which removes SQLITE_BUSY as a class of bug; the workload is a
	// handful of statements per request, so the contention cost is nil.
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		db.Close()

		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	s := &Store{db: db}

	if err := s.migrate(ctx); err != nil {
		db.Close()

		return nil, err
	}

	return s, nil
}

// dsn adds the pragmas that must be set per connection.
//
//   - foreign_keys is OFF by default in SQLite, and audit_log and
//     policy_revisions both rely on ON DELETE SET NULL. Without it, deleting an
//     account leaves rows pointing at an id that no longer exists.
//   - WAL lets the poller and the SSE fanout read while a request writes.
//   - busy_timeout turns a momentary lock into a wait instead of an error.
func dsn(path string) string {
	pragmas := url.Values{}
	pragmas.Add("_pragma", "journal_mode(WAL)")
	pragmas.Add("_pragma", "busy_timeout(5000)")
	pragmas.Add("_pragma", "foreign_keys(on)")

	return "file:" + path + "?" + pragmas.Encode()
}

// DB exposes the handle for the session store, which owns its own table.
func (s *Store) DB() *sql.DB { return s.db }

// Close releases the handle.
func (s *Store) Close() { _ = s.db.Close() }

// migrate applies every unapplied migration in one transaction each.
//
// A hand-rolled runner rather than a migration library: the schema is small and
// append-only, and one embedded directory with a ledger table is less to
// explain than another dependency and its CLI.
func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`)
	if err != nil {
		return fmt.Errorf("creating migration ledger: %w", err)
	}

	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("reading migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	// Filenames carry the ordering, so they must be applied sorted rather
	// than in directory order.
	sort.Strings(names)

	for _, name := range names {
		applied, err := s.migrationApplied(ctx, name)
		if err != nil {
			return err
		}

		if applied {
			continue
		}

		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		if err := s.applyMigration(ctx, name, string(body)); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) migrationApplied(ctx context.Context, name string) (bool, error) {
	var n int

	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM schema_migrations WHERE name = ?`, name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking migration %s: %w", name, err)
	}

	return n > 0, nil
}

func (s *Store) applyMigration(ctx context.Context, name, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning migration %s: %w", name, err)
	}

	defer func() { _ = tx.Rollback() }()

	// database/sql sends one statement per Exec, and a migration is a script.
	// Splitting on the semicolon at end of line keeps the migration files
	// readable rather than forcing one statement per file.
	for _, stmt := range splitStatements(body) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("applying migration %s: %w\n%s", name, err, stmt)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("recording migration %s: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration %s: %w", name, err)
	}

	return nil
}

// splitStatements breaks a migration script into statements.
//
// Naive on purpose: it splits on ";" and drops empties, which is correct for
// DDL and would not be for a script containing a trigger body or a string with
// a semicolon in it. The migrations are checked in next to this code, so the
// constraint is visible rather than surprising.
func splitStatements(body string) []string {
	parts := strings.Split(body, ";")
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		if strings.TrimSpace(stripComments(p)) == "" {
			continue
		}

		out = append(out, p)
	}

	return out
}

func stripComments(s string) string {
	var b strings.Builder

	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}

		b.WriteString(line)
		b.WriteByte('\n')
	}

	return b.String()
}

// Path returns the file a store was opened from, for logging.
func Path(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}

	return abs
}

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = sql.ErrNoRows

// isUniqueViolation reports whether an error is a uniqueness conflict.
//
// Matched on the SQLite result code rather than the message: the text carries
// the column name and would drift, the codes will not.
func isUniqueViolation(err error) bool {
	var sqErr *sqlite.Error

	if !errors.As(err, &sqErr) {
		return false
	}

	code := sqErr.Code()

	return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE ||
		code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}
