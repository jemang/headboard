package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

)

// AuditEntry is one recorded mutation. Headscale keeps no history of its own,
// so this is the only place that can answer "who tagged that node".
type AuditEntry struct {
	ID          int64           `json:"id"`
	ActorUserID *int64          `json:"actorUserId,omitempty"`
	ActorLabel  string          `json:"actorLabel"`
	Action      string          `json:"action"`
	TargetType  string          `json:"targetType"`
	TargetID    string          `json:"targetId,omitempty"`
	Before      json.RawMessage `json:"before,omitempty"`
	After       json.RawMessage `json:"after,omitempty"`
	IP          string          `json:"ip,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// Audit records a mutation.
func (s *Store) Audit(ctx context.Context, e AuditEntry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_log (actor_user_id, actor_label, action, target_type,
		                       target_id, before, after, ip)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ActorUserID, e.ActorLabel, e.Action, e.TargetType, e.TargetID,
		nullJSON(e.Before), nullJSON(e.After), e.IP,
	)
	if err != nil {
		return fmt.Errorf("writing audit entry: %w", err)
	}

	return nil
}

// AuditFilter narrows a log query. Zero values mean "no constraint".
type AuditFilter struct {
	Action     string
	TargetType string
	TargetID   string
	Limit      int
	Before     int64
}

// ListAudit returns entries newest first, keyset-paginated on id.
func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, actor_user_id, actor_label, action, target_type, target_id,
		       before, after, ip, created_at
		FROM audit_log
		WHERE (? = '' OR action = ?)
		  AND (? = '' OR target_type = ?)
		  AND (? = '' OR target_id = ?)
		  AND (? = 0 OR id < ?)
		ORDER BY id DESC
		LIMIT ?`,
		f.Action, f.Action, f.TargetType, f.TargetType,
		f.TargetID, f.TargetID, f.Before, f.Before, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("listing audit log: %w", err)
	}
	defer rows.Close()

	var out []AuditEntry

	for rows.Next() {
		var e AuditEntry

		if err := rows.Scan(&e.ID, &e.ActorUserID, &e.ActorLabel, &e.Action,
			&e.TargetType, &e.TargetID, intoJSON(&e.Before), intoJSON(&e.After),
			&e.IP, intoTime(&e.CreatedAt)); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}

		out = append(out, e)
	}

	return out, rows.Err()
}

// PolicyRevision is one accepted version of the ACL document.
type PolicyRevision struct {
	ID           int64     `json:"id"`
	SHA256       string    `json:"sha256"`
	Body         string    `json:"body,omitempty"`
	AuthorUserID *int64    `json:"authorUserId,omitempty"`
	Note         string    `json:"note,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// SavePolicyRevision snapshots a policy document.
func (s *Store) SavePolicyRevision(ctx context.Context, rev PolicyRevision) (PolicyRevision, error) {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO policy_revisions (sha256, body, author_user_id, note)
		VALUES (?, ?, ?, ?)
		RETURNING id, sha256, body, author_user_id, note, created_at`,
		rev.SHA256, rev.Body, rev.AuthorUserID, rev.Note)

	var out PolicyRevision

	err := row.Scan(&out.ID, &out.SHA256, &out.Body, &out.AuthorUserID, &out.Note,
		intoTime(&out.CreatedAt))
	if err != nil {
		return PolicyRevision{}, fmt.Errorf("saving policy revision: %w", err)
	}

	return out, nil
}

// ListPolicyRevisions returns the history, newest first, without bodies.
func (s *Store) ListPolicyRevisions(ctx context.Context, limit int) ([]PolicyRevision, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, sha256, author_user_id, note, created_at
		FROM policy_revisions ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing policy revisions: %w", err)
	}
	defer rows.Close()

	var out []PolicyRevision

	for rows.Next() {
		var r PolicyRevision

		if err := rows.Scan(&r.ID, &r.SHA256, &r.AuthorUserID, &r.Note,
			intoTime(&r.CreatedAt)); err != nil {
			return nil, fmt.Errorf("scanning policy revision: %w", err)
		}

		out = append(out, r)
	}

	return out, rows.Err()
}

// PolicyRevision returns one revision including its body.
func (s *Store) PolicyRevision(ctx context.Context, id int64) (PolicyRevision, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, sha256, body, author_user_id, note, created_at
		FROM policy_revisions WHERE id = ?`, id)

	var r PolicyRevision

	err := row.Scan(&r.ID, &r.SHA256, &r.Body, &r.AuthorUserID, &r.Note,
		intoTime(&r.CreatedAt))

	return r, err
}

// LatestPolicyRevision returns the most recent snapshot, if any.
func (s *Store) LatestPolicyRevision(ctx context.Context) (PolicyRevision, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, sha256, body, author_user_id, note, created_at
		FROM policy_revisions ORDER BY id DESC LIMIT 1`)

	var r PolicyRevision

	err := row.Scan(&r.ID, &r.SHA256, &r.Body, &r.AuthorUserID, &r.Note,
		intoTime(&r.CreatedAt))

	return r, err
}

func nullJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}

	// String rather than []byte: the column is TEXT, and a []byte would be
	// stored as a BLOB, which reads back as base64 through the JSON API.
	return string(raw)
}

// jsonScanner reads a nullable JSON column into a json.RawMessage.
type jsonScanner struct{ dst *json.RawMessage }

func intoJSON(dst *json.RawMessage) jsonScanner { return jsonScanner{dst: dst} }

func (j jsonScanner) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j.dst = nil
	case []byte:
		*j.dst = json.RawMessage(append([]byte(nil), v...))
	case string:
		*j.dst = json.RawMessage(v)
	default:
		return fmt.Errorf("cannot read %T as json", src)
	}

	return nil
}
