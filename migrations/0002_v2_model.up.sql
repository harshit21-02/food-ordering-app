-- v2 model deltas — see docs/design.md "Locked decisions (v2)".
--
-- 1. Status enum changes: pending/in_progress/completed/cancelled
--    becomes queued/cooking/prepared/completed/cancelled.
-- 2. Orders are now table-scoped (customer_id nullable).
-- 3. Track who added each order_item (added_by_customer_id).
-- 4. Enforce one open order per table via partial unique index.
-- 5. staff_users now login via phone + OTP. org_id nullable (super_admin
--    has no org). password_hash + email become optional.
-- 6. role values constrained to ('super_admin','manager').

BEGIN;

-- ----- orders -----------------------------------------------------------------

-- Replace the inline status CHECK with the new enum.
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;

-- Migrate any existing data to the new enum (for safety; dev DB is empty).
UPDATE orders SET status = 'queued'    WHERE status = 'pending';
UPDATE orders SET status = 'cooking'   WHERE status = 'in_progress';

ALTER TABLE orders
    ADD CONSTRAINT orders_status_check
    CHECK (status IN ('queued','cooking','prepared','completed','cancelled'));

ALTER TABLE orders ALTER COLUMN status SET DEFAULT 'queued';

-- Orders are table-scoped now; customer_id is the *initiator* but optional.
ALTER TABLE orders ALTER COLUMN customer_id DROP NOT NULL;

-- One open order per table.
CREATE UNIQUE INDEX uq_orders_one_open_per_table
    ON orders (table_id)
    WHERE status NOT IN ('completed','cancelled');

-- ----- order_items ------------------------------------------------------------

-- Who added this line. Nullable to keep backfills clean; new code always sets it.
ALTER TABLE order_items
    ADD COLUMN added_by_customer_id BIGINT REFERENCES customers(id);

CREATE INDEX idx_order_items_added_by ON order_items(added_by_customer_id);

-- ----- staff_users ------------------------------------------------------------

-- Loosen email + password requirements (we use phone+OTP now). Allow super_admin
-- to exist without an org.
ALTER TABLE staff_users ALTER COLUMN email         DROP NOT NULL;
ALTER TABLE staff_users ALTER COLUMN password_hash DROP NOT NULL;
ALTER TABLE staff_users ALTER COLUMN org_id        DROP NOT NULL;

-- Add the phone column. Sparse-unique so multiple seed runs don't collide on
-- NULLs and we can have rows without a phone during transition (none in dev).
ALTER TABLE staff_users ADD COLUMN mobile_number TEXT;

CREATE UNIQUE INDEX uq_staff_users_mobile
    ON staff_users (mobile_number)
    WHERE mobile_number IS NOT NULL;

-- Roles are constrained to the three known values.
ALTER TABLE staff_users
    ADD CONSTRAINT staff_users_role_check
    CHECK (role IN ('super_admin','manager'));

ALTER TABLE staff_users ALTER COLUMN role SET DEFAULT 'manager';

COMMIT;
