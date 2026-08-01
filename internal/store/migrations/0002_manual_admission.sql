-- People who became members through the former automatic-login path must be
-- approved before they can use Headboard. Elevated roles were assigned by an
-- operator, so preserve their existing access and avoid a management lockout.
ALTER TABLE users ADD COLUMN admission TEXT NOT NULL DEFAULT 'active';

UPDATE users SET admission = 'pending' WHERE role = 'member';
