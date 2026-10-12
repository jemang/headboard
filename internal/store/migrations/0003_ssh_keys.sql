-- SSH key sync. Keys belong to Headscale users by id (names can collide);
-- servers are named by the policy's hosts map and bound to a physical target
-- (a Proxmox VMID, or 'self') only by an admin's approval.

CREATE TABLE ssh_keys (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    headscale_user_id INTEGER NOT NULL,
    public_key        TEXT NOT NULL,
    fingerprint       TEXT NOT NULL UNIQUE,
    label             TEXT NOT NULL DEFAULT '',
    created_by        INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX ssh_keys_user_idx ON ssh_keys (headscale_user_id);

CREATE TABLE ssh_agents (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL UNIQUE,
    token_hash   TEXT NOT NULL UNIQUE,
    mode         TEXT NOT NULL DEFAULT '',
    version      TEXT NOT NULL DEFAULT '',
    last_seen_at TEXT,
    last_report  TEXT,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE ssh_targets (
    host_name      TEXT PRIMARY KEY,
    agent_id       INTEGER NOT NULL REFERENCES ssh_agents (id) ON DELETE CASCADE,
    target_ref     TEXT NOT NULL,
    approved_ip    TEXT NOT NULL,
    approved_label TEXT NOT NULL DEFAULT '',
    suspended      INTEGER NOT NULL DEFAULT 0,
    approved_by    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    approved_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),

    -- One manager per physical target: two hosts entries, or two agents,
    -- writing the same file would erase each other's keys.
    UNIQUE (agent_id, target_ref)
);

CREATE TABLE ssh_assignments (
    host_name         TEXT NOT NULL,
    unix_user         TEXT NOT NULL,
    headscale_user_id INTEGER NOT NULL,
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    PRIMARY KEY (host_name, unix_user, headscale_user_id)
);

-- Every block an agent reported writing. One with no active assignment left
-- is a pending clear until the agent reports clearing it.
CREATE TABLE ssh_managed (
    agent_id   INTEGER NOT NULL REFERENCES ssh_agents (id) ON DELETE CASCADE,
    target_ref TEXT NOT NULL,
    unix_user  TEXT NOT NULL,
    PRIMARY KEY (agent_id, target_ref, unix_user)
);

CREATE TABLE ssh_revision (
    id  INTEGER PRIMARY KEY CHECK (id = 1),
    rev INTEGER NOT NULL
);

INSERT INTO ssh_revision (id, rev) VALUES (1, 0);
