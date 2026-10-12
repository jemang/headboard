-- Sudo grants live here, keyed by a hash of the session token, rather than in
-- the session's own data: writing the session to grant sudo could bring back a
-- session that a logout deleted a moment earlier.
CREATE TABLE sudo_grants (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    until      TEXT NOT NULL
);
