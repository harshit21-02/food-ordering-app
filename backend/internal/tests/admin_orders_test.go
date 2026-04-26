package tests

import (
	"net/http"
	"testing"
)

const (
	managerEmail    = "noreply.tealogy@gmail.com"
	staffEmail      = "noreply.tealogy+staff@gmail.com"
	superAdminEmail = "devashishs105@gmail.com"
)

// Place a customer order and return its public_code.
func placeCustomerOrder(t *testing.T, email string, tableID int, items ...map[string]any) string {
	t.Helper()
	jwt := customerLogin(t, email)
	w := do(t, "POST", "/api/v1/orders", map[string]any{
		"org_id": 1, "table_id": tableID, "items": items,
	}, jwt)
	assertStatus(t, w, http.StatusOK)
	type r struct {
		PublicCode string `json:"public_code"`
	}
	return mustJSON[r](t, w).PublicCode
}

func TestAdmin_ListActive_ShowsCustomerOrders(t *testing.T) {
	resetDB(t)
	_ = placeCustomerOrder(t, "alice@example.com", 1,
		map[string]any{"menu_item_id": 14, "delta": 2})

	jwt := adminLogin(t, managerEmail)
	w := do(t, "GET", "/api/v1/admin/orders/active", nil, jwt)
	assertStatus(t, w, http.StatusOK)

	type r struct {
		Orders []struct {
			PublicCode string  `json:"public_code"`
			Status     string  `json:"status"`
			TableLabel string  `json:"table_label"`
			Total      float64 `json:"total_amount"`
		} `json:"orders"`
	}
	got := mustJSON[r](t, w)
	if len(got.Orders) != 1 || got.Orders[0].Status != "queued" || got.Orders[0].Total != 78 {
		t.Fatalf("bad active list: %+v", got)
	}
}

func TestAdmin_StatusTransition_QueuedToCookingToPrepared(t *testing.T) {
	resetDB(t)
	code := placeCustomerOrder(t, "alice@example.com", 1,
		map[string]any{"menu_item_id": 14, "delta": 1})
	jwt := adminLogin(t, managerEmail)

	// queued → cooking
	w := do(t, "PATCH", "/api/v1/admin/orders/"+code+"/status",
		map[string]any{"status": "cooking"}, jwt)
	assertStatus(t, w, http.StatusOK)
	if mustJSON[map[string]any](t, w)["status"] != "cooking" {
		t.Fatal("expected status=cooking")
	}

	// cooking → prepared
	w = do(t, "PATCH", "/api/v1/admin/orders/"+code+"/status",
		map[string]any{"status": "prepared"}, jwt)
	assertStatus(t, w, http.StatusOK)
	if mustJSON[map[string]any](t, w)["status"] != "prepared" {
		t.Fatal("expected status=prepared")
	}
}

func TestAdmin_StatusTransition_IllegalRejected(t *testing.T) {
	resetDB(t)
	code := placeCustomerOrder(t, "alice@example.com", 1,
		map[string]any{"menu_item_id": 14, "delta": 1})
	jwt := adminLogin(t, managerEmail)

	// queued → completed is not allowed via /status (must go through /complete)
	w := do(t, "PATCH", "/api/v1/admin/orders/"+code+"/status",
		map[string]any{"status": "completed"}, jwt)
	assertStatus(t, w, http.StatusConflict)
	if errorCode(t, w) != "illegal_transition" {
		t.Fatalf("want illegal_transition, got %s", errorCode(t, w))
	}
}

func TestAdmin_Complete_RequiresPrepared(t *testing.T) {
	resetDB(t)
	code := placeCustomerOrder(t, "alice@example.com", 1,
		map[string]any{"menu_item_id": 14, "delta": 1})
	jwt := adminLogin(t, managerEmail)

	// Try to complete from 'queued' — should reject with not_prepared.
	w := do(t, "POST", "/api/v1/admin/orders/"+code+"/complete",
		map[string]any{"method": "cash", "amount": 39}, jwt)
	assertStatus(t, w, http.StatusConflict)
	if errorCode(t, w) != "not_prepared" {
		t.Fatalf("want not_prepared, got %s", errorCode(t, w))
	}
}

func TestAdmin_Complete_HappyPath_FlipsPaid(t *testing.T) {
	resetDB(t)
	code := placeCustomerOrder(t, "alice@example.com", 1,
		map[string]any{"menu_item_id": 14, "delta": 1})
	jwt := adminLogin(t, managerEmail)

	_ = do(t, "PATCH", "/api/v1/admin/orders/"+code+"/status",
		map[string]any{"status": "cooking"}, jwt)
	_ = do(t, "PATCH", "/api/v1/admin/orders/"+code+"/status",
		map[string]any{"status": "prepared"}, jwt)

	w := do(t, "POST", "/api/v1/admin/orders/"+code+"/complete",
		map[string]any{"method": "cash", "amount": 39}, jwt)
	assertStatus(t, w, http.StatusOK)
	type r struct {
		Status      string `json:"status"`
		IsPaid      bool   `json:"is_paid"`
		CompletedAt string `json:"completed_at"`
	}
	got := mustJSON[r](t, w)
	if got.Status != "completed" || !got.IsPaid || got.CompletedAt == "" {
		t.Fatalf("bad completion: %+v", got)
	}

	// payments row should exist
	var paymentCount int64
	gormDB.Raw("SELECT COUNT(*) FROM payments WHERE order_id = (SELECT id FROM orders WHERE public_code = ?)", code).
		Scan(&paymentCount)
	if paymentCount != 1 {
		t.Fatalf("expected 1 payment row, got %d", paymentCount)
	}
}

func TestAdmin_Cancel_FromQueued(t *testing.T) {
	resetDB(t)
	code := placeCustomerOrder(t, "alice@example.com", 1,
		map[string]any{"menu_item_id": 14, "delta": 1})
	jwt := adminLogin(t, managerEmail)

	w := do(t, "PATCH", "/api/v1/admin/orders/"+code+"/status",
		map[string]any{"status": "cancelled"}, jwt)
	assertStatus(t, w, http.StatusOK)
	if mustJSON[map[string]any](t, w)["status"] != "cancelled" {
		t.Fatal("expected status=cancelled")
	}

	// Active list should now be empty.
	w = do(t, "GET", "/api/v1/admin/orders/active", nil, jwt)
	assertStatus(t, w, http.StatusOK)
	type lst struct {
		Orders []any `json:"orders"`
	}
	if len(mustJSON[lst](t, w).Orders) != 0 {
		t.Fatal("expected no active orders after cancel")
	}
}

func TestAdmin_History_ListsCompletedAndCancelled(t *testing.T) {
	resetDB(t)
	jwt := adminLogin(t, managerEmail)

	// Place + complete one order.
	code := placeCustomerOrder(t, "alice@example.com", 1,
		map[string]any{"menu_item_id": 14, "delta": 1})
	_ = do(t, "PATCH", "/api/v1/admin/orders/"+code+"/status",
		map[string]any{"status": "cooking"}, jwt)
	_ = do(t, "PATCH", "/api/v1/admin/orders/"+code+"/status",
		map[string]any{"status": "prepared"}, jwt)
	_ = do(t, "POST", "/api/v1/admin/orders/"+code+"/complete",
		map[string]any{"method": "cash", "amount": 39}, jwt)

	// Place + cancel another.
	code2 := placeCustomerOrder(t, "bob@example.com", 2,
		map[string]any{"menu_item_id": 82, "delta": 1})
	_ = do(t, "PATCH", "/api/v1/admin/orders/"+code2+"/status",
		map[string]any{"status": "cancelled"}, jwt)

	w := do(t, "GET", "/api/v1/admin/orders/history?limit=10", nil, jwt)
	assertStatus(t, w, http.StatusOK)
	type r struct {
		Total  int64 `json:"total"`
		Orders []struct {
			Status string `json:"status"`
		} `json:"orders"`
	}
	got := mustJSON[r](t, w)
	if got.Total != 2 || len(got.Orders) != 2 {
		t.Fatalf("expected 2 history orders, got total=%d len=%d", got.Total, len(got.Orders))
	}
}

// Staff role has full order powers — must be able to flip statuses and complete.
func TestAdmin_StaffRole_CanProgressOrders(t *testing.T) {
	resetDB(t)
	code := placeCustomerOrder(t, "alice@example.com", 1,
		map[string]any{"menu_item_id": 14, "delta": 1})
	jwt := adminLogin(t, staffEmail)

	w := do(t, "PATCH", "/api/v1/admin/orders/"+code+"/status",
		map[string]any{"status": "cooking"}, jwt)
	assertStatus(t, w, http.StatusOK)
}
