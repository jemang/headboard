-- Bumped when a password or PIN is reset from the command line. Sessions carry
-- the version they signed in with, so a bump signs that person out everywhere.
ALTER TABLE users ADD COLUMN cred_version INTEGER NOT NULL DEFAULT 0;
