-- Rollback v3.
BEGIN;

DROP INDEX IF EXISTS idx_auth_sessions_email;
ALTER TABLE auth_sessions ALTER COLUMN mobile_number SET NOT NULL;
ALTER TABLE auth_sessions DROP COLUMN IF EXISTS email;

DROP INDEX IF EXISTS uq_customers_email;
DROP INDEX IF EXISTS uq_customers_mobile;
ALTER TABLE customers ALTER COLUMN mobile_number SET NOT NULL;
ALTER TABLE customers ADD CONSTRAINT customers_mobile_number_key UNIQUE (mobile_number);
ALTER TABLE customers DROP COLUMN IF EXISTS email;

COMMIT;
