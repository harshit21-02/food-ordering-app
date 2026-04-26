-- v4: enable the staff/admin dashboard.
--
-- Two changes:
-- 1. Add `staff` to the staff_users role enum (alongside super_admin and manager).
--    "manager" = branch admin (full control of one cafe).
--    "staff"   = order operations only (no menu/tables/staff CRUD).
-- 2. Add `staff_id` to auth_sessions so staff/admin OTP sessions can record who
--    they belong to (mirrors `customer_id` for the customer flow).

BEGIN;

ALTER TABLE staff_users DROP CONSTRAINT IF EXISTS staff_users_role_check;
ALTER TABLE staff_users
    ADD CONSTRAINT staff_users_role_check
    CHECK (role IN ('super_admin','manager','staff'));

ALTER TABLE auth_sessions
    ADD COLUMN staff_id BIGINT REFERENCES staff_users(id);

CREATE INDEX idx_auth_sessions_staff
    ON auth_sessions (staff_id)
    WHERE staff_id IS NOT NULL;

COMMIT;
