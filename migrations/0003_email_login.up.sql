-- v3: customer login switches from phone+OTP to email+OTP.
--
-- - customers gains `email` (unique sparse) and `mobile_number` becomes optional
--   (we still capture it for the cafe's records but it's no longer the login key).
-- - auth_sessions gains `email`; `mobile_number` becomes optional.
--
-- Staff still use phone+OTP for now (we'll switch them when we build the
-- manager login UI).

BEGIN;

-- ----- customers --------------------------------------------------------------
ALTER TABLE customers ADD COLUMN email TEXT;
ALTER TABLE customers ALTER COLUMN mobile_number DROP NOT NULL;

-- Old mobile uniqueness was full unique. Replace with sparse unique so multiple
-- customer rows can have NULL mobile_number.
ALTER TABLE customers DROP CONSTRAINT IF EXISTS customers_mobile_number_key;
CREATE UNIQUE INDEX uq_customers_mobile  ON customers (mobile_number) WHERE mobile_number IS NOT NULL;
CREATE UNIQUE INDEX uq_customers_email   ON customers (email)         WHERE email         IS NOT NULL;

-- ----- auth_sessions ----------------------------------------------------------
ALTER TABLE auth_sessions ADD COLUMN email TEXT;
ALTER TABLE auth_sessions ALTER COLUMN mobile_number DROP NOT NULL;

CREATE INDEX idx_auth_sessions_email ON auth_sessions (email, code_expires_at DESC);

COMMIT;
