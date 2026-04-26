# Menu Rendering — Schema & Seed Reference

This file lists the **three tables** involved in rendering the menu on a customer's screen after they scan the QR code, plus ready-to-run `CREATE TABLE` statements and sample `INSERT`s. Hand this to whoever is seeding the database — it's everything they need.

## Render flow (for context)

When a customer scans `https://<app>/o/{org_id}/t/{table_code}`:

1. Frontend calls `GET /public/context?org_id=...&table_code=...` → backend reads **`organisations`** and **`tables`** to validate the QR and return the cafe name + table label.
2. After OTP login, frontend calls `GET /orgs/{org_id}/menu` → backend reads **`menu`** filtered by `org_id` and `is_available=true`, ordered by `category` then `display_order`.

So the three tables required are: `organisations`, `tables`, `menu`.

---

## 1. `organisations`

One row per cafe.

```sql
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
```

### Sample insert

```sql
INSERT INTO organisations (name, address, contact_phone, contact_email)
VALUES ('Velvet Frost Cafe', '12 MG Road, Bengaluru', '+919876543210', 'hello@velvetfrost.cafe');
-- assume this returns id = 1
```

---

## 2. `tables`

Physical tables in a cafe. The `code` column is what goes into the QR URL (keep it short and opaque — a nanoid like `t_a7Kx9` is good). `label` is the human-friendly name shown in the UI ("Table 5", "Window 2").

```sql
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

CREATE INDEX idx_tables_org_id ON tables(org_id);
```

### Sample insert

```sql
INSERT INTO tables (org_id, code, label) VALUES
    (1, 't_a7Kx9', 'Table 1'),
    (1, 't_b2Mn4', 'Table 2'),
    (1, 't_c5Pq7', 'Window Seat');
```

The QR for Table 1 would encode: `https://<app>/o/1/t/t_a7Kx9`.

---

## 3. `menu`

The menu items themselves. `category` is a free-form text field (e.g. `"Beverages"`, `"Starters"`, `"Desserts"`) — no separate categories table. `display_order` controls sort within a category. `is_available=false` hides an item from customers without deleting it.

```sql
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
```

### Column reference

| column | required | notes |
|---|---|---|
| `org_id` | yes | which cafe this item belongs to |
| `name` | yes | shown in the menu list |
| `description` | no | shown under the name |
| `category` | no (but recommended) | grouping header in the UI; free-form string |
| `price` | yes | in rupees, two decimals (e.g. `120.00`) |
| `image_url` | no | absolute URL to the item image |
| `display_order` | no (defaults 0) | lower numbers appear first within the category |
| `is_available` | no (defaults true) | set false to hide without deleting |

### Sample insert

```sql
INSERT INTO menu (org_id, name, description, category, price, image_url, display_order)
VALUES
    (1, 'Espresso',         'Single shot, robust',                          'Beverages', 120.00, NULL, 1),
    (1, 'Cappuccino',       'Espresso with steamed milk and foam',          'Beverages', 180.00, NULL, 2),
    (1, 'Iced Latte',       'Chilled latte over ice',                       'Beverages', 200.00, NULL, 3),
    (1, 'Avocado Toast',    'Sourdough, smashed avocado, chilli flakes',    'Starters',  280.00, NULL, 1),
    (1, 'Caesar Salad',     'Romaine, parmesan, croutons',                  'Starters',  320.00, NULL, 2),
    (1, 'Tiramisu',         'Classic Italian dessert',                      'Desserts',  240.00, NULL, 1),
    (1, 'Brownie Sundae',   'Warm brownie, vanilla ice cream',              'Desserts',  220.00, NULL, 2);
```

---

## Putting it all together — minimal seed script

Run this once on a fresh DB to get a working menu visible to customers:

```sql
-- 1. The cafe
INSERT INTO organisations (name, address, contact_phone, contact_email)
VALUES ('Velvet Frost Cafe', '12 MG Road, Bengaluru', '+919876543210', 'hello@velvetfrost.cafe');

-- 2. A few tables (use the returned org id; here we assume id=1)
INSERT INTO tables (org_id, code, label) VALUES
    (1, 't_a7Kx9', 'Table 1'),
    (1, 't_b2Mn4', 'Table 2');

-- 3. The menu
INSERT INTO menu (org_id, name, description, category, price, display_order) VALUES
    (1, 'Espresso',      'Single shot, robust',                       'Beverages', 120.00, 1),
    (1, 'Cappuccino',    'Espresso with steamed milk and foam',       'Beverages', 180.00, 2),
    (1, 'Avocado Toast', 'Sourdough, smashed avocado, chilli flakes', 'Starters',  280.00, 1),
    (1, 'Tiramisu',      'Classic Italian dessert',                   'Desserts',  240.00, 1);
```

After running this, scanning a QR encoding `https://<app>/o/1/t/t_a7Kx9` should land the customer on Velvet Frost Cafe – Table 1, and after OTP login they'll see all four items grouped into Beverages / Starters / Desserts.

---

## Notes for whoever is inserting data

- **Always include `org_id`** on `tables` and `menu` rows. Multi-tenancy depends on it; a row without `org_id` will be invisible to that org's customers.
- **Don't reuse `code` values across orgs**? Actually you can — the unique constraint is `(org_id, code)`, so `t_001` is fine in two different cafes. But do keep them unique within an org.
- **Price** is `NUMERIC(10,2)` — write `120.00`, not `120` (technically both work, but stick to two decimals to keep the data clean).
- **`is_available = false`** is the correct way to "remove" an item from the menu. Don't `DELETE` from `menu` if any past order ever referenced the item — `order_items.menu_item_id` FKs to `menu`.
