CREATE UNIQUE INDEX IF NOT EXISTS uq_orders_one_open_per_table
    ON orders (table_id)
    WHERE status NOT IN ('completed','cancelled');
