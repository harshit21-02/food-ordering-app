# Cafe Food Ordering

Multi-tenant cafe ordering — customers scan a QR on their table, log in with email + OTP, place an order, and the cafe admin sees and progresses the order. See [`docs/design.md`](docs/design.md) for the full schema and API design.

## Stack

- **Backend**: Go + Gin + GORM
- **Frontend**: React + Vite + TypeScript
- **Database**: Postgres 16 (local Docker for dev, any cloud Postgres for prod)
- **Email**: SMTP via stdlib `net/smtp` (Gmail / Resend / SendGrid all work)

## Prerequisites

- Go 1.22+
- Node 20+
- Docker (only for local Postgres — skip if you're using a cloud DB)

## First-time setup (local Docker DB)

```bash
# 1. Copy env file and fill in JWT_SECRET (and SMTP_* if you want real emails)
cp .env.example .env

# 2. Start Postgres
make db-up

# 3. Apply migrations and load dev seed
make migrate-up
make seed

# 4. Install backend deps
cd backend && go mod download && cd ..

# 5. Install frontend deps
cd frontend && npm install && cd ..
```

## Running

Two terminals:

```bash
make backend     # serves :8081
make frontend    # serves :5173
```

Open http://localhost:5173 — try the `/o/1/t/t_a7Kx9` link to enter the customer flow as Tealogy Cafe Table 1.

### Seeded users you can sign in as

| Role | Email | Where |
|---|---|---|
| Customer | any email (creates on first OTP) | `/o/1/t/<table_code>` |
| Branch Admin | `noreply.tealogy@gmail.com` | `/admin/login` |
| Branch Staff | `noreply.tealogy+staff@gmail.com` | `/admin/login` |
| Super Admin | `devashishs105@gmail.com` | `/admin/login` |

## Running against a cloud database (AWS RDS, Supabase, Neon, …)

The migrate / seed / backend programs all read `DATABASE_URL` from `.env` (no `psql` or `migrate` CLI needed). To switch to a cloud DB:

1. Edit `.env`:
   ```
   DATABASE_URL=postgres://USER:PASS@HOST:5432/DBNAME?sslmode=require
   ```
   AWS RDS typically requires `sslmode=require`. Supabase / Neon: same. Local Docker uses `sslmode=disable`.
2. Apply schema and seed against the cloud DB:
   ```bash
   make migrate-up    # creates all tables in the cloud DB
   make seed          # inserts org + staff + 88-item Tealogy menu
   ```
3. Start the backend — it'll connect to the cloud DB:
   ```bash
   make backend
   ```

That's it. The local Docker Postgres can stay stopped; nothing else changes.

## Useful commands

```bash
make db-shell        # psql inside local container (skip if cloud)
make migrate-version # show current migration version
make migrate-down    # roll back one migration
make migrate-reset   # drop everything + re-apply (DESTRUCTIVE)
make help            # full list
```

## Tests

The integration suite runs against a separate `cafedb_test` database. It exercises every HTTP route — auth, orders, admin CRUD, role gating, super-admin flows. **42 tests, ~10s wall time.**

```bash
make test-db-create   # one-time, only for local Docker
make test
```

To run against a cloud DB instead (e.g. an isolated staging cluster):

```bash
TEST_DATABASE_URL='postgres://...?sslmode=require' make test
```

⚠️ The test runner **drops and recreates the schema** at start. Never point `TEST_DATABASE_URL` at a database that holds anything you care about.

### What the suite covers

- **Auth**: customer email-OTP request/verify, admin email-OTP for all staff roles, wrong-code rejection, audience checks
- **Customer orders**: place, append (table-scoped, multi-customer), reduce, reduce-too-much, active-order (200/204), unique-open-order-per-table DB constraint
- **Admin orders**: list active, status transitions (queued → cooking → prepared), illegal-transition 409, complete-with-payment (writes payments row + flips `is_paid`), cancel, history (paid + cancelled)
- **Admin menu**: list (incl. unavailable), create + update + hide; hidden items disappear from the customer-facing menu
- **Admin tables**: create with auto-generated code, disable + verify customer context returns 404
- **Admin staff**: create, login as the new staff, duplicate email → 409, can't-disable-self check
- **Super admin**: list orgs with stats, atomic create-org+manager (and the new manager can sign in), list all staff across orgs (super_admin row hidden), toggle org active
- **Role gating**: staff role gets 403 on every admin-only mutation; staff/manager get 403 on `/super/*`; customer JWT rejected on admin routes; admin JWT rejected on customer routes

## Layout

```
backend/
├── cmd/
│   ├── migrate/        Go-native migrate runner (no psql / migrate CLI needed)
│   └── seed/           Go-native seed loader
├── internal/
│   ├── auth/           OTP + JWT
│   ├── config/         .env + env loading
│   ├── db/             GORM connection
│   ├── handlers/       HTTP handlers (customer, admin, super)
│   ├── middleware/     auth, role gates
│   ├── models/         GORM structs
│   ├── server/         router wiring (used by main + tests)
│   └── tests/          integration test suite
└── main.go             builds + runs the server

frontend/
└── src/
    ├── components/     LoginScreen
    ├── contexts/       AuthContext + AdminAuthContext
    ├── lib/api.ts      typed fetch wrapper, customer + admin + super endpoints
    └── pages/          MenuPage (customer) + AdminLogin + AdminDashboard

migrations/             0001_init … 0004_staff_dashboard
seeds/dev.sql           Tealogy org + staff + 88 menu items
docs/design.md          Schema + API reference
```
