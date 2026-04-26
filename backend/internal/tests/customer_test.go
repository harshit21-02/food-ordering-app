package tests

import (
	"net/http"
	"testing"
)

// ---------- Customer auth (email + OTP) ----------

func TestCustomerOTP_HappyPath(t *testing.T) {
	resetDB(t)

	w := do(t, "POST", "/api/v1/auth/otp/request",
		map[string]any{"email": "alice@example.com"}, "")
	assertStatus(t, w, http.StatusOK)

	type r struct {
		RequestID int64  `json:"request_id"`
		ExpiresIn int    `json:"expires_in"`
		DevOTP    string `json:"dev_otp"`
	}
	resp := mustJSON[r](t, w)
	if resp.DevOTP == "" {
		t.Fatal("expected dev_otp in test mode (mailer disabled)")
	}

	w = do(t, "POST", "/api/v1/auth/otp/verify",
		map[string]any{"email": "alice@example.com", "code": resp.DevOTP}, "")
	assertStatus(t, w, http.StatusOK)

	type v struct {
		JWT      string `json:"jwt"`
		Customer struct {
			ID    int64  `json:"id"`
			Email string `json:"email"`
		} `json:"customer"`
	}
	got := mustJSON[v](t, w)
	if got.JWT == "" || got.Customer.ID == 0 || got.Customer.Email != "alice@example.com" {
		t.Fatalf("bad verify response: %+v", got)
	}
}

func TestCustomerOTP_InvalidEmailRejected(t *testing.T) {
	resetDB(t)
	w := do(t, "POST", "/api/v1/auth/otp/request",
		map[string]any{"email": "not-an-email"}, "")
	assertStatus(t, w, http.StatusBadRequest)
	if errorCode(t, w) != "invalid_email" {
		t.Fatalf("want invalid_email, got %s", errorCode(t, w))
	}
}

func TestCustomerOTP_WrongCodeRejected(t *testing.T) {
	resetDB(t)
	w := do(t, "POST", "/api/v1/auth/otp/request",
		map[string]any{"email": "bob@example.com"}, "")
	assertStatus(t, w, http.StatusOK)

	w = do(t, "POST", "/api/v1/auth/otp/verify",
		map[string]any{"email": "bob@example.com", "code": "000000"}, "")
	assertStatus(t, w, http.StatusUnauthorized)
	if errorCode(t, w) != "wrong_code" {
		t.Fatalf("want wrong_code, got %s", errorCode(t, w))
	}
}

func TestCustomer_Me_RequiresAuth(t *testing.T) {
	resetDB(t)
	w := do(t, "GET", "/api/v1/me", nil, "")
	assertStatus(t, w, http.StatusUnauthorized)
}

// ---------- Customer reads: context + menu ----------

func TestPublicContext_HappyPath(t *testing.T) {
	resetDB(t)
	w := do(t, "GET", "/api/v1/public/context?org_id=1&table_code=t_a7Kx9", nil, "")
	assertStatus(t, w, http.StatusOK)

	type r struct {
		Org   struct{ ID int64 } `json:"org"`
		Table struct {
			ID    int64
			Code  string
			Label *string
		} `json:"table"`
	}
	got := mustJSON[r](t, w)
	if got.Org.ID != 1 || got.Table.ID != 1 || got.Table.Code != "t_a7Kx9" {
		t.Fatalf("bad context: %+v", got)
	}
}

func TestPublicContext_BadTableIs404(t *testing.T) {
	resetDB(t)
	w := do(t, "GET", "/api/v1/public/context?org_id=1&table_code=nope", nil, "")
	assertStatus(t, w, http.StatusNotFound)
	if errorCode(t, w) != "table_not_found" {
		t.Fatalf("want table_not_found, got %s", errorCode(t, w))
	}
}

func TestMenu_RequiresCustomerAuth(t *testing.T) {
	resetDB(t)
	w := do(t, "GET", "/api/v1/orgs/1/menu", nil, "")
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestMenu_AuthedReturnsItems(t *testing.T) {
	resetDB(t)
	jwt := customerLogin(t, "alice@example.com")
	w := do(t, "GET", "/api/v1/orgs/1/menu", nil, jwt)
	assertStatus(t, w, http.StatusOK)

	type r struct {
		Items []struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Category string `json:"category"`
		} `json:"items"`
	}
	got := mustJSON[r](t, w)
	if len(got.Items) < 80 { // seed has 88
		t.Fatalf("expected ~88 menu items, got %d", len(got.Items))
	}
}

// ---------- Customer orders: place / append / reduce ----------

func TestPlaceOrder_NewOrderCreated(t *testing.T) {
	resetDB(t)
	jwt := customerLogin(t, "alice@example.com")

	body := map[string]any{
		"org_id":   1,
		"table_id": 1,
		"items": []map[string]any{
			{"menu_item_id": 1, "delta": 2}, // Cutting Chai (Paper) ₹20 × 2
			{"menu_item_id": 2, "delta": 1}, // Cutting Chai (Medium Kulhad) ₹39 × 1
		},
	}
	w := do(t, "POST", "/api/v1/orders", body, jwt)
	assertStatus(t, w, http.StatusOK)

	type orderResp struct {
		PublicCode  string  `json:"public_code"`
		Status      string  `json:"status"`
		TotalAmount float64 `json:"total_amount"`
		Items       []struct {
			ItemName  string  `json:"item_name"`
			UnitPrice float64 `json:"unit_price"`
			Quantity  int     `json:"quantity"`
		} `json:"items"`
	}
	got := mustJSON[orderResp](t, w)
	if got.Status != "queued" {
		t.Fatalf("want queued, got %s", got.Status)
	}
	if got.TotalAmount != 79 { // 20*2 + 39*1
		t.Fatalf("want total 79, got %v", got.TotalAmount)
	}
	if len(got.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(got.Items))
	}
}

func TestAppendItems_SecondCustomerAppendsToSameOrder(t *testing.T) {
	resetDB(t)
	jwtAlice := customerLogin(t, "alice@example.com")
	jwtBob := customerLogin(t, "bob@example.com")

	// Alice places.
	w := do(t, "POST", "/api/v1/orders", map[string]any{
		"org_id": 1, "table_id": 1,
		"items": []map[string]any{{"menu_item_id": 14, "delta": 2}}, // Choc Tea M ₹39 × 2 = 78
	}, jwtAlice)
	assertStatus(t, w, http.StatusOK)
	type orderResp struct {
		PublicCode  string  `json:"public_code"`
		TotalAmount float64 `json:"total_amount"`
		Items       []struct {
			AddedBy *int64 `json:"added_by_customer_id"`
			Qty     int    `json:"quantity"`
		} `json:"items"`
	}
	first := mustJSON[orderResp](t, w)

	// Bob appends to the SAME order (table-scoped rule).
	w = do(t, "POST", "/api/v1/orders", map[string]any{
		"org_id": 1, "table_id": 1,
		"items": []map[string]any{{"menu_item_id": 82, "delta": 1}}, // Veg Burger ₹89
	}, jwtBob)
	assertStatus(t, w, http.StatusOK)
	second := mustJSON[orderResp](t, w)

	if first.PublicCode != second.PublicCode {
		t.Fatalf("expected same public_code; got %s then %s", first.PublicCode, second.PublicCode)
	}
	if second.TotalAmount != 167 { // 78 + 89
		t.Fatalf("want total 167, got %v", second.TotalAmount)
	}
	if len(second.Items) != 2 {
		t.Fatalf("want 2 items after append, got %d", len(second.Items))
	}
}

func TestReduceItems_NegativeDeltaShrinksLine(t *testing.T) {
	resetDB(t)
	jwt := customerLogin(t, "alice@example.com")

	// Place 2 chocolate teas
	_ = do(t, "POST", "/api/v1/orders", map[string]any{
		"org_id": 1, "table_id": 1,
		"items": []map[string]any{{"menu_item_id": 14, "delta": 2}},
	}, jwt)

	// Reduce by 1
	w := do(t, "POST", "/api/v1/orders", map[string]any{
		"org_id": 1, "table_id": 1,
		"items": []map[string]any{{"menu_item_id": 14, "delta": -1}},
	}, jwt)
	assertStatus(t, w, http.StatusOK)

	type orderResp struct {
		TotalAmount float64 `json:"total_amount"`
		Items       []struct {
			MenuItemID *int64 `json:"menu_item_id"`
			Quantity   int    `json:"quantity"`
		} `json:"items"`
	}
	got := mustJSON[orderResp](t, w)
	if got.TotalAmount != 39 {
		t.Fatalf("want total 39, got %v", got.TotalAmount)
	}
	// Single line of qty 1 remaining.
	totalQty := 0
	for _, it := range got.Items {
		if it.MenuItemID != nil && *it.MenuItemID == 14 {
			totalQty += it.Quantity
		}
	}
	if totalQty != 1 {
		t.Fatalf("want 1 chocolate tea left, got %d", totalQty)
	}
}

func TestReduceTooMuch_Rejected(t *testing.T) {
	resetDB(t)
	jwt := customerLogin(t, "alice@example.com")

	// Need an existing order so the server can attempt to reduce.
	_ = do(t, "POST", "/api/v1/orders", map[string]any{
		"org_id": 1, "table_id": 1,
		"items": []map[string]any{{"menu_item_id": 14, "delta": 1}},
	}, jwt)

	w := do(t, "POST", "/api/v1/orders", map[string]any{
		"org_id": 1, "table_id": 1,
		"items": []map[string]any{{"menu_item_id": 14, "delta": -99}},
	}, jwt)
	assertStatus(t, w, http.StatusBadRequest)
	if errorCode(t, w) != "reduce_too_much" {
		t.Fatalf("want reduce_too_much, got %s", errorCode(t, w))
	}
}

func TestActiveOrder_NoneReturns204(t *testing.T) {
	resetDB(t)
	jwt := customerLogin(t, "alice@example.com")
	w := do(t, "GET", "/api/v1/orgs/1/tables/t_b2Mn4/active-order", nil, jwt)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204 for empty active-order, got %d", w.Code)
	}
}

func TestActiveOrder_ReturnsOpenOrder(t *testing.T) {
	resetDB(t)
	jwt := customerLogin(t, "alice@example.com")
	_ = do(t, "POST", "/api/v1/orders", map[string]any{
		"org_id": 1, "table_id": 1,
		"items": []map[string]any{{"menu_item_id": 14, "delta": 1}},
	}, jwt)

	w := do(t, "GET", "/api/v1/orgs/1/tables/t_a7Kx9/active-order", nil, jwt)
	assertStatus(t, w, http.StatusOK)
	type r struct {
		PublicCode string  `json:"public_code"`
		Total      float64 `json:"total_amount"`
	}
	got := mustJSON[r](t, w)
	if got.PublicCode == "" || got.Total != 39 {
		t.Fatalf("bad active order: %+v", got)
	}
}

func TestCannotReduceWhenNoOrder(t *testing.T) {
	resetDB(t)
	jwt := customerLogin(t, "alice@example.com")

	// No order exists for table 2 yet — negative delta should be rejected.
	w := do(t, "POST", "/api/v1/orders", map[string]any{
		"org_id": 1, "table_id": 2,
		"items": []map[string]any{{"menu_item_id": 14, "delta": -1}},
	}, jwt)
	assertStatus(t, w, http.StatusBadRequest)
	if errorCode(t, w) != "nothing_to_reduce" {
		t.Fatalf("want nothing_to_reduce, got %s", errorCode(t, w))
	}
}

// Schema-level guarantee: only one open order per table at a time.
func TestUniqueOpenOrderPerTable_ConstraintEnforced(t *testing.T) {
	resetDB(t)
	jwt := customerLogin(t, "alice@example.com")

	// Place an order on table 1 (creates first open order).
	_ = do(t, "POST", "/api/v1/orders", map[string]any{
		"org_id": 1, "table_id": 1,
		"items": []map[string]any{{"menu_item_id": 14, "delta": 1}},
	}, jwt)

	// Manually attempt to insert a second open order via raw SQL — DB partial
	// unique index should block it.
	err := gormDB.Exec(`
		INSERT INTO orders (public_code, org_id, table_id, status, total_amount)
		VALUES ('ORD-DUPL', 1, 1, 'queued', 0)
	`).Error
	if err == nil {
		t.Fatal("expected unique constraint violation, got none")
	}
}
