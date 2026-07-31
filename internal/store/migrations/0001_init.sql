-- Headboard owns accounts, sessions, audit, policy history and preferences. It
-- mirrors no tailnet state: nodes, users, keys and the policy are always read
-- live from Headscale, which stays the single source of truth for those.
--
-- SQLite dialect. Timestamps are RFC3339 UTC text so they sort lexicographically
-- and round-trip through Go without depending on driver conversion.

CREATE TABLE users (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,

    -- Identity. For an OIDC account these are the issuer and subject, the same
    -- pair Headscale concatenates into provider_identifier, which is what lets
    -- an account be matched to the Headscale user that owns the devices.
    --
    -- For a local password account, oidc_iss is the literal 'local' and
    -- oidc_sub is the lowercased email. Reusing the columns rather than adding
    -- a parallel concept keeps UNIQUE (oidc_iss, oidc_sub) meaningful for both
    -- kinds and leaves every existing query unchanged.
    oidc_iss           TEXT NOT NULL,
    oidc_sub           TEXT NOT NULL,

    -- Empty for OIDC accounts. argon2id encoded string for local accounts.
    password_hash      TEXT NOT NULL DEFAULT '',

    email              TEXT NOT NULL DEFAULT '',
    display_name       TEXT NOT NULL DEFAULT '',
    avatar_url         TEXT NOT NULL DEFAULT '',

    -- The Headscale user this account is linked to. Null when no match was
    -- found: users created with `headscale users create` have no
    -- provider_identifier, so they need an explicit link by an admin.
    headscale_user_id  INTEGER,

    role               TEXT NOT NULL,

    created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    last_login_at      TEXT,

    UNIQUE (oidc_iss, oidc_sub)
);

-- One Headscale user must not be claimed by two accounts, or two people would
-- each see the other's devices as their own.
CREATE UNIQUE INDEX users_headscale_user_id_key
    ON users (headscale_user_id)
    WHERE headscale_user_id IS NOT NULL;

-- Managed by the scs store in sessions.go.
CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry TEXT NOT NULL
);

CREATE INDEX sessions_expiry_idx ON sessions (expiry);

-- Headscale records no audit trail of its own, so every mutation Headboard
-- performs is logged here with enough context to answer "who changed this".
CREATE TABLE audit_log (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    actor_label   TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL,
    target_type   TEXT NOT NULL,
    target_id     TEXT NOT NULL DEFAULT '',
    before        TEXT,
    after         TEXT,
    ip            TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX audit_log_created_at_idx ON audit_log (created_at DESC);
CREATE INDEX audit_log_target_idx ON audit_log (target_type, target_id);

-- Every accepted policy write is snapshotted, which is what makes one-click
-- rollback possible. Headscale keeps only the current document.
CREATE TABLE policy_revisions (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    sha256         TEXT NOT NULL,
    body           TEXT NOT NULL,
    author_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    note           TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX policy_revisions_created_at_idx ON policy_revisions (created_at DESC);

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
