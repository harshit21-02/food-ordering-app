package models

import "time"

// Organisation represents one cafe.
type Organisation struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	Name         string    `json:"name"`
	Address      *string   `json:"address,omitempty"`
	ContactPhone *string   `gorm:"column:contact_phone" json:"contact_phone,omitempty"`
	ContactEmail *string   `gorm:"column:contact_email" json:"contact_email,omitempty"`
	IsActive     bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Table is a physical table in a cafe. The QR code encodes (org_id, code).
type Table struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	OrgID     int64     `gorm:"column:org_id" json:"org_id"`
	Code      string    `json:"code"`
	Label     *string   `json:"label,omitempty"`
	IsActive  bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MenuItem belongs to one cafe.
// Note: float64 is fine for prices in this scale (max ~9999.99).
// If the cafe ever needs strict decimal arithmetic, swap to shopspring/decimal.
type MenuItem struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	OrgID        int64     `gorm:"column:org_id" json:"org_id"`
	Name         string    `json:"name"`
	Description  *string   `json:"description,omitempty"`
	Category     *string   `json:"category,omitempty"`
	Price        float64   `gorm:"type:numeric(10,2)" json:"price"`
	ImageURL     *string   `gorm:"column:image_url" json:"image_url,omitempty"`
	DisplayOrder int       `gorm:"column:display_order" json:"display_order"`
	IsAvailable  bool      `gorm:"column:is_available" json:"is_available"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName overrides GORM's pluralisation: struct Menu* would map to "menus", but our table is "menu".
func (MenuItem) TableName() string { return "menu" }
