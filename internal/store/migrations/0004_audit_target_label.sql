-- A readable name for what an entry changed (a device or user name, an SSH
-- host), kept with the entry because the target itself may be gone by the
-- time anyone reads the log.
ALTER TABLE audit_log ADD COLUMN target_label TEXT NOT NULL DEFAULT '';
