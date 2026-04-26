package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/models"
)

type PublicHandler struct {
	DB *gorm.DB
}

func NewPublicHandler(db *gorm.DB) *PublicHandler {
	return &PublicHandler{DB: db}
}

// ContextResponse is what /public/context returns on the wire.
type ContextResponse struct {
	Org   contextOrg   `json:"org"`
	Table contextTable `json:"table"`
}

type contextOrg struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type contextTable struct {
	ID    int64   `json:"id"`
	Code  string  `json:"code"`
	Label *string `json:"label,omitempty"`
}

// GetContext: GET /public/context?org_id=&table_code=
// Validates the QR-derived (org_id, table_code) pair and returns the cafe + table label.
func (h *PublicHandler) GetContext(c *gin.Context) {
	orgID := c.Query("org_id")
	tableCode := c.Query("table_code")
	if orgID == "" || tableCode == "" {
		RespondError(c, http.StatusBadRequest, "missing_params", "org_id and table_code are required")
		return
	}

	var org models.Organisation
	if err := h.DB.Where("id = ? AND is_active = TRUE", orgID).First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "org_not_found", "organisation not found or inactive")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	var table models.Table
	if err := h.DB.Where("org_id = ? AND code = ? AND is_active = TRUE", org.ID, tableCode).First(&table).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "table_not_found", "table not found or inactive")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	c.JSON(http.StatusOK, ContextResponse{
		Org:   contextOrg{ID: org.ID, Name: org.Name},
		Table: contextTable{ID: table.ID, Code: table.Code, Label: table.Label},
	})
}

type MenuItemResponse struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Description  *string `json:"description,omitempty"`
	Category     *string `json:"category,omitempty"`
	Price        float64 `json:"price"`
	ImageURL     *string `json:"image_url,omitempty"`
	DisplayOrder int     `json:"display_order"`
}

// GetMenu: GET /orgs/:org_id/menu
// Public for Phase 1. Will move behind customer JWT in Phase 2.
func (h *PublicHandler) GetMenu(c *gin.Context) {
	orgID := c.Param("org_id")

	var items []models.MenuItem
	err := h.DB.
		Where("org_id = ? AND is_available = TRUE", orgID).
		Order("category NULLS LAST, display_order, name").
		Find(&items).Error
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	out := make([]MenuItemResponse, 0, len(items))
	for _, m := range items {
		out = append(out, MenuItemResponse{
			ID:           m.ID,
			Name:         m.Name,
			Description:  m.Description,
			Category:     m.Category,
			Price:        m.Price,
			ImageURL:     m.ImageURL,
			DisplayOrder: m.DisplayOrder,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}
