-- Sudo mode: a six-digit PIN confirms dangerous actions. Failures are counted
-- here rather than in memory so a restart or a second browser does not reset
-- them, and a lock outlives the session that earned it.
ALTER TABLE users ADD COLUMN pin_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN pin_failures INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN pin_locked_until TEXT;
