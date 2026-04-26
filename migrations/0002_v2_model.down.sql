-- Rollback the v2 deltas. Mirrors 0002_v2_model.up.sql in reverse order.

BEGIN;

-- ----- staff_users ------------------------------------------------------------

ALTER TABLE staff_users ALTER COLUMN role SET DEFAULT 'admin';
ALTER TABLE staff_users DROP CONSTRAINT IF EXISTS staff_users_role_check;

DROP INDEX IF EXISTS uq_staff_users_mobile;
ALTER TABLE staff_users DROP COLUMN IF EXISTS mobile_number;

-- Restoring NOT NULL only succeeds if the table has no rows that violate it.
-- That's true on a clean rollback after wiping the dev seed.
ALTER TABLE staff_users ALTER COLUMN org_id        SET NOT NULL;
ALTER TABLE staff_users ALTER COLUMN password_hash SET NOT NULL;
ALTER TABLE staff_users ALTER COLUMN email         SET NOT NULL;

-- ----- order_items ------------------------------------------------------------

DROP INDEX IF EXISTS idx_order_items_added_by;
ALTER TABLE order_items DROP COLUMN IF EXISTS added_by_customer_id;

-- ----- orders -----------------------------------------------------------------

DROP INDEX IF EXISTS uq_orders_one_open_per_table;

ALTER TABLE orders ALTER COLUMN customer_id SET NOT NULL;

UPDATE orders SET status = 'pending'     WHERE status = 'queued';
UPDATE orders SET status = 'in_progress' WHERE status = 'cooking';
UPDATE orders SET status = 'pending'     WHERE status = 'prepared';

ALTER TABLE orders ALTER COLUMN status SET DEFAULT 'pending';
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE orders
    ADD CONSTRAINT orders_status_check
    CHECK (status IN ('pending','in_progress','completed','cancelled'));

COMMIT;
