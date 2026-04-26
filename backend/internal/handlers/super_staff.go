package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/models"
)

// SuperStaffHandler — list and edit staff/manager rows across all orgs.
// Caller must be super_admin.
type SuperStaffHandler struct {
	DB *gorm.DB
}

func NewSuperStaffHandler(db *gorm.DB) *SuperStaffHandler {
	return &SuperStaffHandler{DB: db}
}

type allStaffOut struct {
	staffOut
	OrgName *string `json:"org_name,omitempty"`
}

// GET /super/staff?org_id=N (optional)
// List staff and managers across orgs (excluding other super_admins).
// Each row carries the org name for easy display.
func (h *SuperStaffHandler) ListAll(c *gin.Context) {
	q := h.DB.
		Where("role IN ?", []string{models.RoleManager, models.RoleStaff}).
		Order("org_id, is_active DESC, created_at ASC")

	if v := c.Query("org_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			q = q.Where("org_id = ?", id)
		}
	}

	var rows []models.StaffUser
	if err := q.Find(&rows).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Bulk load org names so we don't N+1.
	orgIDs := map[int64]struct{}{}
	for _, s := range rows {
		if s.OrgID != nil {
			orgIDs[*s.OrgID] = struct{}{}
		}
	}
	ids := make([]int64, 0, len(orgIDs))
	for id := range orgIDs {
		ids = append(ids, id)
	}
	var orgs []models.Organisation
	if len(ids) > 0 {
		h.DB.Where("id IN ?", ids).Find(&orgs)
	}
	orgName := map[int64]string{}
	for _, o := range orgs {
		orgName[o.ID] = o.Name
	}

	out := make([]allStaffOut, 0, len(rows))
	for _, s := range rows {
		row := allStaffOut{staffOut: toOut(s)}
		if s.OrgID != nil {
			n := orgName[*s.OrgID]
			row.OrgName = &n
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"staff": out})
}

// POST /super/staff — create a manager or staff for ANY org.
type superCreateStaffBody struct {
	OrgID        int64   `json:"org_id" binding:"required"`
	Email        string  `json:"email"  binding:"required"`
	Name         string  `json:"name"   binding:"required"`
	Role         string  `json:"role"   binding:"required"` // manager | staff
	MobileNumber *string `json:"mobile_number"`
}

func (h *SuperStaffHandler) Create(c *gin.Context) {
	var body superCreateStaffBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if !emailRe.MatchString(body.Email) {
		RespondError(c, http.StatusBadRequest, "invalid_email", "email must look like name@domain.tld")
		return
	}
	if body.Role != models.RoleManager && body.Role != models.RoleStaff {
		RespondError(c, http.StatusBadRequest, "invalid_role", "role must be 'manager' or 'staff'")
		return
	}

	// Verify the org exists.
	var org models.Organisation
	if err := h.DB.First(&org, body.OrgID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "org_not_found", "org_id not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	row := models.StaffUser{
		OrgID:        &body.OrgID,
		Email:        &body.Email,
		Name:         &body.Name,
		Role:         body.Role,
		MobileNumber: body.MobileNumber,
		IsActive:     true,
	}
	if err := h.DB.Create(&row).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			RespondError(c, http.StatusConflict, "email_taken", "a staff member with this email already exists in this org")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, toOut(row))
}

// PATCH /super/staff/:id — edit any staff/manager row across orgs.
// Same body shape as the org-scoped admin endpoint, but no org check.
func (h *SuperStaffHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_id", "id must be a number")
		return
	}
	var body updateStaffBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	var row models.StaffUser
	if err := h.DB.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "not_found", "staff member not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if row.Role == models.RoleSuperAdmin {
		RespondError(c, http.StatusForbidden, "cannot_edit_super_admin", "super_admin rows are managed elsewhere")
		return
	}
	updates := map[string]any{}
	if body.Name != nil {
		updates["name"] = body.Name
	}
	if body.Role != nil {
		if *body.Role != models.RoleManager && *body.Role != models.RoleStaff {
			RespondError(c, http.StatusBadRequest, "invalid_role", "role must be 'manager' or 'staff'")
			return
		}
		updates["role"] = *body.Role
	}
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}
	if body.MobileNumber != nil {
		if *body.MobileNumber != "" && !mobileRe.MatchString(*body.MobileNumber) {
			RespondError(c, http.StatusBadRequest, "invalid_mobile", "mobile_number must be in E.164 form")
			return
		}
		updates["mobile_number"] = body.MobileNumber
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, toOut(row))
		return
	}
	if err := h.DB.Model(&row).Updates(updates).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	h.DB.First(&row, id)
	c.JSON(http.StatusOK, toOut(row))
}
