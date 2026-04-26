# Deploying to Render

Render hosts both the backend (Go web service) and the frontend (Vite static site) and the Postgres DB you already created. The **whole stack is one git push** plus a few env-var clicks. No Dockerfiles, no Kubernetes, no AWS console.

## Prerequisites

- The repository pushed to GitHub (private or public, both work).
- The Render account with the `cafe_db_6a7z` Postgres DB already created (you have this).
- An SMTP provider for real OTP emails. Gmail with an App Password works (we already configured it for local dev). Reuse those creds.

## What gets deployed

The blueprint at [`render.yaml`](../render.yaml) defines two services:

| Service | Type | What it serves |
|---|---|---|
| `cafe-backend` | Go Web Service (free plan) | The API on `https://cafe-backend.onrender.com` (Render gives you a free `*.onrender.com` subdomain) |
| `cafe-frontend` | Static Site (free plan) | The Vite-built SPA on `https://cafe-frontend.onrender.com` |

The DB is referenced via its Internal URL — pasted in as a paste-once env var.

## One-time setup

### 1. Push the code to GitHub

```bash
git init
git add .
git commit -m "initial deploy"
git remote add origin git@github.com:<your-handle>/food-ordering-app.git
git push -u origin main
```

### 2. Create the Render Blueprint

1. Render dashboard → **New +** → **Blueprint**.
2. Connect the GitHub repo. Render reads `render.yaml` and proposes the two services.
3. Click **Apply**.

Render will then **prompt you for every `sync: false` env var**. Fill them in:

#### Backend (cafe-backend) env vars

| Key | Value | Where to find it |
|---|---|---|
| `DATABASE_URL` | the **Internal Database URL** for cafe_db_6a7z | Render → DB page → "Internal Database URL" — *internal* because backend lives in the same Render network as the DB; no SSL overhead |
| `CORS_ORIGINS` | `https://cafe-frontend.onrender.com` | the URL Render assigns to your frontend (you'll see it after the first deploy completes) |
| `JWT_SECRET` | leave as auto-generated | Render fills this with a random 32-char value |
| `SMTP_HOST` | `smtp.gmail.com` | from local `.env` |
| `SMTP_USER` | your Gmail address | from local `.env` |
| `SMTP_PASS` | the 16-char app password | from local `.env` |
| `SMTP_FROM` | `Tealogy Cafe <your-gmail>` | from local `.env` |

> **Important:** `ENV=production` (already set in the blueprint) forces the API to *never* return `dev_otp` even if SMTP fails. So make sure SMTP works before/at first deploy or admins won't be able to log in.

#### Frontend (cafe-frontend) env vars

| Key | Value |
|---|---|
| `VITE_API_URL` | `https://cafe-backend.onrender.com` (no trailing slash) |

The frontend bakes this URL into the JS bundle at build time. **Changing it later requires a manual redeploy.**

### 3. Watch the first build

Render runs:

- Backend: `go build -o app .` → `./cmd/migrate up` (pre-deploy) → `./app`
- Frontend: `npm install && npm run build` → publishes `frontend/dist`

The pre-deploy migrate is idempotent — runs `golang-migrate up`, which sees the DB at version 4 already and exits cleanly with `no change`. On future schema changes (new migration files), it applies them automatically.

### 4. Verify

- Visit `https://cafe-backend.onrender.com/api/v1/health` → `{"ok":true}`.
- Visit `https://cafe-frontend.onrender.com/` → home page.
- Open `https://cafe-frontend.onrender.com/admin/login` → sign in with `devashishs105@gmail.com` (super admin) — OTP arrives in your Gmail inbox.
- Browse a menu URL like `https://cafe-frontend.onrender.com/o/1/t/t_a7Kx9` → log in and place an order.

## Subsequent deploys

Just `git push`. Render auto-deploys on every push to the connected branch. The pre-deploy hook runs `migrate up` so adding a `migrations/0005_*.sql` file is enough — no manual migration step.

## Free-plan caveats

- **Backend spins down after 15 min idle**, cold-starts in ~30s on next request. The first scan after a quiet period is slow. Upgrade to the Starter plan ($7/mo) for always-on.
- **Postgres free tier expires after 90 days.** Move to a paid Postgres plan before that or migrate to Neon/Supabase free tiers (which last forever, just bigger setup work).

## Updating env vars

Render dashboard → service → **Environment** → edit → save → **Manual Deploy**. The frontend specifically needs a redeploy (not just a restart) because Vite bakes env vars into the JS bundle at build time.

## Tightening security after first successful deploy

1. **Lock the DB to internal-only:** Render dashboard → DB → **Access Control** → flip "Allow access from anywhere" off. The Internal URL keeps working from the backend service. External access (your laptop) stops, which means `make migrate-up` from your laptop will fail unless you re-enable allow-from-anywhere temporarily.
2. **Rotate `SMTP_PASS`** if it ever leaks (chat history, screenshots).
3. **Custom domain:** add CNAMEs in Render → service → Custom Domains. Render handles HTTPS / Let's Encrypt automatically.

## Rolling back

Render → service → **Deploys** tab → pick a previous successful deploy → "Rollback to this deploy". Database migrations are not rolled back automatically — if a release added a destructive migration, you'd run `make migrate-down` against the cloud DB manually first.

## Local dev still works

Nothing about adding `render.yaml` changes local development. `make backend` and `make frontend` continue to read `.env` and serve on `localhost:5173` + `:8081`. The blueprint and the dev workflow are completely independent.
