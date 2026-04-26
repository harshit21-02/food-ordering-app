package tests

import (
	"net/http"
	"strconv"
	"testing"
)

// ---------- Menu CRUD ----------

func TestMenu_AdminListIncludesUnavailable(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, managerEmail)

	w := do(t, "GET", "/api/v1/admin/menu", nil, jwt)
	assertStatus(t, w, http.StatusOK)
	type r struct {
		Items []any `json:"items"`
	}
	if len(mustJSON[r](t, w).Items) != 88 {
		t.Fatalf("expected 88 menu items, got %d", len(mustJSON[r](t, w).Items))
	}
}

func TestMenu_CreateAndUpdate(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, managerEmail)

	// Create
	w := do(t, "POST", "/api/v1/admin/menu", map[string]any{
		"name": "Test Espresso", "category": "Coffee", "price": 80,
		"description": "Test only",
	}, jwt)
	assertStatus(t, w, http.StatusOK)
	type item struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		IsAvailable bool   `json:"is_available"`
	}
	created := mustJSON[item](t, w)
	if created.ID == 0 || !created.IsAvailable {
		t.Fatalf("bad create: %+v", created)
	}

	// Hide it (toggle availability)
	w = do(t, "PATCH", "/api/v1/admin/menu/"+strconv.FormatInt(created.ID, 10),
		map[string]any{"is_available": false}, jwt)
	assertStatus(t, w, http.StatusOK)
	if mustJSON[item](t, w).IsAvailable {
		t.Fatal("expected is_available=false after toggle")
	}

	// Customer-facing menu must NOT include it now.
	custJWT := customerLogin(t, "cust@example.com")
	w = do(t, "GET", "/api/v1/orgs/1/menu", nil, custJWT)
	type cmRes struct {
		Items []struct{ ID int64 } `json:"items"`
	}
	for _, it := range mustJSON[cmRes](t, w).Items {
		if it.ID == created.ID {
			t.Fatal("hidden item should not be visible to customers")
		}
	}
}

func TestMenu_StaffRoleCannotCreate(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, staffEmail)
	w := do(t, "POST", "/api/v1/admin/menu", map[string]any{
		"name": "Sneaky", "price": 1,
	}, jwt)
	assertStatus(t, w, http.StatusForbidden)
	if errorCode(t, w) != "admin_only" {
		t.Fatalf("want admin_only, got %s", errorCode(t, w))
	}
}

// ---------- Tables CRUD ----------

func TestTables_CreateAutogeneratesCode(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, managerEmail)

	w := do(t, "POST", "/api/v1/admin/tables", map[string]any{"label": "Garden 1"}, jwt)
	assertStatus(t, w, http.StatusOK)
	type tbl struct {
		ID    int64  `json:"id"`
		Code  string `json:"code"`
		Label string `json:"label"`
	}
	got := mustJSON[tbl](t, w)
	if !startsWith(got.Code, "t_") || len(got.Code) != 7 {
		t.Fatalf("expected code like t_XXXXX, got %q", got.Code)
	}
	if got.Label != "Garden 1" {
		t.Fatalf("expected label 'Garden 1', got %q", got.Label)
	}
}

func TestTables_DisableHidesFromContext(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, managerEmail)

	// Disable the seeded Table 1 (id=1).
	w := do(t, "PATCH", "/api/v1/admin/tables/1", map[string]any{"is_active": false}, jwt)
	assertStatus(t, w, http.StatusOK)

	// Public context should now 404 for that table.
	w = do(t, "GET", "/api/v1/public/context?org_id=1&table_code=t_a7Kx9", nil, "")
	assertStatus(t, w, http.StatusNotFound)
}

// ---------- Staff CRUD ----------

func TestStaff_AdminCanCreateThenStaffCanLogin(t *testing.T) {
	resetDB(t)
	mgr := adminLogin(t, managerEmail)

	w := do(t, "POST", "/api/v1/admin/staff", map[string]any{
		"email": "barista@tealogy.in", "name": "Barista B", "role": "staff",
	}, mgr)
	assertStatus(t, w, http.StatusOK)

	// Newly-created barista can sign in.
	jwt := adminLogin(t, "barista@tealogy.in")
	w = do(t, "GET", "/api/v1/admin/me", nil, jwt)
	assertStatus(t, w, http.StatusOK)
	type me struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if got := mustJSON[me](t, w); got.Email != "barista@tealogy.in" || got.Role != "staff" {
		t.Fatalf("bad /me: %+v", got)
	}
}

func TestStaff_DuplicateEmailIs409(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, managerEmail)
	body := map[string]any{"email": "dup@tealogy.in", "name": "X", "role": "staff"}

	w := do(t, "POST", "/api/v1/admin/staff", body, jwt)
	assertStatus(t, w, http.StatusOK)

	w = do(t, "POST", "/api/v1/admin/staff", body, jwt)
	assertStatus(t, w, http.StatusConflict)
	if errorCode(t, w) != "email_taken" {
		t.Fatalf("want email_taken, got %s", errorCode(t, w))
	}
}

func TestStaff_CannotDisableSelf(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, managerEmail)

	w := do(t, "PATCH", "/api/v1/admin/staff/2", // staff id 2 = manager
		map[string]any{"is_active": false}, jwt)
	assertStatus(t, w, http.StatusBadRequest)
	if errorCode(t, w) != "cannot_disable_self" {
		t.Fatalf("want cannot_disable_self, got %s", errorCode(t, w))
	}
}

func TestStaff_StaffRoleCannotCreateOthers(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, staffEmail)
	w := do(t, "POST", "/api/v1/admin/staff", map[string]any{
		"email": "x@tealogy.in", "name": "X", "role": "staff",
	}, jwt)
	assertStatus(t, w, http.StatusForbidden)
}

// ---------- helper ----------

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
