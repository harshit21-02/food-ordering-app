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

// SuperOrgsHandler — platform-level org and manager management.
// Every method here assumes the caller is a super_admin (RequireSuperAdmin).
type SuperOrgsHandler struct {
	DB *gorm.DB
}

func NewSuperOrgsHandler(db *gorm.DB) *SuperOrgsHandler {
	return &SuperOrgsHandler{DB: db}
}

// orgWithStats — list shape: org row + counts.
type orgWithStats struct {
	models.Organisation
	StaffCount int64 `json:"staff_count"`
	TableCount int64 `json:"table_count"`
	MenuCount  int64 `json:"menu_count"`
	OrderCount int64 `json:"order_count"`
}

// GET /super/orgs
func (h *SuperOrgsHandler) ListOrgs(c *gin.Context) {
	var orgs []models.Organisation
	if err := h.DB.Order("id").Find(&orgs).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	out := make([]orgWithStats, 0, len(orgs))
	for _, o := range orgs {
		ws := orgWithStats{Organisation: o}
		h.DB.Model(&models.StaffUser{}).
			Where("org_id = ? AND role IN ?", o.ID, []string{models.RoleManager, models.RoleStaff}).
			Count(&ws.StaffCount)
		h.DB.Model(&models.Table{}).Where("org_id = ?", o.ID).Count(&ws.TableCount)
		h.DB.Model(&models.MenuItem{}).Where("org_id = ?", o.ID).Count(&ws.MenuCount)
		h.DB.Model(&models.Order{}).Where("org_id = ?", o.ID).Count(&ws.OrderCount)
		out = append(out, ws)
	}
	c.JSON(http.StatusOK, gin.H{"orgs": out})
}

// POST /super/orgs — create org + first manager in one transaction.
type createOrgBody struct {
	Org struct {
		Name         string  `json:"name"          binding:"required"`
		Address      *string `json:"address"`
		ContactPhone *string `json:"contact_phone"`
		ContactEmail *string `json:"contact_email"`
	} `json:"org" binding:"required"`
	Manager struct {
		Email        string  `json:"email" binding:"required"`
		Name         string  `json:"name"  binding:"required"`
		MobileNumber *string `json:"mobile_number"`
	} `json:"manager" binding:"required"`
}

type createOrgResp struct {
	Org     models.Organisation `json:"org"`
	Manager models.StaffUser    `json:"manager"`
}

func (h *SuperOrgsHandler) CreateOrgWithManager(c *gin.Context) {
	var body createOrgBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	body.Manager.Email = strings.TrimSpace(strings.ToLower(body.Manager.Email))
	if !emailRe.MatchString(body.Manager.Email) {
		RespondError(c, http.StatusBadRequest, "invalid_email", "manager.email must look like name@domain.tld")
		return
	}
	if body.Manager.MobileNumber != nil && *body.Manager.MobileNumber != "" && !mobileRe.MatchString(*body.Manager.MobileNumber) {
		RespondError(c, http.StatusBadRequest, "invalid_mobile", "manager.mobile_number must be in E.164 form")
		return
	}

	var resp createOrgResp
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		org := models.Organisation{
			Name:         strings.TrimSpace(body.Org.Name),
			Address:      trimPtr(body.Org.Address),
			ContactPhone: trimPtr(body.Org.ContactPhone),
			ContactEmail: trimPtr(body.Org.ContactEmail),
			IsActive:     true,
		}
		if err := tx.Create(&org).Error; err != nil {
			return err
		}
		manager := models.StaffUser{
			OrgID:        &org.ID,
			Email:        &body.Manager.Email,
			Name:         &body.Manager.Name,
			MobileNumber: body.Manager.MobileNumber,
			Role:         models.RoleManager,
			IsActive:     true,
		}
		if err := tx.Create(&manager).Error; err != nil {
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
				return apiErr(http.StatusConflict, "manager_email_taken",
					"a staff member with this email already exists in this org")
			}
			return err
		}
		resp = createOrgResp{Org: org, Manager: manager}
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
	c.JSON(http.StatusOK, resp)
}

// PATCH /super/orgs/:id — update name/address/contacts/is_active.
type updateOrgBody struct {
	Name         *string `json:"name"`
	Address      *string `json:"address"`
	ContactPhone *string `json:"contact_phone"`
	ContactEmail *string `json:"contact_email"`
	IsActive     *bool   `json:"is_active"`
}

func (h *SuperOrgsHandler) UpdateOrg(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_id", "id must be a number")
		return
	}
	var body updateOrgBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	var org models.Organisation
	if err := h.DB.First(&org, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "not_found", "org not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	updates := map[string]any{}
	if body.Name != nil {
		updates["name"] = strings.TrimSpace(*body.Name)
	}
	if body.Address != nil {
		updates["address"] = trimPtr(body.Address)
	}
	if body.ContactPhone != nil {
		updates["contact_phone"] = trimPtr(body.ContactPhone)
	}
	if body.ContactEmail != nil {
		updates["contact_email"] = trimPtr(body.ContactEmail)
	}
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, org)
		return
	}
	if err := h.DB.Model(&org).Updates(updates).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	h.DB.First(&org, id)
	c.JSON(http.StatusOK, org)
}
