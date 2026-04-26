package tests

import (
	"net/http"
	"testing"
)

func TestSuper_ListOrgs_ReturnsSeededOrg(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, superAdminEmail)

	w := do(t, "GET", "/api/v1/super/orgs", nil, jwt)
	assertStatus(t, w, http.StatusOK)
	type r struct {
		Orgs []struct {
			ID         int64   `json:"id"`
			Name       string  `json:"name"`
			StaffCount int64   `json:"staff_count"`
			TableCount int64   `json:"table_count"`
			MenuCount  int64   `json:"menu_count"`
			OrderCount int64   `json:"order_count"`
			IsActive   bool    `json:"is_active"`
		} `json:"orgs"`
	}
	got := mustJSON[r](t, w)
	if len(got.Orgs) != 1 {
		t.Fatalf("want 1 seeded org, got %d", len(got.Orgs))
	}
	o := got.Orgs[0]
	if o.StaffCount != 2 || o.TableCount != 5 || o.MenuCount != 88 {
		t.Fatalf("bad stats: staff=%d tables=%d menu=%d", o.StaffCount, o.TableCount, o.MenuCount)
	}
}

func TestSuper_CreateOrgWithManager_AtomicAndLoginable(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, superAdminEmail)

	body := map[string]any{
		"org": map[string]any{
			"name":          "Tealogy — Sector 18",
			"address":       "Sec 18, Noida",
			"contact_phone": "+919999000202",
		},
		"manager": map[string]any{
			"email": "sec18@tealogy.in", "name": "Sec18 Mgr",
		},
	}
	w := do(t, "POST", "/api/v1/super/orgs", body, jwt)
	assertStatus(t, w, http.StatusOK)

	type r struct {
		Org struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"org"`
		Manager struct {
			ID    int64  `json:"id"`
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"manager"`
	}
	got := mustJSON[r](t, w)
	if got.Org.ID == 0 || got.Manager.Role != "manager" || got.Manager.Email != "sec18@tealogy.in" {
		t.Fatalf("bad response: %+v", got)
	}

	// New manager can sign in.
	mgrJWT := adminLogin(t, "sec18@tealogy.in")
	w = do(t, "GET", "/api/v1/admin/me", nil, mgrJWT)
	assertStatus(t, w, http.StatusOK)
}

func TestSuper_ListAllStaff_FlatList(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, superAdminEmail)

	w := do(t, "GET", "/api/v1/super/staff", nil, jwt)
	assertStatus(t, w, http.StatusOK)
	type s struct {
		Staff []struct {
			Email   string `json:"email"`
			Role    string `json:"role"`
			OrgName string `json:"org_name"`
		} `json:"staff"`
	}
	got := mustJSON[s](t, w)
	// 2 seeded (manager + staff). super_admin must NOT appear.
	if len(got.Staff) != 2 {
		t.Fatalf("want 2 staff (super_admin hidden), got %d", len(got.Staff))
	}
	for _, x := range got.Staff {
		if x.Role == "super_admin" {
			t.Fatal("super_admin should not appear in /super/staff list")
		}
		if x.OrgName == "" {
			t.Fatalf("expected org_name on each row, got empty for %s", x.Email)
		}
	}
}

func TestSuper_UpdateOrg_TogglesIsActive(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, superAdminEmail)

	w := do(t, "PATCH", "/api/v1/super/orgs/1", map[string]any{"is_active": false}, jwt)
	assertStatus(t, w, http.StatusOK)
	type org struct {
		IsActive bool `json:"is_active"`
	}
	if mustJSON[org](t, w).IsActive {
		t.Fatal("expected is_active=false")
	}
}
