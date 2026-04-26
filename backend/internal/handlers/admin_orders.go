package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/models"
)

type AdminOrderHandler struct {
	DB *gorm.DB
}

func NewAdminOrderHandler(db *gorm.DB) *AdminOrderHandler {
	return &AdminOrderHandler{DB: db}
}

// adminOrderView is what the dashboard renders per order.
type adminOrderView struct {
	models.Order
	TableLabel    *string             `json:"table_label,omitempty"`
	TableCode     string              `json:"table_code"`
	CustomerEmail *string             `json:"customer_email,omitempty"`
	Items         []models.OrderItem  `json:"items"`
}

func (h *AdminOrderHandler) buildOne(tx *gorm.DB, order models.Order) (adminOrderView, error) {
	view := adminOrderView{Order: order}

	var t models.Table
	if err := tx.First(&t, order.TableID).Error; err == nil {
		view.TableLabel = t.Label
		view.TableCode = t.Code
	}

	if order.CustomerID != nil {
		var cust models.Customer
		if err := tx.First(&cust, *order.CustomerID).Error; err == nil {
			view.CustomerEmail = cust.Email
		}
	}

	if err := tx.Where("order_id = ?", order.ID).Order("created_at, id").Find(&view.Items).Error; err != nil {
		return view, err
	}
	return view, nil
}

func (h *AdminOrderHandler) buildMany(tx *gorm.DB, orders []models.Order) ([]adminOrderView, error) {
	out := make([]adminOrderView, 0, len(orders))
	for _, o := range orders {
		v, err := h.buildOne(tx, o)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// ----- GET /admin/orders/active ---------------------------------------------
//
// All non-final orders for the staff's org, sorted by placed_at ASC (oldest
// first) so the kitchen sees the next-up at the top. Front-end splits them
// into the prep queue (queued+cooking) and the ready queue (prepared).

func (h *AdminOrderHandler) ListActive(c *gin.Context) {
	orgID := c.GetInt64("org_id")

	var orders []models.Order
	if err := h.DB.
		Where("org_id = ? AND status IN ?", orgID,
			[]string{models.OrderStatusQueued, models.OrderStatusCooking, models.OrderStatusPrepared}).
		Order("placed_at ASC").
		Find(&orders).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	views, err := h.buildMany(h.DB, orders)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"orders": views})
}

// ----- GET /admin/orders/history?limit=10&offset=N --------------------------

func (h *AdminOrderHandler) ListHistory(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 1 || limit > 50 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	h.DB.Model(&models.Order{}).
		Where("org_id = ? AND status IN ?", orgID,
			[]string{models.OrderStatusCompleted, models.OrderStatusCancelled}).
		Count(&total)

	var orders []models.Order
	if err := h.DB.
		Where("org_id = ? AND status IN ?", orgID,
			[]string{models.OrderStatusCompleted, models.OrderStatusCancelled}).
		Order("COALESCE(completed_at, updated_at) DESC").
		Limit(limit).Offset(offset).
		Find(&orders).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	views, err := h.buildMany(h.DB, orders)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"orders": views,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// ----- PATCH /admin/orders/:public_code/status -------------------------------
// body: {status: "cooking" | "prepared" | "cancelled"}
//
// Allowed transitions:
//   queued    → cooking, cancelled
//   cooking   → prepared, cancelled
//   prepared  → cancelled (use POST .../complete to mark completed+paid)
//   completed → ✗
//   cancelled → ✗

type statusBody struct {
	Status string `json:"status" binding:"required"`
}

var allowedNext = map[string]map[string]bool{
	models.OrderStatusQueued: {
		models.OrderStatusCooking:   true,
		models.OrderStatusCancelled: true,
	},
	models.OrderStatusCooking: {
		models.OrderStatusPrepared:  true,
		models.OrderStatusCancelled: true,
	},
	models.OrderStatusPrepared: {
		models.OrderStatusCancelled: true,
		// completion goes through /complete, not /status
	},
}

func (h *AdminOrderHandler) UpdateStatus(c *gin.Context) {
	code := c.Param("public_code")
	orgID := c.GetInt64("org_id")

	var body statusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	var order models.Order
	if err := h.DB.Where("public_code = ? AND org_id = ?", code, orgID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "order_not_found", "order not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	if !allowedNext[order.Status][body.Status] {
		RespondError(c, http.StatusConflict, "illegal_transition",
			"cannot move from "+order.Status+" to "+body.Status)
		return
	}

	updates := map[string]any{"status": body.Status}
	if body.Status == models.OrderStatusCancelled {
		// keep completed_at NULL; we do not set it for cancellations.
	}
	if err := h.DB.Model(&order).Updates(updates).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	order.Status = body.Status

	view, err := h.buildOne(h.DB, order)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, view)
}

// ----- POST /admin/orders/:public_code/complete -----------------------------
// body: {method: "cash"|"card"|"upi", amount: number, txn_ref?: string}
//
// Atomic: insert payments row + set status=completed + is_paid=true + completed_at=now.
// Order must currently be in 'prepared' status.

type completeBody struct {
	Method string  `json:"method"  binding:"required"`
	Amount float64 `json:"amount"  binding:"required"`
	TxnRef *string `json:"txn_ref"`
}

func (h *AdminOrderHandler) Complete(c *gin.Context) {
	code := c.Param("public_code")
	orgID := c.GetInt64("org_id")

	var body completeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	switch body.Method {
	case "cash", "card", "upi", "online":
	default:
		RespondError(c, http.StatusBadRequest, "invalid_method", "method must be cash | card | upi | online")
		return
	}

	var view adminOrderView
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Where("public_code = ? AND org_id = ?", code, orgID).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apiErr(http.StatusNotFound, "order_not_found", "order not found")
			}
			return err
		}
		if order.Status != models.OrderStatusPrepared {
			return apiErr(http.StatusConflict, "not_prepared",
				"only orders in 'prepared' status can be completed (current: "+order.Status+")")
		}

		// Insert payment row.
		if err := tx.Exec(`
			INSERT INTO payments (order_id, org_id, method, amount, txn_ref, paid_at)
			VALUES (?, ?, ?, ?, ?, NOW())`,
			order.ID, order.OrgID, body.Method, body.Amount, body.TxnRef,
		).Error; err != nil {
			return err
		}

		now := time.Now()
		if err := tx.Model(&order).Updates(map[string]any{
			"status":       models.OrderStatusCompleted,
			"is_paid":      true,
			"completed_at": &now,
		}).Error; err != nil {
			return err
		}
		order.Status = models.OrderStatusCompleted
		order.IsPaid = true
		order.CompletedAt = &now

		v, err := h.buildOne(tx, order)
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

// ----- GET /admin/orders/:public_code ---------------------------------------

func (h *AdminOrderHandler) GetByCode(c *gin.Context) {
	code := c.Param("public_code")
	orgID := c.GetInt64("org_id")

	var order models.Order
	if err := h.DB.Where("public_code = ? AND org_id = ?", code, orgID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "order_not_found", "order not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	view, err := h.buildOne(h.DB, order)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, view)
}
