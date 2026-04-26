package handlers

import (
	"crypto/rand"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/models"
)

type AdminTableHandler struct {
	DB *gorm.DB
}

func NewAdminTableHandler(db *gorm.DB) *AdminTableHandler {
	return &AdminTableHandler{DB: db}
}

// Same alphabet as order public_codes (no 0/O/I/1 to avoid handwritten typos).
const tableCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// newTableCode → "t_" + 5 chars from the alphabet.
func newTableCode() (string, error) {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	out := make([]byte, 0, 7)
	out = append(out, 't', '_')
	for _, c := range b {
		out = append(out, tableCodeAlphabet[int(c)%len(tableCodeAlphabet)])
	}
	return string(out), nil
}

// GET /admin/tables — all tables for the org (active + inactive).
func (h *AdminTableHandler) List(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	var rows []models.Table
	if err := h.DB.
		Where("org_id = ?", orgID).
		Order("is_active DESC, label, code").
		Find(&rows).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"tables": rows})
}

type createTableBody struct {
	Label *string `json:"label"` // optional, e.g. "Table 6"
}

// POST /admin/tables (admin-only). Auto-generates a unique code.
// Retries up to a few times in the unlikely event of a collision.
func (h *AdminTableHandler) Create(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	var body createTableBody
	_ = c.ShouldBindJSON(&body)

	row := models.Table{
		OrgID:    orgID,
		Label:    trimPtr(body.Label),
		IsActive: true,
	}

	for attempt := 0; attempt < 5; attempt++ {
		code, err := newTableCode()
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "rand_error", err.Error())
			return
		}
		row.Code = code
		err = h.DB.Create(&row).Error
		if err == nil {
			c.JSON(http.StatusOK, row)
			return
		}
		// retry on unique violation, fail otherwise
		if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "unique") {
			RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
	}
	RespondError(c, http.StatusInternalServerError, "code_collision", "could not generate a unique code, try again")
}

type updateTableBody struct {
	Label    *string `json:"label"`
	IsActive *bool   `json:"is_active"`
}

// PATCH /admin/tables/:id (admin-only)
func (h *AdminTableHandler) Update(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_id", "id must be a number")
		return
	}
	var body updateTableBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	var row models.Table
	if err := h.DB.Where("id = ? AND org_id = ?", id, orgID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "not_found", "table not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	updates := map[string]any{}
	if body.Label != nil {
		updates["label"] = trimPtr(body.Label)
	}
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, row)
		return
	}
	if err := h.DB.Model(&row).Updates(updates).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	h.DB.First(&row, id)
	c.JSON(http.StatusOK, row)
}
