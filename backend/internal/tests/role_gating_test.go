package tests

import (
	"net/http"
	"testing"
)

// Staff role JWT must be rejected by every admin-only endpoint with 403.
func TestRoleGating_StaffRejectedFromAdminMutations(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, staffEmail)

	// We assert each endpoint returns 403 admin_only. For body shape we only
	// need the request to *reach* the gate, so a placeholder body is fine.
	cases := []struct {
		method, path string
		body         any
	}{
		{"POST",   "/api/v1/admin/menu",     map[string]any{"name": "x", "price": 1}},
		{"PATCH",  "/api/v1/admin/menu/1",   map[string]any{"name": "x"}},
		{"POST",   "/api/v1/admin/tables",   map[string]any{"label": "x"}},
		{"PATCH",  "/api/v1/admin/tables/1", map[string]any{"label": "x"}},
		{"POST",   "/api/v1/admin/staff",    map[string]any{"email": "x@y.com", "name": "x", "role": "staff"}},
		{"PATCH",  "/api/v1/admin/staff/2",  map[string]any{"name": "x"}},
	}
	for _, c := range cases {
		w := do(t, c.method, c.path, c.body, jwt)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s — expected 403, got %d (body=%s)", c.method, c.path, w.Code, w.Body.String())
			continue
		}
		if errorCode(t, w) != "admin_only" {
			t.Errorf("%s %s — expected admin_only, got %s", c.method, c.path, errorCode(t, w))
		}
	}
}

// Staff and manager are rejected from /super/* with 403 super_admin_only.
func TestRoleGating_NonSuperRejectedFromSuper(t *testing.T) {
	resetDB(t)
	roles := map[string]string{
		"staff":   staffEmail,
		"manager": managerEmail,
	}
	for label, email := range roles {
		jwt := adminLogin(t, email)
		w := do(t, "GET", "/api/v1/super/orgs", nil, jwt)
		if w.Code != http.StatusForbidden {
			t.Errorf("[%s] expected 403 on /super/orgs, got %d", label, w.Code)
		}
		if errorCode(t, w) != "super_admin_only" {
			t.Errorf("[%s] expected super_admin_only, got %s", label, errorCode(t, w))
		}
	}
}

// Wrong audience (customer JWT) cannot use admin endpoints.
func TestRoleGating_CustomerJWTCannotHitAdmin(t *testing.T) {
	resetDB(t)
	cjwt := customerLogin(t, "alice@example.com")
	w := do(t, "GET", "/api/v1/admin/orders/active", nil, cjwt)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for customer JWT on admin route, got %d", w.Code)
	}
	if errorCode(t, w) != "wrong_audience" {
		t.Fatalf("want wrong_audience, got %s", errorCode(t, w))
	}
}

// Admin/staff JWT cannot reach customer-only endpoints either.
func TestRoleGating_AdminJWTCannotHitCustomer(t *testing.T) {
	resetDB(t)
	ajwt := adminLogin(t, managerEmail)
	w := do(t, "GET", "/api/v1/me", nil, ajwt)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for admin JWT on customer /me, got %d", w.Code)
	}
}

// Health endpoint is open and returns ok.
func TestHealth_Open(t *testing.T) {
	resetDB(t)
	w := do(t, "GET", "/api/v1/health", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
