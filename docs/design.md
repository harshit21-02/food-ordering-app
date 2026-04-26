# Cafe Food Ordering System — Design (v2)

## Overview

Multi-tenant restaurant ordering system. **Three role tiers**: super admin (registers cafes + their managers), manager (runs one cafe — menu, tables, orders), customer (scans a table QR, logs in with mobile + OTP, orders).

Order ownership is **table-scoped**, not customer-scoped: at most one open order exists per physical table. Anyone at the table can append to the open order; nobody can remove or reduce an existing line. The order finalises when the manager marks it COMPLETED with the paid checkbox ticked.

### Stack

- **Backend**: Go + Gin + GORM
- **Frontend**: React + Vite + TypeScript
- **Database**: Postgres 16 (via Docker)
- **Auth**: phone + OTP for all three role tiers; HS256 JWT (separate audiences for customer vs staff).
- **File storage** (menu images): local filesystem under `backend/uploads/menu/` for v1.

### Open TODOs (do later)

- **Migrate menu image storage to Cloudinary** (or S3). Local filesystem is fine for dev / single-host but doesn't survive container rebuilds and won't scale across multiple backend instances.
- **Real SMS provider** for OTP (MSG91 / Twilio / Fast2SMS). Currently OTPs are returned in the API response as `dev_otp` for testing.

### Core design choices

- **Multi-tenant** via `org_id` on every tenant-scoped row (shared schema, shared DB).
- **Customers are global** — one mobile number works across all cafes.
- **Order naming**: `orders` is the header (one per *table session*); `order_items` are the lines, each tagged with the customer who added it.
- **One open order per table** is enforced by a partial unique index on `orders(table_id) WHERE status NOT IN ('completed','cancelled')`.
- **Append-only edits** while an order is open. Quantity changes are upserted as new line items in v1 (we never UPDATE existing lines once added).
- **Order status** flows through `QUEUED → COOKING → PREPARED → COMPLETED`. `CANCELLED` is reachable from any non-terminal state by the manager only.
- **Staff phone+OTP login** uses the same `auth_sessions` table as customers; the JWT's `aud_kind` claim distinguishes `customer` / `manager` / `super_admin`.
- **QR URL shape**: `https://<app>/o/{org_id}/t/{table_code}` — frontend extracts both from the route and uses them in API calls.

---

## Schema

Conventions:
- `id` is `bigint generated always as identity` unless noted.
- All tables have `created_at timestamptz default now()`, `updated_at timestamptz default now()`.
- `org_id` is denormalized onto every tenant-scoped table for clean tenant-scoped indexes and future row-level security.
- Money stored as `numeric(10,2)`.

### `organisations`

Represents one cafe.

| column | type | notes |
|---|---|---|
| id | bigint PK identity | |
| name | text not null | |
| address | text | |
| contact_phone | text | |
| contact_email | text | |
| is_active | boolean default true | soft-disable an org |

### `customers` — global

End users who scan and order. Global across all orgs.

| column | type | notes |
|---|---|---|
| id | bigint PK identity | |
| mobile_number | text not null unique | E.164 format, e.g. `+919876543210` |
| name | text | optional, may be captured later at first order |

### `auth_sessions` — combined OTP + session

A row's lifecycle: created when OTP is requested, becomes an active session once OTP is verified and a JWT is issued. Same row holds both halves.

| column | type | notes |
|---|---|---|
| id | bigint PK identity | |
| mobile_number | text not null | indexed; row exists before customer row may exist |
| customer_id | bigint fk → customers | null until OTP verified (customer is upserted on verify) |
| code_hash | text not null | bcrypt/argon2 hash of the OTP |
| code_expires_at | timestamptz not null | typically now() + 5 min |
| attempts | int default 0 | rate-limit verification tries |
| verified_at | timestamptz | set when OTP matched |
| jwt_id | text | the JWT's `jti` claim — set on verify; used for revocation |
| session_expires_at | timestamptz | JWT exp; set on verify (e.g. now() + 30 days) |
| revoked_at | timestamptz | set on logout / forced sign-out |

Notes:
- The JWT itself lives client-side; we only store `jwt_id` server-side so we can revoke. Validation on each request: decode JWT → look up `jwt_id` → ensure `revoked_at IS NULL` and `session_expires_at > now()`.
- Index: `(mobile_number, code_expires_at desc)` for OTP lookup; `(jwt_id)` unique-where-not-null for session validation.

### `staff_users`

Cafe-side admins. Scoped to one org.

| column | type | notes |
|---|---|---|
| id | bigint PK identity | |
| org_id | bigint fk → organisations not null | |
| email | text not null | |
| password_hash | text not null | argon2 |
| name | text | |
| role | text not null | `admin` or `staff` |
| is_active | boolean default true | |

Unique: `(org_id, email)`. Index: `org_id`.

### `tables`

Physical tables in a cafe.

| column | type | notes |
|---|---|---|
| id | bigint PK identity | internal id |
| org_id | bigint fk → organisations not null | |
| code | text not null | short opaque id used in the QR URL (e.g. nanoid). Unique within org. |
| label | text | human label like "T-12" or "Window 3" |
| is_active | boolean default true | |

Unique: `(org_id, code)`. Index: `org_id`.

QR encodes `https://<app>/o/{org_id}/t/{code}`. No `current_order_id` on this table — derive from `orders` when needed.

### `menu`

Items each cafe sells. Category lives here as a plain text field — no separate categories table. Group/order in queries with `category` and `display_order`.

| column | type | notes |
|---|---|---|
| id | bigint PK identity | |
| org_id | bigint fk → organisations not null | |
| name | text not null | |
| description | text | |
| category | text | e.g. `"Beverages"`, `"Starters"`. Free-form per org. Nullable. |
| price | numeric(10,2) not null | |
| image_url | text | |
| display_order | int default 0 | for ordering within a category |
| is_available | boolean default true | toggled by staff without deleting |

Indexes: `(org_id, is_available)`, `(org_id, category)`.

### `orders` — header

One row per checkout / bill.

| column | type | notes |
|---|---|---|
| id | bigint PK identity | |
| public_code | text not null unique | short opaque code shown to customer/staff (e.g. `ORD-7K2X`) |
| org_id | bigint fk → organisations not null | |
| table_id | bigint fk → tables not null | |
| customer_id | bigint fk → customers not null | |
| status | text not null default 'pending' | check in (`pending`,`in_progress`,`completed`,`cancelled`) |
| total_amount | numeric(10,2) not null | sum of line items at checkout |
| is_paid | boolean default false | flips when payment recorded |
| placed_at | timestamptz default now() | |
| completed_at | timestamptz | |

Indexes: `(org_id, status, placed_at desc)` for the admin dashboard, `(table_id, status)` for "what's currently on this table", `(customer_id, placed_at desc)` for customer history.

### `order_items` — lines

One row per item in an order. Snapshots name+price so menu edits don't rewrite history.

| column | type | notes |
|---|---|---|
| id | bigint PK identity | |
| order_id | bigint fk → orders not null on delete cascade | |
| org_id | bigint fk → organisations not null | denormalized for tenant queries |
| menu_item_id | bigint fk → menu | nullable so deleted items don't break history |
| item_name | text not null | snapshot |
| unit_price | numeric(10,2) not null | snapshot |
| quantity | int not null check (quantity > 0) | |
| line_total | numeric(10,2) generated always as (unit_price * quantity) stored | |

Index: `order_id`, `(org_id, menu_item_id)`.

### `payments`

Separate so we can support multiple methods and partial payments later. v1 may only insert one row per order.

| column | type | notes |
|---|---|---|
| id | bigint PK identity | |
| order_id | bigint fk → orders not null | |
| org_id | bigint fk → organisations not null | |
| method | text not null | `cash`, `card`, `upi`, `online` |
| amount | numeric(10,2) not null | |
| txn_ref | text | gateway/upi ref, nullable for cash |
| paid_at | timestamptz default now() | |

Index: `order_id`.

### Notes / decisions baked in

- **No `current_order_id` on `tables`** — derived via `orders WHERE table_id = ? AND status NOT IN ('completed','cancelled')`. Avoids circular FK.
- **Snapshots on `order_items`** (`item_name`, `unit_price`) — deliberate denormalization so menu edits/deletes don't corrupt past bills.
- **`org_id` denormalized everywhere** — small storage cost for big query/security wins.
- **`public_code` on orders** — never expose raw integer ids in URLs or to customers.
- **Status as text + CHECK** — simpler to evolve than a Postgres enum.
- **OTP and session merged into `auth_sessions`** — one row tracks the full login lifecycle. JWT-based: only `jwt_id` (jti) stored for revocation, JWT lives on the client.
- **Flat `menu` table** with `category` as a text column. Sufficient for v1; can normalize later if cafes need rich category metadata.

---

## API Design

All paths prefixed with `/api/v1`. JSON in/out. Auth via `Authorization: Bearer <jwt>` header.

Two distinct JWT audiences: **customer** (mobile+OTP) and **staff** (email+password). Middleware enforces audience per route group.

### Customer APIs

#### Public (no auth)

| # | Method | Path | Purpose |
|---|---|---|---|
| 1 | GET | `/public/context?org_id={id}&table_code={code}` | Validate org+table from QR URL. Returns org name + table label. 404 if either invalid/inactive. |
| 2 | POST | `/auth/otp/request` | Body: `{mobile_number}`. Creates `auth_sessions` row, hashes & "sends" OTP (v1: log to console / mock SMS). Rate-limited per mobile. |
| 3 | POST | `/auth/otp/verify` | Body: `{mobile_number, code}`. On success: upserts `customers`, fills `verified_at` + `jwt_id` + `session_expires_at`, returns `{jwt, customer}`. On failure: increments `attempts`, 401. |

#### Authenticated (customer JWT)

| # | Method | Path | Purpose |
|---|---|---|---|
| 4 | GET | `/me` | Returns current customer (`id`, `mobile_number`, `name`). |
| 5 | POST | `/auth/logout` | Sets `revoked_at` on the customer's current `auth_sessions` row. |
| 6 | GET | `/orgs/{org_id}/menu` | Lists `menu` rows for the org where `is_available=true`. Grouped/ordered by `category` then `display_order`. |
| 7 | POST | `/orders` | Body: `{org_id, table_id, items: [{menu_item_id, quantity}]}`. Server re-fetches prices from `menu` (never trusts client), snapshots `item_name`+`unit_price` into `order_items`, computes `total_amount`, returns the created order with `public_code`. |
| 8 | GET | `/orders/{public_code}` | Single order detail (header + line items + status). Authorized only if `customer_id` matches caller. Drives the "your order status" screen via polling. |

### Admin (Staff) APIs

All under `/admin`. All require staff JWT. Tenant scoping is implicit: the JWT carries `org_id` and every query is filtered by it server-side. Staff cannot specify `org_id` in URLs.

#### Auth

| # | Method | Path | Purpose |
|---|---|---|---|
| A1 | POST | `/admin/auth/login` | Body: `{email, password}`. Verifies against `staff_users.password_hash` (argon2). Returns staff JWT containing `staff_id`, `org_id`, `role`. |
| A2 | POST | `/admin/auth/logout` | Revoke current staff token (or client-side discard if staff JWTs are stateless in v1). |
| A3 | GET | `/admin/me` | Current staff user: `id`, `org_id`, `email`, `name`, `role`. |

#### Tables

| # | Method | Path | Purpose |
|---|---|---|---|
| A4 | GET | `/admin/tables` | List all tables for the staff's org. |
| A5 | POST | `/admin/tables` | Body: `{label}`. Auto-generates `code` (nanoid). Returns row + the QR URL the cafe should print. |
| A6 | PATCH | `/admin/tables/{id}` | Body: `{label?, is_active?}`. |
| A7 | DELETE | `/admin/tables/{id}` | Soft delete (sets `is_active=false`). |

#### Menu

| # | Method | Path | Purpose |
|---|---|---|---|
| A8 | GET | `/admin/menu` | List all menu rows for the org (including unavailable). |
| A9 | POST | `/admin/menu` | Body: `{name, description?, category?, price, image_url?, display_order?}`. |
| A10 | PATCH | `/admin/menu/{id}` | Body: any of the above plus `is_available`. |
| A11 | DELETE | `/admin/menu/{id}` | Soft delete (sets `is_available=false`). Hard delete only if no `order_items` reference it. |

#### Orders (the core admin screen)

| # | Method | Path | Purpose |
|---|---|---|---|
| A12 | GET | `/admin/orders?status=&table_id=&from=&to=&limit=&offset=` | List orders for the org. Default sort: `placed_at desc`. Default filter: status in (`pending`,`in_progress`). The dashboard polls this endpoint. |
| A13 | GET | `/admin/orders/{public_code}` | Order detail with line items, customer mobile, table label, payments. |
| A14 | PATCH | `/admin/orders/{public_code}/status` | Body: `{status: "in_progress" \| "completed" \| "cancelled"}`. Validates state transitions: `pending → in_progress → completed`; `pending`/`in_progress → cancelled`. 409 on illegal transition. Sets `completed_at` when status becomes `completed`. |
| A15 | POST | `/admin/orders/{public_code}/payment` | Body: `{method, amount, txn_ref?}`. Inserts a `payments` row, flips `orders.is_paid=true` once `sum(payments.amount) >= orders.total_amount`. |

#### Org settings

| # | Method | Path | Purpose |
|---|---|---|---|
| A16 | GET | `/admin/org` | Returns the staff's own org (name, address, contacts). |
| A17 | PATCH | `/admin/org` | Body: `{name?, address?, contact_phone?, contact_email?}`. |

### Out of v1 scope

- Customer-side: `PATCH /me`, order history, active-order convenience endpoint, customer-initiated cancel.
- Admin-side: staff user CRUD, platform-level org provisioning, role-based permission split.
- Orgs and the first staff user are seeded via SQL/CLI.

---

## Conventions (apply to both surfaces)

- **Auth middleware**: decode JWT → look up `auth_sessions` by `jwt_id` → check `revoked_at IS NULL` and `session_expires_at > now()` → load `customer_id` (or `staff_id` + `org_id`) into request context.
- **Tenant guard**: customer routes that take `org_id` validate the org exists and is active. Admin routes never accept `org_id` from the client — it's read from the JWT.
- **Price integrity**: client never sends prices on `POST /orders`; server reads `menu.price`.
- **Status values**: `pending | in_progress | completed | cancelled` (matches the CHECK on `orders.status`).
- **Errors**: `{error: {code, message}}` shape. HTTP status conveys class — 400 validation, 401 auth, 403 tenant/role, 404 missing, 409 state conflict.
- **Realtime**: v1 uses polling on `GET /admin/orders` (e.g. every 5s). Websockets/SSE deferred.

---

## Locked decisions (v2)

- **Three role tiers**: `super_admin` (creates orgs + managers; not org-scoped), `manager` (one per org; runs that cafe), `customer` (global, table-scoped during a session).
- **Phone + OTP login for everyone**, including managers and the super admin. Same `auth_sessions` table; the JWT's `aud_kind` claim distinguishes the role.
- **Table-scoped orders.** One open order per physical table. Multiple customers at one table append to the same order. `order_items.added_by_customer_id` tracks who added what.
- **Append-only while open.** Customers can add new items or place duplicate lines that increase quantity. Removing items / decreasing qty is **not allowed** while the order is open.
- **Status enum**: `queued | cooking | prepared | completed | cancelled`. Manager moves through them. Customer never changes status.
- **Manager can cancel** an order at any non-terminal status.
- **Payments are manual.** Manager ticks a "paid" checkbox at the same time they mark COMPLETED — both transition together. `payments` table stores the row for audit.
- **Menu images** are uploaded by the manager from the dashboard and stored at `backend/uploads/menu/<id>.<ext>`, served at `/uploads/menu/<id>.<ext>`. (Move to Cloudinary later — see Open TODOs at top.)
- **Go DB layer**: `gorm`.
