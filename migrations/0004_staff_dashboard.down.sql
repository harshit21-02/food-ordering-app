BEGIN;

DROP INDEX IF EXISTS idx_auth_sessions_staff;
ALTER TABLE auth_sessions DROP COLUMN IF EXISTS staff_id;

ALTER TABLE staff_users DROP CONSTRAINT IF EXISTS staff_users_role_check;
ALTER TABLE staff_users
    ADD CONSTRAINT staff_users_role_check
    CHECK (role IN ('super_admin','manager'));

COMMIT;
