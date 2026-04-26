// Package tests is the integration test suite. It hits the real Gin router
// and a real Postgres instance (cafedb_test by default).
//
// Run with:  make test
// Override DB:  TEST_DATABASE_URL=postgres://... make test
package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/config"
	dbpkg "github.com/harshit/food-ordering-app/internal/db"
	"github.com/harshit/food-ordering-app/internal/mailer"
	"github.com/harshit/food-ordering-app/internal/server"
)

var (
	router *gin.Engine
	gormDB *gorm.DB
)

const defaultTestURL = "postgres://postgres:postgres@localhost:5432/cafedb_test?sslmode=disable"

// TestMain runs once: connects to the test DB, runs all migrations, builds the
// router. Each test calls resetDB(t) to start from a clean seeded state.
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	log.SetOutput(io.Discard) // silence handler logs in tests

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = defaultTestURL
	}

	// Apply all migrations to the test DB. Drop first so a re-run is idempotent.
	mig, err := migrate.New("file://../../../migrations", dbURL)
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Fatalf("migrate init: %v (is cafedb_test created? `make test-db-create`)", err)
	}
	if err := mig.Drop(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.SetOutput(os.Stderr)
		log.Fatalf("migrate drop: %v", err)
	}
	mig.Close()
	mig2, err := migrate.New("file://../../../migrations", dbURL)
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Fatalf("migrate re-init: %v", err)
	}
	if err := mig2.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.SetOutput(os.Stderr)
		log.Fatalf("migrate up: %v", err)
	}
	mig2.Close()

	// Build router using the same wiring as production.
	gdb, err := dbpkg.Open(dbURL)
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Fatalf("gorm open: %v", err)
	}
	gormDB = gdb

	cfg := &config.Config{
		DatabaseURL:   dbURL,
		BackendAddr:   ":0",
		JWTSecret:     "test-secret-key-please-do-not-use-in-prod",
		OTPTTLSeconds: 300,
		JWTTTLDays:    1,
	}
	mail := mailer.New(mailer.Config{}) // disabled — dev_otp will be in responses
	r, err := server.New(cfg, gdb, mail)
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Fatalf("server build: %v", err)
	}
	router = r

	code := m.Run()
	os.Exit(code)
}

// resetDB wipes every test-mutable table and re-runs the dev seed so each test
// starts from the same clean state.
func resetDB(t *testing.T) {
	t.Helper()
	tables := []string{
		"payments", "order_items", "orders", "auth_sessions",
		"customers", "menu", "tables", "staff_users", "organisations",
	}
	for _, tbl := range tables {
		if err := gormDB.Exec("TRUNCATE TABLE " + tbl + " RESTART IDENTITY CASCADE").Error; err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
	seed, err := os.ReadFile("../../../seeds/dev.sql")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	if err := gormDB.Exec(string(seed)).Error; err != nil {
		t.Fatalf("re-seed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func do(t *testing.T, method, path string, body any, jwt string) *httptest.ResponseRecorder {
	t.Helper()
	var br io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		br = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, br)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func mustJSON[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode JSON (status %d, body=%s): %v", w.Code, w.Body.String(), err)
	}
	return out
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("expected status %d, got %d. body=%s", want, w.Code, w.Body.String())
	}
}

// errorBody peeks at the standard {error: {code, message}} envelope.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var eb errorBody
	if err := json.Unmarshal(w.Body.Bytes(), &eb); err != nil {
		return ""
	}
	return eb.Error.Code
}

// ---------------------------------------------------------------------------
// Login helpers — call the real OTP endpoints and return JWTs
// ---------------------------------------------------------------------------

// customerLogin gets a customer JWT for `email`. Creates the customer on first
// call (the OTP verify upserts).
func customerLogin(t *testing.T, email string) string {
	t.Helper()
	w := do(t, "POST", "/api/v1/auth/otp/request",
		map[string]any{"email": email}, "")
	assertStatus(t, w, http.StatusOK)
	var rsp struct {
		DevOTP string `json:"dev_otp"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &rsp)
	if rsp.DevOTP == "" {
		t.Fatalf("expected dev_otp in response (mailer should be disabled in tests). body=%s", w.Body.String())
	}

	w = do(t, "POST", "/api/v1/auth/otp/verify",
		map[string]any{"email": email, "code": rsp.DevOTP}, "")
	assertStatus(t, w, http.StatusOK)
	var v struct {
		JWT string `json:"jwt"`
	}
	mustDecode(t, w, &v)
	return v.JWT
}

// adminLogin returns a staff JWT for the staff_users row matching `email`.
// The email must already be seeded.
func adminLogin(t *testing.T, email string) string {
	t.Helper()
	w := do(t, "POST", "/api/v1/admin/auth/otp/request",
		map[string]any{"email": email}, "")
	assertStatus(t, w, http.StatusOK)
	var rsp struct {
		DevOTP string `json:"dev_otp"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &rsp)
	if rsp.DevOTP == "" {
		t.Fatalf("expected dev_otp for admin login (body=%s)", w.Body.String())
	}

	w = do(t, "POST", "/api/v1/admin/auth/otp/verify",
		map[string]any{"email": email, "code": rsp.DevOTP}, "")
	assertStatus(t, w, http.StatusOK)
	var v struct {
		JWT string `json:"jwt"`
	}
	mustDecode(t, w, &v)
	return v.JWT
}

func mustDecode(t *testing.T, w *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
		t.Fatalf("decode JSON (status %d, body=%s): %v", w.Code, w.Body.String(), err)
	}
}

// suppress unused-import warnings if the suite is reduced
var (
	_ = fmt.Sprintf
	_ = strings.TrimSpace
	_ = time.Now
)
