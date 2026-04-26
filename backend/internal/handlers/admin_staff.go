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

// AdminStaffHandler manages staff_users within a single org.
// All endpoints expect the caller to be admin/manager (RequireAdmin middleware).
// super_admin rows are hidden from these listings — they're managed elsewhere.
type AdminStaffHandler struct {
	DB *gorm.DB
}

func NewAdminStaffHandler(db *gorm.DB) *AdminStaffHandler {
	return &AdminStaffHandler{DB: db}
}

type staffOut struct {
	ID           int64   `json:"id"`
	OrgID        *int64  `json:"org_id,omitempty"`
	Email        *string `json:"email,omitempty"`
	MobileNumber *string `json:"mobile_number,omitempty"`
	Name         *string `json:"name,omitempty"`
	Role         string  `json:"role"`
	IsActive     bool    `json:"is_active"`
	CreatedAt    string  `json:"created_at"`
}

func toOut(s models.StaffUser) staffOut {
	return staffOut{
		ID: s.ID, OrgID: s.OrgID, Email: s.Email, MobileNumber: s.MobileNumber,
		Name: s.Name, Role: s.Role, IsActive: s.IsActive,
		CreatedAt: s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// GET /admin/staff
// Lists active + inactive manager/staff rows for the caller's org.
// (super_admin rows are excluded — they aren't org-scoped anyway.)
func (h *AdminStaffHandler) List(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	var rows []models.StaffUser
	err := h.DB.
		Where("org_id = ? AND role IN ?", orgID, []string{models.RoleManager, models.RoleStaff}).
		Order("is_active DESC, created_at ASC").
		Find(&rows).Error
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	out := make([]staffOut, 0, len(rows))
	for _, s := range rows {
		out = append(out, toOut(s))
	}
	c.JSON(http.StatusOK, gin.H{"staff": out})
}

type createStaffBody struct {
	Email        string  `json:"email" binding:"required"`
	Name         string  `json:"name"  binding:"required"`
	Role         string  `json:"role"  binding:"required"` // manager | staff
	MobileNumber *string `json:"mobile_number"`
}

// POST /admin/staff (admin-only)
func (h *AdminStaffHandler) Create(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	var body createStaffBody
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
	if body.MobileNumber != nil && *body.MobileNumber != "" && !mobileRe.MatchString(*body.MobileNumber) {
		RespondError(c, http.StatusBadRequest, "invalid_mobile", "mobile_number must be in E.164 form")
		return
	}

	row := models.StaffUser{
		OrgID:        &orgID,
		Email:        &body.Email,
		Name:         &body.Name,
		Role:         body.Role,
		MobileNumber: body.MobileNumber,
		IsActive:     true,
	}
	if err := h.DB.Create(&row).Error; err != nil {
		// Unique violation on (org_id, email)
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			RespondError(c, http.StatusConflict, "email_taken", "a staff member with this email already exists in this org")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, toOut(row))
}

type updateStaffBody struct {
	Name         *string `json:"name"`
	Role         *string `json:"role"`
	IsActive     *bool   `json:"is_active"`
	MobileNumber *string `json:"mobile_number"`
}

// PATCH /admin/staff/:id (admin-only)
// Allows editing name, role (manager/staff only), is_active, mobile.
// Cannot demote/promote to super_admin and cannot edit a super_admin row.
func (h *AdminStaffHandler) Update(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	callerID := c.GetInt64("staff_id")

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
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
	if row.OrgID == nil || *row.OrgID != orgID {
		RespondError(c, http.StatusForbidden, "wrong_org", "this staff member belongs to a different org")
		return
	}
	if row.Role == models.RoleSuperAdmin {
		RespondError(c, http.StatusForbidden, "cannot_edit_super_admin", "super_admin rows cannot be edited from here")
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
		// Don't let an admin lock themselves out of their own row.
		if !*body.IsActive && row.ID == callerID {
			RespondError(c, http.StatusBadRequest, "cannot_disable_self",
				"you can't disable your own account here — ask another admin")
			return
		}
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
	// Re-read for fresh values.
	h.DB.First(&row, id)
	c.JSON(http.StatusOK, toOut(row))
}
