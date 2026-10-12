package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SessionStore implements scs.Store (and scs.IterableStore) over SQLite.
//
// Written here rather than pulled in: scs ships stores for Postgres and for the
// cgo SQLite driver, and neither fits a pure-Go build. The interface is four
// methods over one table, which is less code than a dependency and its version
// skew.
type SessionStore struct {
	db *sql.DB
}

// Sessions returns the session store for this database.
func (s *Store) Sessions() *SessionStore { return &SessionStore{db: s.db} }

// Find returns the data for a token, and false when it is missing or expired.
func (st *SessionStore) Find(token string) ([]byte, bool, error) {
	var data []byte

	err := st.db.QueryRow(
		`SELECT data FROM sessions WHERE token = ? AND expiry > ?`,
		token, nowText(),
	).Scan(&data)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("reading session: %w", err)
	}

	return data, true, nil
}

// Commit writes a session, replacing any existing one for the token.
func (st *SessionStore) Commit(token string, b []byte, expiry time.Time) error {
	_, err := st.db.Exec(`
		INSERT INTO sessions (token, data, expiry) VALUES (?, ?, ?)
		ON CONFLICT (token) DO UPDATE SET data = excluded.data, expiry = excluded.expiry`,
		token, b, fromTime(expiry),
	)
	if err != nil {
		return fmt.Errorf("writing session: %w", err)
	}

	return nil
}

// Delete removes a session.
func (st *SessionStore) Delete(token string) error {
	if _, err := st.db.Exec(`DELETE FROM sessions WHERE token = ?`, token); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}

	return nil
}

// All returns every live session, which scs uses to invalidate in bulk.
func (st *SessionStore) All() (map[string][]byte, error) {
	rows, err := st.db.Query(`SELECT token, data FROM sessions WHERE expiry > ?`, nowText())
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	out := map[string][]byte{}

	for rows.Next() {
		var (
			token string
			data  []byte
		)

		if err := rows.Scan(&token, &data); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}

		out[token] = data
	}

	return out, rows.Err()
}

// Cleanup deletes expired sessions until the context is cancelled.
//
// scs stores are expected to prune themselves; nothing else ever deletes these
// rows, so without this the table grows for the life of the deployment.
func (st *SessionStore) Cleanup(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = st.db.ExecContext(ctx, `DELETE FROM sessions WHERE expiry < ?`, nowText())
		}
	}
}

// GrantSudo records sudo for one session until the given time. tokenHash is a
// hash of the session token, never the token itself.
func (s *Store) GrantSudo(ctx context.Context, tokenHash string, userID int64, until time.Time) error {
	// Expired grants are dead weight; clearing them here keeps the table small
	// without a background job.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sudo_grants WHERE until <= ?`, nowText()); err != nil {
		return fmt.Errorf("clearing expired sudo: %w", err)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sudo_grants (token_hash, user_id, until) VALUES (?, ?, ?)
		ON CONFLICT (token_hash) DO UPDATE SET user_id = excluded.user_id, until = excluded.until`,
		tokenHash, userID, fromTime(until))
	if err != nil {
		return fmt.Errorf("granting sudo: %w", err)
	}

	return nil
}

// SudoUntil reports when this session's sudo ends, if it is on for this account at now.
func (s *Store) SudoUntil(ctx context.Context, tokenHash string, userID int64, now time.Time) (time.Time, bool) {
	var until time.Time

	err := s.db.QueryRowContext(ctx,
		`SELECT until FROM sudo_grants WHERE token_hash = ? AND user_id = ? AND until > ?`,
		tokenHash, userID, fromTime(now),
	).Scan(intoTime(&until))

	return until, err == nil
}

// EndSudo closes sudo for one session.
func (s *Store) EndSudo(ctx context.Context, tokenHash string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sudo_grants WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("ending sudo: %w", err)
	}

	return nil
}
