package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/auth"
	"github.com/harshit/food-ordering-app/internal/mailer"
	"github.com/harshit/food-ordering-app/internal/models"
)

// AdminAuthHandler handles email + OTP login for staff_users (super_admin /
// manager / staff). Same OTP machinery as customer auth, separate routes,
// separate audience claim ("staff").
type AdminAuthHandler struct {
	DB            *gorm.DB
	JWTConfig     auth.JWTConfig
	OTPTTLSeconds int
	Mailer        *mailer.Mailer
	IsProduction  bool
}

func NewAdminAuthHandler(db *gorm.DB, jwtCfg auth.JWTConfig, otpTTL int, m *mailer.Mailer, isProd bool) *AdminAuthHandler {
	return &AdminAuthHandler{DB: db, JWTConfig: jwtCfg, OTPTTLSeconds: otpTTL, Mailer: m, IsProduction: isProd}
}

type adminOtpRequestBody struct {
	Email string `json:"email" binding:"required"`
}

// POST /admin/auth/otp/request — body: {email}
func (h *AdminAuthHandler) RequestOTP(c *gin.Context) {
	var body adminOtpRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if !emailRe.MatchString(body.Email) {
		RespondError(c, http.StatusBadRequest, "invalid_email", "email must look like name@domain.tld")
		return
	}

	// Email must already belong to a registered staff user. Don't auto-create.
	var staff models.StaffUser
	err := h.DB.Where("email = ? AND is_active = TRUE", body.Email).First(&staff).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusUnauthorized, "not_a_staff_account",
				"this email is not registered as a staff or admin account")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	code, err := auth.GenerateOTP()
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "otp_gen_failed", err.Error())
		return
	}
	hash, err := auth.HashOTP(code)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "otp_hash_failed", err.Error())
		return
	}

	emailRef := body.Email
	staffIDRef := staff.ID
	session := models.AuthSession{
		Email:         &emailRef,
		StaffID:       &staffIDRef,
		CodeHash:      hash,
		CodeExpiresAt: time.Now().Add(time.Duration(h.OTPTTLSeconds) * time.Second),
	}
	if err := h.DB.Create(&session).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	mailSent := false
	if h.Mailer != nil && h.Mailer.Enabled() {
		subject := "Your Tealogy Cafe staff login OTP"
		bodyText := fmt.Sprintf(
			"You're signing in to the Tealogy admin dashboard as %s.\n\nYour one-time code is %s.\nIt expires in %d seconds.\n",
			staff.Role, code, h.OTPTTLSeconds,
		)
		if err := h.Mailer.Send(body.Email, subject, bodyText); err != nil {
			log.Printf("[ADMIN-OTP] email send failed (will return dev_otp): %v", err)
		} else {
			mailSent = true
		}
	}
	log.Printf("[ADMIN-OTP] email=%s role=%s code=%s (request_id=%d, mail_sent=%v)",
		body.Email, staff.Role, code, session.ID, mailSent)

	resp := gin.H{
		"request_id": session.ID,
		"expires_in": h.OTPTTLSeconds,
	}
	if !mailSent && !h.IsProduction {
		resp["dev_otp"] = code
	}
	c.JSON(http.StatusOK, resp)
}

type adminOtpVerifyBody struct {
	Email string `json:"email" binding:"required"`
	Code  string `json:"code"  binding:"required"`
}

type staffView struct {
	ID    int64   `json:"id"`
	OrgID *int64  `json:"org_id,omitempty"`
	Email *string `json:"email,omitempty"`
	Name  *string `json:"name,omitempty"`
	Role  string  `json:"role"`
}

// POST /admin/auth/otp/verify — body: {email, code}
func (h *AdminAuthHandler) VerifyOTP(c *gin.Context) {
	var body adminOtpVerifyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	var session models.AuthSession
	err := h.DB.
		Where("email = ? AND staff_id IS NOT NULL AND verified_at IS NULL AND code_expires_at > ?",
			body.Email, time.Now()).
		Order("code_expires_at DESC").
		First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusUnauthorized, "no_active_otp",
				"no active OTP for this email — request a new one")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	if session.Attempts >= 5 {
		RespondError(c, http.StatusTooManyRequests, "too_many_attempts", "too many wrong attempts — request a new OTP")
		return
	}

	if err := auth.VerifyOTP(session.CodeHash, body.Code); err != nil {
		h.DB.Model(&session).UpdateColumn("attempts", session.Attempts+1)
		RespondError(c, http.StatusUnauthorized, "wrong_code", "incorrect OTP")
		return
	}

	// Re-fetch the staff row (might have changed since OTP was requested).
	var staff models.StaffUser
	if err := h.DB.Where("id = ? AND is_active = TRUE", *session.StaffID).First(&staff).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	orgID := int64(0)
	if staff.OrgID != nil {
		orgID = *staff.OrgID
	}
	token, jti, exp, err := auth.Sign(h.JWTConfig, auth.AudStaff, staff.ID, orgID, staff.Role)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "jwt_sign_failed", err.Error())
		return
	}
	now := time.Now()
	if err := h.DB.Model(&session).Updates(map[string]any{
		"verified_at":        &now,
		"jwt_id":             &jti,
		"session_expires_at": &exp,
	}).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jwt": token,
		"staff": staffView{
			ID:    staff.ID,
			OrgID: staff.OrgID,
			Email: staff.Email,
			Name:  staff.Name,
			Role:  staff.Role,
		},
	})
}

// GET /admin/me — current staff user
func (h *AdminAuthHandler) Me(c *gin.Context) {
	staffID := c.GetInt64("staff_id")
	var s models.StaffUser
	if err := h.DB.First(&s, staffID).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, staffView{
		ID:    s.ID,
		OrgID: s.OrgID,
		Email: s.Email,
		Name:  s.Name,
		Role:  s.Role,
	})
}

// POST /admin/auth/logout — revoke staff sessions
func (h *AdminAuthHandler) Logout(c *gin.Context) {
	staffID := c.GetInt64("staff_id")
	now := time.Now()
	if err := h.DB.Model(&models.AuthSession{}).
		Where("staff_id = ? AND revoked_at IS NULL", staffID).
		UpdateColumn("revoked_at", &now).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
