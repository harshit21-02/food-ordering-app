-- Rollback initial schema. Order matters: drop dependents first.
DROP TABLE IF EXISTS payments       CASCADE;
DROP TABLE IF EXISTS order_items    CASCADE;
DROP TABLE IF EXISTS orders         CASCADE;
DROP TABLE IF EXISTS menu           CASCADE;
DROP TABLE IF EXISTS tables         CASCADE;
DROP TABLE IF EXISTS staff_users    CASCADE;
DROP TABLE IF EXISTS auth_sessions  CASCADE;
DROP TABLE IF EXISTS customers      CASCADE;
DROP TABLE IF EXISTS organisations  CASCADE;
