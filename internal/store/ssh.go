package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrDuplicateSSHKey = errors.New("this key is already registered")
	ErrTargetTaken     = errors.New("that target is already bound to another host")
	ErrAgentNameTaken  = errors.New("an agent with that name already exists")
)

type SSHKey struct {
	ID              int64
	HeadscaleUserID uint64
	PublicKey       string
	Fingerprint     string
	Label           string
	CreatedAt       time.Time
}

type SSHAssignment struct {
	HostName        string
	UnixUser        string
	HeadscaleUserID uint64
}

type SSHTarget struct {
	HostName      string
	AgentID       int64
	TargetRef     string
	ApprovedIP    string
	ApprovedLabel string
	Suspended     bool
}

type SSHAgent struct {
	ID         int64
	Name       string
	Mode       string
	Version    string
	LastSeenAt *time.Time
	LastReport json.RawMessage
	CreatedAt  time.Time
}

type SSHManaged struct {
	AgentID   int64
	TargetRef string
	UnixUser  string
}

// SSHReport is what RecordSSHReport needs from an agent's report. Raw is kept
// whole for the UI; the other fields change state.
type SSHReport struct {
	Mode       string
	Version    string
	Raw        json.RawMessage
	Applied    []SSHManaged
	Cleared    []SSHManaged
	Mismatched []string
}

// querier is satisfied by both *sql.DB and *sql.Tx, so the list readers can
// run alone or inside SSHState's single read transaction.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// SSHData is everything a sync needs, read at one instant.
type SSHData struct {
	Keys        []SSHKey
	Assignments []SSHAssignment
	Targets     []SSHTarget
	Agents      []SSHAgent
	Managed     []SSHManaged
}

// SSHState reads all SSH tables in one read transaction. Separate reads could
// straddle a write and hand an agent a write and a clear for the same row.
func (s *Store) SSHState(ctx context.Context) (SSHData, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SSHData{}, err
	}
	defer tx.Rollback()

	var d SSHData
	if d.Keys, err = listSSHKeys(ctx, tx, nil); err != nil {
		return SSHData{}, err
	}
	if d.Assignments, err = listSSHAssignments(ctx, tx); err != nil {
		return SSHData{}, err
	}
	if d.Targets, err = listSSHTargets(ctx, tx); err != nil {
		return SSHData{}, err
	}
	if d.Agents, err = listSSHAgents(ctx, tx); err != nil {
		return SSHData{}, err
	}
	if d.Managed, err = listSSHManaged(ctx, tx); err != nil {
		return SSHData{}, err
	}

	return d, nil
}

// sshTx runs fn in a transaction and bumps the SSH revision in the same
// transaction when fn reports a change, so an agent can never see new data
// under an old ETag.
func (s *Store) sshTx(ctx context.Context, fn func(tx *sql.Tx) (bool, error)) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	changed, err := fn(tx)
	if err != nil {
		return err
	}

	if changed {
		if _, err := tx.ExecContext(ctx, `UPDATE ssh_revision SET rev = rev + 1 WHERE id = 1`); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func affected(res sql.Result, err error) (int64, error) {
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func (s *Store) SSHRevision(ctx context.Context) (int64, error) {
	var rev int64
	err := s.db.QueryRowContext(ctx, `SELECT rev FROM ssh_revision WHERE id = 1`).Scan(&rev)

	return rev, err
}

const sshKeyColumns = ` id, headscale_user_id, public_key, fingerprint, label, created_at`

func scanSSHKey(row scanner) (SSHKey, error) {
	var k SSHKey
	err := row.Scan(&k.ID, &k.HeadscaleUserID, &k.PublicKey, &k.Fingerprint, &k.Label, intoTime(&k.CreatedAt))

	return k, err
}

func (s *Store) AddSSHKey(ctx context.Context, k SSHKey, by int64) (SSHKey, error) {
	var out SSHKey

	err := s.sshTx(ctx, func(tx *sql.Tx) (bool, error) {
		row := tx.QueryRowContext(ctx, `
			INSERT INTO ssh_keys (headscale_user_id, public_key, fingerprint, label, created_by)
			VALUES (?, ?, ?, ?, ?) RETURNING`+sshKeyColumns,
			k.HeadscaleUserID, k.PublicKey, k.Fingerprint, k.Label, by)

		var err error
		out, err = scanSSHKey(row)
		if isUniqueViolation(err) {
			return false, ErrDuplicateSSHKey
		}

		return err == nil, err
	})

	return out, err
}

// ListSSHKeys returns keys in id order; nil userID means every user's.
func (s *Store) ListSSHKeys(ctx context.Context, userID *uint64) ([]SSHKey, error) {
	return listSSHKeys(ctx, s.db, userID)
}

func listSSHKeys(ctx context.Context, q querier, userID *uint64) ([]SSHKey, error) {
	query := `SELECT` + sshKeyColumns + ` FROM ssh_keys`
	args := []any{}

	if userID != nil {
		query += ` WHERE headscale_user_id = ?`
		args = append(args, *userID)
	}

	rows, err := q.QueryContext(ctx, query+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SSHKey{}
	for rows.Next() {
		k, err := scanSSHKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}

	return out, rows.Err()
}

func (s *Store) DeleteSSHKey(ctx context.Context, id int64) (SSHKey, error) {
	var out SSHKey

	err := s.sshTx(ctx, func(tx *sql.Tx) (bool, error) {
		var err error
		out, err = scanSSHKey(tx.QueryRowContext(ctx, `DELETE FROM ssh_keys WHERE id = ? RETURNING`+sshKeyColumns, id))

		return err == nil, err
	})

	return out, err
}

func (s *Store) ListSSHAssignments(ctx context.Context) ([]SSHAssignment, error) {
	return listSSHAssignments(ctx, s.db)
}

func listSSHAssignments(ctx context.Context, q querier) ([]SSHAssignment, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT host_name, unix_user, headscale_user_id FROM ssh_assignments
		ORDER BY host_name, unix_user, headscale_user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SSHAssignment{}
	for rows.Next() {
		var a SSHAssignment
		if err := rows.Scan(&a.HostName, &a.UnixUser, &a.HeadscaleUserID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}

	return out, rows.Err()
}

// SetSSHRow replaces who may log in as unixUser on host. An empty list
// removes the row.
func (s *Store) SetSSHRow(ctx context.Context, host, unixUser string, ids []uint64) error {
	return s.sshTx(ctx, func(tx *sql.Tx) (bool, error) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM ssh_assignments WHERE host_name = ? AND unix_user = ?`, host, unixUser); err != nil {
			return false, err
		}

		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO ssh_assignments (host_name, unix_user, headscale_user_id)
				VALUES (?, ?, ?)`, host, unixUser, id); err != nil {
				return false, err
			}
		}

		return true, nil
	})
}

// DeleteSSHHost forgets a host that has left the policy: its rows and binding.
func (s *Store) DeleteSSHHost(ctx context.Context, host string) error {
	return s.sshTx(ctx, func(tx *sql.Tx) (bool, error) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM ssh_assignments WHERE host_name = ?`, host); err != nil {
			return false, err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM ssh_targets WHERE host_name = ?`, host)

		return err == nil, err
	})
}

// DeleteSSHUser forgets a deleted Headscale user's keys and rows.
func (s *Store) DeleteSSHUser(ctx context.Context, id uint64) error {
	return s.sshTx(ctx, func(tx *sql.Tx) (bool, error) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM ssh_keys WHERE headscale_user_id = ?`, id); err != nil {
			return false, err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM ssh_assignments WHERE headscale_user_id = ?`, id)

		return err == nil, err
	})
}

func (s *Store) ListSSHTargets(ctx context.Context) ([]SSHTarget, error) {
	return listSSHTargets(ctx, s.db)
}

func listSSHTargets(ctx context.Context, q querier) ([]SSHTarget, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT host_name, agent_id, target_ref, approved_ip, approved_label, suspended
		FROM ssh_targets ORDER BY host_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SSHTarget{}
	for rows.Next() {
		var t SSHTarget
		if err := rows.Scan(&t.HostName, &t.AgentID, &t.TargetRef, &t.ApprovedIP, &t.ApprovedLabel, &t.Suspended); err != nil {
			return nil, err
		}
		out = append(out, t)
	}

	return out, rows.Err()
}

// ApproveSSHTarget binds a host to a target, or re-approves it, which also
// lifts a suspension.
func (s *Store) ApproveSSHTarget(ctx context.Context, t SSHTarget, by int64) error {
	return s.sshTx(ctx, func(tx *sql.Tx) (bool, error) {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO ssh_targets (host_name, agent_id, target_ref, approved_ip, approved_label, approved_by)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (host_name) DO UPDATE SET
				agent_id = excluded.agent_id, target_ref = excluded.target_ref,
				approved_ip = excluded.approved_ip, approved_label = excluded.approved_label,
				approved_by = excluded.approved_by, approved_at = excluded.approved_at,
				suspended = 0`,
			t.HostName, t.AgentID, t.TargetRef, t.ApprovedIP, t.ApprovedLabel, by)
		if isUniqueViolation(err) {
			return false, ErrTargetTaken
		}

		return err == nil, err
	})
}

func (s *Store) DeleteSSHTarget(ctx context.Context, host string) error {
	return s.sshTx(ctx, func(tx *sql.Tx) (bool, error) {
		n, err := affected(tx.ExecContext(ctx, `DELETE FROM ssh_targets WHERE host_name = ?`, host))
		if err == nil && n == 0 {
			return false, ErrNotFound
		}

		return n > 0, err
	})
}

const sshAgentColumns = ` id, name, mode, version, last_seen_at, last_report, created_at`

func scanSSHAgent(row scanner) (SSHAgent, error) {
	var a SSHAgent
	err := row.Scan(&a.ID, &a.Name, &a.Mode, &a.Version, intoNullTime(&a.LastSeenAt), intoJSON(&a.LastReport), intoTime(&a.CreatedAt))

	return a, err
}

func (s *Store) CreateSSHAgent(ctx context.Context, name, tokenHash string) (SSHAgent, error) {
	a, err := scanSSHAgent(s.db.QueryRowContext(ctx,
		`INSERT INTO ssh_agents (name, token_hash) VALUES (?, ?) RETURNING`+sshAgentColumns, name, tokenHash))
	if isUniqueViolation(err) {
		return SSHAgent{}, ErrAgentNameTaken
	}

	return a, err
}

func (s *Store) ListSSHAgents(ctx context.Context) ([]SSHAgent, error) {
	return listSSHAgents(ctx, s.db)
}

func listSSHAgents(ctx context.Context, q querier) ([]SSHAgent, error) {
	rows, err := q.QueryContext(ctx, `SELECT`+sshAgentColumns+` FROM ssh_agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SSHAgent{}
	for rows.Next() {
		a, err := scanSSHAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}

	return out, rows.Err()
}

func (s *Store) SSHAgentByTokenHash(ctx context.Context, hash string) (SSHAgent, error) {
	return scanSSHAgent(s.db.QueryRowContext(ctx, `SELECT`+sshAgentColumns+` FROM ssh_agents WHERE token_hash = ?`, hash))
}

func (s *Store) RotateSSHAgentToken(ctx context.Context, id int64, hash string) error {
	n, err := affected(s.db.ExecContext(ctx, `UPDATE ssh_agents SET token_hash = ? WHERE id = ?`, hash, id))
	if err == nil && n == 0 {
		return ErrNotFound
	}

	return err
}

// DeleteSSHAgent revokes an agent; its bindings and managed rows go with it
// through ON DELETE CASCADE.
// DeleteSSHAgent removes an agent with its token, bindings and managed rows,
// and returns those managed rows as they were at the moment of the delete:
// the blocks still written that the revoke's cleanup commands must cover.
func (s *Store) DeleteSSHAgent(ctx context.Context, id int64) ([]SSHManaged, error) {
	var gone []SSHManaged

	err := s.sshTx(ctx, func(tx *sql.Tx) (bool, error) {
		all, err := listSSHManaged(ctx, tx)
		if err != nil {
			return false, err
		}
		for _, m := range all {
			if m.AgentID == id {
				gone = append(gone, m)
			}
		}

		n, err := affected(tx.ExecContext(ctx, `DELETE FROM ssh_agents WHERE id = ?`, id))
		if err == nil && n == 0 {
			return false, ErrNotFound
		}

		return true, err
	})

	return gone, err
}

// RetireSSHAgent records the blocks an uninstalling agent removed and deletes
// the agent, token and bindings with it, once none of its blocks remain. While
// some remain it returns them and keeps the agent, so Headboard never forgets
// keys still sitting on a server.
func (s *Store) RetireSSHAgent(ctx context.Context, agentID int64, cleared []SSHManaged) ([]SSHManaged, error) {
	var left []SSHManaged

	err := s.sshTx(ctx, func(tx *sql.Tx) (bool, error) {
		for _, m := range cleared {
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM ssh_managed WHERE agent_id = ? AND target_ref = ? AND unix_user = ?`,
				agentID, m.TargetRef, m.UnixUser); err != nil {
				return false, err
			}
		}

		all, err := listSSHManaged(ctx, tx)
		if err != nil {
			return false, err
		}
		left = []SSHManaged{}
		for _, m := range all {
			if m.AgentID == agentID {
				left = append(left, m)
			}
		}
		if len(left) > 0 {
			return len(cleared) > 0, nil
		}

		n, err := affected(tx.ExecContext(ctx, `DELETE FROM ssh_agents WHERE id = ?`, agentID))
		if err == nil && n == 0 {
			return false, ErrNotFound
		}

		return true, err
	})

	return left, err
}

func (s *Store) ListSSHManaged(ctx context.Context) ([]SSHManaged, error) {
	return listSSHManaged(ctx, s.db)
}

func listSSHManaged(ctx context.Context, q querier) ([]SSHManaged, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT agent_id, target_ref, unix_user FROM ssh_managed
		ORDER BY agent_id, target_ref, unix_user`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SSHManaged{}
	for rows.Next() {
		var m SSHManaged
		if err := rows.Scan(&m.AgentID, &m.TargetRef, &m.UnixUser); err != nil {
			return nil, err
		}
		out = append(out, m)
	}

	return out, rows.Err()
}

// RecordSSHReport stores an agent's report. It bumps the revision only when
// the report changed managed rows or suspended a binding: agents report every
// 30 seconds, and bumping on each would turn every 304 into a full re-apply.
func (s *Store) RecordSSHReport(ctx context.Context, agentID int64, r SSHReport) error {
	return s.sshTx(ctx, func(tx *sql.Tx) (bool, error) {
		if _, err := tx.ExecContext(ctx, `
			UPDATE ssh_agents SET mode = ?, version = ?, last_seen_at = ?, last_report = ?
			WHERE id = ?`, r.Mode, r.Version, nowText(), nullJSON(r.Raw), agentID); err != nil {
			return false, err
		}

		var changed int64

		for _, m := range r.Applied {
			n, err := affected(tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO ssh_managed (agent_id, target_ref, unix_user) VALUES (?, ?, ?)`,
				agentID, m.TargetRef, m.UnixUser))
			if err != nil {
				return false, err
			}
			changed += n
		}

		for _, m := range r.Cleared {
			n, err := affected(tx.ExecContext(ctx, `
				DELETE FROM ssh_managed WHERE agent_id = ? AND target_ref = ? AND unix_user = ?`,
				agentID, m.TargetRef, m.UnixUser))
			if err != nil {
				return false, err
			}
			changed += n
		}

		for _, ref := range r.Mismatched {
			n, err := affected(tx.ExecContext(ctx, `
				UPDATE ssh_targets SET suspended = 1
				WHERE agent_id = ? AND target_ref = ? AND suspended = 0`, agentID, ref))
			if err != nil {
				return false, err
			}
			changed += n
		}

		return changed > 0, nil
	})
}
