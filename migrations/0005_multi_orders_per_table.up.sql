-- Allow multiple open orders per table so each customer submission creates a
-- fresh order visible to staff rather than silently appending to an existing one.
DROP INDEX IF EXISTS uq_orders_one_open_per_table;
