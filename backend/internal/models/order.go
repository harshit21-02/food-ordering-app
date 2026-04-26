package models

import "time"

// Order — table-scoped: one open order per physical table at a time.
// CustomerID is the *initiator* (first person to add an item) and is nullable.
// Individual lines record their own added_by_customer_id.
type Order struct {
	ID           int64      `gorm:"primaryKey" json:"id"`
	PublicCode   string     `gorm:"column:public_code;unique" json:"public_code"`
	OrgID        int64      `gorm:"column:org_id" json:"org_id"`
	TableID      int64      `gorm:"column:table_id" json:"table_id"`
	CustomerID   *int64     `gorm:"column:customer_id" json:"customer_id,omitempty"`
	Status       string     `json:"status"` // queued|cooking|prepared|completed|cancelled
	TotalAmount  float64    `gorm:"column:total_amount;type:numeric(10,2)" json:"total_amount"`
	IsPaid       bool       `gorm:"column:is_paid" json:"is_paid"`
	PlacedAt     time.Time  `gorm:"column:placed_at" json:"placed_at"`
	CompletedAt  *time.Time `gorm:"column:completed_at" json:"completed_at,omitempty"`
	CreatedAt    time.Time  `json:"-"`
	UpdatedAt    time.Time  `json:"-"`
}

// Order status enum (matches the CHECK constraint on the column).
const (
	OrderStatusQueued    = "queued"
	OrderStatusCooking   = "cooking"
	OrderStatusPrepared  = "prepared"
	OrderStatusCompleted = "completed"
	OrderStatusCancelled = "cancelled"
)

// OrderItem — one row per line item. item_name + unit_price are snapshots so
// later menu edits never rewrite history. line_total is a generated column
// in the DB so we mark it select-only for GORM.
type OrderItem struct {
	ID                 int64     `gorm:"primaryKey" json:"id"`
	OrderID            int64     `gorm:"column:order_id" json:"order_id"`
	OrgID              int64     `gorm:"column:org_id" json:"org_id"`
	MenuItemID         *int64    `gorm:"column:menu_item_id" json:"menu_item_id,omitempty"`
	ItemName           string    `gorm:"column:item_name" json:"item_name"`
	UnitPrice          float64   `gorm:"column:unit_price;type:numeric(10,2)" json:"unit_price"`
	Quantity           int       `gorm:"column:quantity" json:"quantity"`
	LineTotal          float64   `gorm:"->;column:line_total;type:numeric(10,2)" json:"line_total"`
	AddedByCustomerID  *int64    `gorm:"column:added_by_customer_id" json:"added_by_customer_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"-"`
}

func (OrderItem) TableName() string { return "order_items" }
