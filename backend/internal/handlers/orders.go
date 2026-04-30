package handlers

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/models"
)

type OrderHandler struct {
	DB *gorm.DB
}

func NewOrderHandler(db *gorm.DB) *OrderHandler {
	return &OrderHandler{DB: db}
}

// public_code is what we expose to customers/staff (avoid leaking sequential ids).
// 4 chars from a confusable-free alphabet, prefixed "ORD-".
const publicCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newPublicCode() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	out := []byte("ORD-XXXX")
	for i, c := range b {
		out[4+i] = publicCodeAlphabet[int(c)%len(publicCodeAlphabet)]
	}
	return string(out), nil
}

// orderView is the wire shape returned for any order-fetching endpoint.
type orderView struct {
	models.Order
	Items []models.OrderItem `json:"items"`
}

func (h *OrderHandler) buildOrderView(tx *gorm.DB, order models.Order) (orderView, error) {
	var items []models.OrderItem
	if err := tx.Where("order_id = ?", order.ID).Order("created_at, id").Find(&items).Error; err != nil {
		return orderView{}, err
	}
	return orderView{Order: order, Items: items}, nil
}

// ----- GET /orgs/:org_id/tables/:table_code/active-order ---------------------
//
// Returns the currently-open order on this table, with all line items, or
// 204 No Content if there's no open order.

func (h *OrderHandler) GetActiveOrder(c *gin.Context) {
	orgID := c.Param("org_id")
	tableCode := c.Param("table_code")

	var table models.Table
	if err := h.DB.
		Where("org_id = ? AND code = ? AND is_active = TRUE", orgID, tableCode).
		First(&table).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "table_not_found", "table not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	var order models.Order
	err := h.DB.
		Where("table_id = ? AND status NOT IN ? AND is_paid IS NOT TRUE", table.ID, []string{models.OrderStatusCompleted, models.OrderStatusCancelled}).
		First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.Status(http.StatusNoContent)
		return
	}
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	view, err := h.buildOrderView(h.DB, order)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, view)
}

// ----- POST /orders ----------------------------------------------------------
//
// Body: { org_id, table_id, items: [{menu_item_id, delta}, ...] }
//
// `delta` is the *signed* change to apply per menu item:
//   delta > 0 → add `delta` more of that item to the order
//   delta < 0 → reduce the placed quantity by abs(delta), oldest line first
//   delta == 0 → rejected by validation
//
// If a non-final order exists for table_id, the deltas are applied to it.
// Otherwise a new order is created (only positive deltas accepted in that case).
// Server re-fetches each menu item's price from `menu` for additions — never
// trusts the client.
//
// Returns the resulting order view (header + all items).

type reconcileBody struct {
	OrgID   int64               `json:"org_id"   binding:"required"`
	TableID int64               `json:"table_id" binding:"required"`
	Items   []reconcileItemIn   `json:"items"    binding:"required,min=1,dive"`
}

type reconcileItemIn struct {
	MenuItemID int64 `json:"menu_item_id" binding:"required"`
	Delta      int   `json:"delta"        binding:"required"` // non-zero (zero is filtered out by binding)
}

func (h *OrderHandler) PlaceOrAppend(c *gin.Context) {
	var body reconcileBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	customerID := c.GetInt64("customer_id")

	var view orderView
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		// Lock the table row to serialise concurrent requests for this table.
		var table models.Table
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ? AND org_id = ? AND is_active = TRUE", body.TableID, body.OrgID).
			First(&table).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apiErr(http.StatusNotFound, "table_not_found", "table not found in this org")
			}
			return err
		}

		// We only need menu rows for the *positive* deltas (we re-fetch their
		// price). For negative deltas we read price from existing order_items.
		menuByID := map[int64]models.MenuItem{}
		var addIDs []int64
		hasAdds := false
		for _, it := range body.Items {
			if it.Delta > 0 {
				addIDs = append(addIDs, it.MenuItemID)
				hasAdds = true
			}
		}
		if hasAdds {
			var rows []models.MenuItem
			if err := tx.Where("id IN ? AND org_id = ? AND is_available = TRUE", addIDs, body.OrgID).
				Find(&rows).Error; err != nil {
				return err
			}
			seen := map[int64]bool{}
			for _, m := range rows {
				menuByID[m.ID] = m
				seen[m.ID] = true
			}
			for _, id := range addIDs {
				if !seen[id] {
					return apiErr(http.StatusBadRequest, "invalid_items", "one or more menu items are missing or unavailable")
				}
			}
		}

		// Find or create the open order on this table.
		// Exclude paid orders — a paid order is closed from the customer's side.
		var order models.Order
		err := tx.Where("table_id = ? AND status NOT IN ? AND is_paid IS NOT TRUE", table.ID,
			[]string{models.OrderStatusCompleted, models.OrderStatusCancelled}).
			First(&order).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// New order — every delta must be positive (nothing to reduce yet).
			for _, it := range body.Items {
				if it.Delta < 0 {
					return apiErr(http.StatusBadRequest, "nothing_to_reduce", "no existing order on this table to reduce from")
				}
			}
			code, gerr := newPublicCode()
			if gerr != nil {
				return gerr
			}
			order = models.Order{
				PublicCode:  code,
				OrgID:       body.OrgID,
				TableID:     table.ID,
				CustomerID:  &customerID,
				Status:      models.OrderStatusQueued,
				TotalAmount: 0,
				PlacedAt:    time.Now(),
			}
			if err := tx.Create(&order).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		netDeltaTotal := 0.0

		for _, it := range body.Items {
			if it.Delta > 0 {
				// Append a new line for the addition.
				m := menuByID[it.MenuItemID]
				cust := customerID
				row := models.OrderItem{
					OrderID:           order.ID,
					OrgID:             order.OrgID,
					MenuItemID:        &m.ID,
					ItemName:          m.Name,
					UnitPrice:         m.Price,
					Quantity:          it.Delta,
					AddedByCustomerID: &cust,
				}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
				netDeltaTotal += m.Price * float64(it.Delta)
				continue
			}

			// Negative delta: reduce existing lines for this menu_item_id, oldest first.
			toRemove := -it.Delta
			var lines []models.OrderItem
			if err := tx.Where("order_id = ? AND menu_item_id = ?", order.ID, it.MenuItemID).
				Order("created_at, id").Find(&lines).Error; err != nil {
				return err
			}
			placed := 0
			for _, l := range lines {
				placed += l.Quantity
			}
			if toRemove > placed {
				return apiErr(http.StatusBadRequest, "reduce_too_much",
					"requested reduction exceeds the placed quantity for this item")
			}
			for i := range lines {
				if toRemove == 0 {
					break
				}
				l := &lines[i]
				if l.Quantity <= toRemove {
					netDeltaTotal -= l.UnitPrice * float64(l.Quantity)
					toRemove -= l.Quantity
					if err := tx.Delete(l).Error; err != nil {
						return err
					}
				} else {
					netDeltaTotal -= l.UnitPrice * float64(toRemove)
					newQ := l.Quantity - toRemove
					toRemove = 0
					if err := tx.Model(l).UpdateColumn("quantity", newQ).Error; err != nil {
						return err
					}
				}
			}
		}

		newTotal := order.TotalAmount + netDeltaTotal
		if newTotal < 0 {
			newTotal = 0 // defensive; shouldn't happen given validation above
		}
		if err := tx.Model(&order).UpdateColumn("total_amount", newTotal).Error; err != nil {
			return err
		}
		order.TotalAmount = newTotal

		// If after applying deltas the order has zero items, leave it as-is
		// (still queued, total 0). Manager can cancel it from the dashboard.

		v, err := h.buildOrderView(tx, order)
		if err != nil {
			return err
		}
		view = v
		return nil
	})
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) {
			RespondError(c, ae.status, ae.code, ae.msg)
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, view)
}

// ----- GET /orders/:public_code ---------------------------------------------
//
// Returns the order + items if the caller contributed any line to it
// (table-scoped orders → "my order" means "order with at least one of my lines").

func (h *OrderHandler) GetByPublicCode(c *gin.Context) {
	code := c.Param("public_code")
	customerID := c.GetInt64("customer_id")

	var order models.Order
	if err := h.DB.Where("public_code = ?", code).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "order_not_found", "order not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Authorise: caller must have contributed at least one line OR be the order's initiator.
	authorised := false
	if order.CustomerID != nil && *order.CustomerID == customerID {
		authorised = true
	} else {
		var cnt int64
		h.DB.Model(&models.OrderItem{}).
			Where("order_id = ? AND added_by_customer_id = ?", order.ID, customerID).
			Count(&cnt)
		if cnt > 0 {
			authorised = true
		}
	}
	if !authorised {
		RespondError(c, http.StatusForbidden, "not_your_order", "you didn't contribute to this order")
		return
	}

	view, err := h.buildOrderView(h.DB, order)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, view)
}

// ----- POST /orders/:public_code/mark-paid ------------------------------------------
//
// Called by the customer after completing UPI payment. Sets is_paid = true so
// the next PlaceOrAppend on the same table creates a fresh order instead of
// appending to this one.

func (h *OrderHandler) MarkPaid(c *gin.Context) {
	code := c.Param("public_code")
	customerID := c.GetInt64("customer_id")

	var order models.Order
	if err := h.DB.Where("public_code = ?", code).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "order_not_found", "order not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	if order.CustomerID == nil || *order.CustomerID != customerID {
		RespondError(c, http.StatusForbidden, "not_your_order", "you didn't place this order")
		return
	}

	if order.IsPaid {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	if err := h.DB.Model(&order).UpdateColumn("is_paid", true).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// --- internal: a tiny error type so transaction code can return early with status -----

type apiError struct {
	status int
	code   string
	msg    string
}

func (e *apiError) Error() string { return fmt.Sprintf("%s: %s", e.code, e.msg) }

func apiErr(status int, code, msg string) error {
	return &apiError{status: status, code: code, msg: msg}
}
