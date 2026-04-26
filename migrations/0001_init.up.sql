-- Cafe Ordering System — initial schema
-- Mirrors docs/design.md.

-- 1. organisations
CREATE TABLE organisations (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name            TEXT NOT NULL,
    address         TEXT,
    contact_phone   TEXT,
    contact_email   TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. customers — global across orgs
CREATE TABLE customers (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    mobile_number   TEXT NOT NULL UNIQUE,
    name            TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. auth_sessions — combined OTP + JWT session record
CREATE TABLE auth_sessions (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    mobile_number       TEXT NOT NULL,
    customer_id         BIGINT REFERENCES customers(id),
    code_hash           TEXT NOT NULL,
    code_expires_at     TIMESTAMPTZ NOT NULL,
    attempts            INT NOT NULL DEFAULT 0,
    verified_at         TIMESTAMPTZ,
    jwt_id              TEXT,
    session_expires_at  TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_auth_sessions_mobile ON auth_sessions(mobile_number, code_expires_at DESC);
CREATE UNIQUE INDEX idx_auth_sessions_jwt_id ON auth_sessions(jwt_id) WHERE jwt_id IS NOT NULL;

-- 4. staff_users — cafe-side admins, scoped per org
CREATE TABLE staff_users (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organisations(id),
    email           TEXT NOT NULL,
    password_hash   TEXT NOT NULL,
    name            TEXT,
    role            TEXT NOT NULL DEFAULT 'admin',
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, email)
);
CREATE INDEX idx_staff_users_org ON staff_users(org_id);

-- 5. tables — physical tables in a cafe
CREATE TABLE tables (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organisations(id),
    code            TEXT NOT NULL,
    label           TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, code)
);
CREATE INDEX idx_tables_org ON tables(org_id);

-- 6. menu — items each cafe sells
CREATE TABLE menu (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organisations(id),
    name            TEXT NOT NULL,
    description     TEXT,
    category        TEXT,
    price           NUMERIC(10,2) NOT NULL,
    image_url       TEXT,
    display_order   INT NOT NULL DEFAULT 0,
    is_available    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_menu_org_available ON menu(org_id, is_available);
CREATE INDEX idx_menu_org_category  ON menu(org_id, category);

-- 7. orders — header
CREATE TABLE orders (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_code     TEXT NOT NULL UNIQUE,
    org_id          BIGINT NOT NULL REFERENCES organisations(id),
    table_id        BIGINT NOT NULL REFERENCES tables(id),
    customer_id     BIGINT NOT NULL REFERENCES customers(id),
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','in_progress','completed','cancelled')),
    total_amount    NUMERIC(10,2) NOT NULL,
    is_paid         BOOLEAN NOT NULL DEFAULT FALSE,
    placed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_orders_org_status_placed ON orders(org_id, status, placed_at DESC);
CREATE INDEX idx_orders_table_status      ON orders(table_id, status);
CREATE INDEX idx_orders_customer_placed   ON orders(customer_id, placed_at DESC);

-- 8. order_items — lines, with name+price snapshots
CREATE TABLE order_items (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id        BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    org_id          BIGINT NOT NULL REFERENCES organisations(id),
    menu_item_id    BIGINT REFERENCES menu(id),
    item_name       TEXT NOT NULL,
    unit_price      NUMERIC(10,2) NOT NULL,
    quantity        INT NOT NULL CHECK (quantity > 0),
    line_total      NUMERIC(10,2) GENERATED ALWAYS AS (unit_price * quantity) STORED,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_order_items_order    ON order_items(order_id);
CREATE INDEX idx_order_items_org_menu ON order_items(org_id, menu_item_id);

-- 9. payments
CREATE TABLE payments (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id        BIGINT NOT NULL REFERENCES orders(id),
    org_id          BIGINT NOT NULL REFERENCES organisations(id),
    method          TEXT NOT NULL CHECK (method IN ('cash','card','upi','online')),
    amount          NUMERIC(10,2) NOT NULL,
    txn_ref         TEXT,
    paid_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_payments_order ON payments(order_id);
