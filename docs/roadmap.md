# Roadmap — How We Build This Without Getting Stuck

## Guiding rules

1. **Vertical slices, not horizontal layers.** Every phase ends with something that works end-to-end (DB → API → UI). No "finish all backend then all frontend." That's how projects die at integration time.
2. **Hard things first.** DB connection, CORS, auth, and tenant scoping are the things that fail. Front-load them.
3. **A phase isn't "done" until the checkpoint passes.** No moving on with a half-working slice.
4. **Defer everything not on the critical path.** Tests, prod deploy, fancy UI, payment gateway — all later.

---

## Phase 0 — Foundations (the boring part that kills 50% of projects)

**Goal:** A repo that runs. Both processes start, talk to each other, and to the DB.

- [ ] Folder layout: `backend/`, `frontend/`, `docs/`, `migrations/`
- [ ] Postgres running locally (docker-compose preferred — one-line start)
- [ ] `backend/`: Go module + Gin + GORM. Connect to Postgres using `DATABASE_URL` from `.env`. Add `GET /health` that returns `{"ok":true}` after pinging the DB.
- [ ] `frontend/`: Vite + React + React Router. One placeholder page that fetches `/api/v1/health` and shows "ok".
- [ ] `migrations/0001_init.sql` containing **all 8 tables** from `docs/design.md`. Pick a migration runner (`golang-migrate` or `goose`) and wire `make migrate-up` / `make migrate-down`.
- [ ] CORS configured on Gin to allow `http://localhost:5173`.
- [ ] `.env.example` with `DATABASE_URL`, `JWT_SECRET`, `OTP_TTL_SECONDS=300`, `JWT_TTL_DAYS=30`.

**Checkpoint:**
- `make migrate-up` creates all tables; `\dt` in psql lists them.
- Backend on `:8080`, frontend on `:5173`. Frontend page shows "backend is ok".
- Run the seed script from `docs/menu-render-schema.md` — confirm 1 org + 2 tables + 4 menu items in DB.

**Why this phase matters:** if CORS, env vars, or migrations are broken, every subsequent phase is sand. Fix them now while there's nothing else to debug.

---

## Phase 1 — Menu render (no auth yet)

**Goal:** Scan a (fake) QR URL and see the menu. Auth is stubbed off entirely.

- [ ] Backend: `GET /api/v1/public/context?org_id=&table_code=` (reads `organisations` + `tables`).
- [ ] Backend: `GET /api/v1/orgs/:org_id/menu` — **temporarily public** for this phase, marked with a `// TODO: move behind auth in Phase 2` comment.
- [ ] Frontend: route `/o/:orgId/t/:tableCode`. On mount, fetch context, render header. Then fetch menu, render grouped by category, sorted by `display_order`.
- [ ] Frontend: simple "Add to cart" UI with quantity controls (state only, no submit yet).

**Checkpoint:** open `http://localhost:5173/o/1/t/t_a7Kx9` (using the seed data) → see "Velvet Frost Cafe – Table 1" + the four menu items in their categories. Tap items, see cart count update.

**Why this slice first:** zero auth, zero state mutation, zero auth-related bugs. Pure read path. Surfaces 80% of the integration issues (URL parsing, CORS, JSON shape, env config) with the smallest possible blast radius.

---

## Phase 2 — Customer auth (mobile + OTP + JWT)

**Goal:** Login gates the menu.

- [ ] Backend: `POST /auth/otp/request` — insert `auth_sessions` row, generate 6-digit code, hash with argon2, **log the plain code to stdout** (no SMS gateway in v1). Rate-limit: max 1 request per mobile per 30s.
- [ ] Backend: `POST /auth/otp/verify` — match hash, increment `attempts` on miss, on success upsert `customers`, generate JWT with `jti`, write `jwt_id` + `session_expires_at` + `verified_at` on the row, return `{jwt, customer}`.
- [ ] Backend: `customerAuth` middleware — decode JWT → look up `auth_sessions` by `jwt_id` → check `revoked_at IS NULL AND session_expires_at > now()` → put `customer_id` in context.
- [ ] Backend: move `/orgs/:org_id/menu` behind `customerAuth`. Add `GET /me`, `POST /auth/logout`.
- [ ] Frontend: login page (mobile input → request OTP → 6-digit input → verify). Store JWT in `localStorage`. Auth-guard the menu route.

**Checkpoint:** scan URL → land on login → enter mobile → grab OTP from backend stdout → enter it → land on menu. Refresh the page — still logged in. Click logout — back to login screen.

**Risk to watch:** double-issuing OTPs while one is still valid (let the new one supersede the old; mark old `code_expires_at = now()` on insert). And: `jwt_id` index must be set right or session lookup will be a sequential scan.

---

## Phase 3 — Place order + customer order status

**Goal:** Customer can order something and watch its status.

- [ ] Backend: `POST /orders`. Inside one DB transaction: validate `org_id` + `table_id` + each `menu_item_id` belong to the org; re-fetch each item's `price` and `name` from `menu` (**never trust client**); insert `orders` (with generated `public_code` like `ORD-` + nanoid 4); insert `order_items` rows with snapshots; commit.
- [ ] Backend: `GET /orders/:public_code` — return order + items, scoped to caller's `customer_id`.
- [ ] Frontend: cart sidebar with "Place Order" button. On submit, POST and navigate to `/order/:publicCode`.
- [ ] Frontend: order status page polls `GET /orders/:public_code` every 5s. Shows badge per status (Pending / In Progress / Completed).

**Checkpoint:** place a real order from the customer UI → see it in DB with correct `total_amount` and snapshot prices → status page shows "Pending".

---

## Phase 4 — Admin login + order dashboard (the other half of the loop)

**Goal:** Cafe staff can see and progress orders.

- [ ] Seed one `staff_users` row via SQL (argon2-hash a known password offline; provide a one-shot `make seed-staff` that takes args).
- [ ] Backend: `POST /admin/auth/login` (email + password → staff JWT containing `staff_id`, `org_id`, `role`).
- [ ] Backend: `staffAuth` middleware (analogous to customer middleware but populates `org_id` + `staff_id`).
- [ ] Backend: `GET /admin/orders` (filters: status, table_id; default status filter `pending,in_progress`); `GET /admin/orders/:public_code`; `PATCH /admin/orders/:public_code/status` with state-transition validation (returns 409 on illegal moves; sets `completed_at` on completion).
- [ ] Frontend: `/admin/login` and `/admin` routes. Dashboard polls `GET /admin/orders` every 5s and **groups by `table_id`** (since multiple concurrent orders per table is allowed). Each card has Pending → In Progress → Completed buttons.

**Checkpoint:** new customer order appears on the admin dashboard within 5s. Click "In Progress" → status updates → customer status page reflects it on its next poll. Click "Completed" → `completed_at` populated.

**Risk to watch:** state-transition logic must reject things like `completed → pending` with 409, not silently allow it.

---

## Phase 5 — Manual payments + admin CRUD

**Goal:** Admins can record payments and manage their own menu/tables.

- [ ] `POST /admin/orders/:code/payment` (insert `payments`, recompute `is_paid`).
- [ ] Tables CRUD: `GET/POST/PATCH/DELETE /admin/tables[/:id]`. POST returns the QR URL; print/download flow can be a simple "Show QR" modal using a client-side QR library.
- [ ] Menu CRUD: `GET/POST/PATCH/DELETE /admin/menu[/:id]`.
- [ ] Org settings: `GET/PATCH /admin/org`.

**Checkpoint:** admin creates a new menu item → it appears in the customer menu fetch. Admin marks an order paid → customer status page shows "Paid".

---

## Phase 6 — Tighten the screws

(Do these only after Phases 0–5 are working. Don't get pulled into them earlier.)

- Input validation library wiring (e.g. `go-playground/validator`).
- Structured logging with request IDs.
- OTP rate-limit per IP (not just per mobile).
- Tenant-leak audit: write a checklist test — "as admin of org A, can I read anything from org B?" for every admin endpoint.
- Loading/error states across the frontend.
- Mobile-friendly CSS pass (the customer flow is on phones).

---

## Where to start *right now*

**Phase 0, in this order:**

1. Create folder structure + docker-compose for Postgres.
2. `migrations/0001_init.sql` matching `docs/design.md`. Run it. Verify with `\dt`.
3. Run the seed from `docs/menu-render-schema.md`.
4. Backend health endpoint hitting the DB.
5. Frontend page hitting the health endpoint.

That's it. Don't start Phase 1 until both processes start cleanly and the health check round-trips.

---

## Open question still pending (doesn't block Phase 0–3, blocks Phase 4)

**Bill grouping on a table:** "concurrent orders per table" is locked, but you haven't confirmed whether each customer pays their own bill (table-grouped *display*, per-customer *billing*) or all orders on a table merge into one shared bill (group billing). Per-customer is the simpler default — schema already supports it. Group billing would need a new `table_sessions` concept and `orders.session_id` linking. **Default I'll proceed with: per-customer bills, table-grouped admin display.** Push back before Phase 4 if you want group billing instead.
