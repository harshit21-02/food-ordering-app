package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/auth"
	"github.com/harshit/food-ordering-app/internal/mailer"
	"github.com/harshit/food-ordering-app/internal/models"
)

type AuthHandler struct {
	DB            *gorm.DB
	JWTConfig     auth.JWTConfig
	OTPTTLSeconds int
	Mailer        *mailer.Mailer
	IsProduction  bool
}

func NewAuthHandler(db *gorm.DB, jwtCfg auth.JWTConfig, otpTTLSeconds int, m *mailer.Mailer, isProd bool) *AuthHandler {
	return &AuthHandler{DB: db, JWTConfig: jwtCfg, OTPTTLSeconds: otpTTLSeconds, Mailer: m, IsProduction: isProd}
}

// Conservative email pattern: anything resembling foo@bar.tld. Server side we
// only need a sanity check; the OTP loop is the real validator.
var emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)

// Optional mobile in E.164 form: leading +, 8–15 digits.
var mobileRe = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

// --- POST /auth/otp/request ---

type otpRequestBody struct {
	Email        string  `json:"email"         binding:"required"`
	MobileNumber *string `json:"mobile_number"`
}

func (h *AuthHandler) RequestOTP(c *gin.Context) {
	var body otpRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if !emailRe.MatchString(body.Email) {
		RespondError(c, http.StatusBadRequest, "invalid_email", "email must look like name@domain.tld")
		return
	}
	if body.MobileNumber != nil && *body.MobileNumber != "" && !mobileRe.MatchString(*body.MobileNumber) {
		RespondError(c, http.StatusBadRequest, "invalid_mobile", "mobile_number must be in E.164 form (e.g. +919876543210)")
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
	session := models.AuthSession{
		Email:         &emailRef,
		MobileNumber:  body.MobileNumber,
		CodeHash:      hash,
		CodeExpiresAt: time.Now().Add(time.Duration(h.OTPTTLSeconds) * time.Second),
	}
	if err := h.DB.Create(&session).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Try real email. If it fails or SMTP isn't configured, log + return dev_otp.
	mailSent := false
	if h.Mailer != nil && h.Mailer.Enabled() {
		subject := "Your Tealogy Cafe OTP"
		bodyText := fmt.Sprintf(
			"Your one-time code is %s.\n\nIt expires in %d seconds.\nIf you didn't request this, you can ignore the email.\n",
			code, h.OTPTTLSeconds,
		)
		if err := h.Mailer.Send(body.Email, subject, bodyText); err != nil {
			log.Printf("[OTP] email send failed (will return dev_otp): %v", err)
		} else {
			mailSent = true
		}
	}
	log.Printf("[OTP] email=%s code=%s (request_id=%d, mail_sent=%v)",
		body.Email, code, session.ID, mailSent)

	resp := gin.H{
		"request_id": session.ID,
		"expires_in": h.OTPTTLSeconds,
	}
	// dev_otp is ONLY ever returned in non-production. In production we'd
	// rather error obviously than silently leak codes when SMTP is broken.
	if !mailSent && !h.IsProduction {
		resp["dev_otp"] = code
	}
	c.JSON(http.StatusOK, resp)
}

// --- POST /auth/otp/verify ---

type otpVerifyBody struct {
	Email string `json:"email" binding:"required"`
	Code  string `json:"code"  binding:"required"`
}

type customerView struct {
	ID           int64   `json:"id"`
	Email        *string `json:"email,omitempty"`
	MobileNumber *string `json:"mobile_number,omitempty"`
	Name         *string `json:"name,omitempty"`
}

func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var body otpVerifyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if !emailRe.MatchString(body.Email) {
		RespondError(c, http.StatusBadRequest, "invalid_email", "email must look like name@domain.tld")
		return
	}

	// Pick the most recent un-verified, un-expired OTP for this email.
	var session models.AuthSession
	err := h.DB.
		Where("email = ? AND verified_at IS NULL AND code_expires_at > ?", body.Email, time.Now()).
		Order("code_expires_at DESC").
		First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusUnauthorized, "no_active_otp", "no active OTP for this email — request a new one")
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

	// Upsert the customer by email. If the request also captured a mobile and
	// the existing row doesn't have one, save it for future reference.
	var customer models.Customer
	emailRef := body.Email
	if err := h.DB.Where("email = ?", body.Email).
		Attrs(models.Customer{Email: &emailRef}).
		FirstOrCreate(&customer).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	// Persist the mobile from the OTP request if we don't have one yet.
	if customer.MobileNumber == nil && session.MobileNumber != nil && *session.MobileNumber != "" {
		if err := h.DB.Model(&customer).UpdateColumn("mobile_number", session.MobileNumber).Error; err != nil {
			log.Printf("[OTP] could not save mobile_number for customer %d: %v", customer.ID, err)
		} else {
			customer.MobileNumber = session.MobileNumber
		}
	}

	token, jti, exp, err := auth.Sign(h.JWTConfig, auth.AudCustomer, customer.ID, 0, "")
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "jwt_sign_failed", err.Error())
		return
	}
	now := time.Now()
	if err := h.DB.Model(&session).Updates(map[string]any{
		"customer_id":        customer.ID,
		"verified_at":        &now,
		"jwt_id":             &jti,
		"session_expires_at": &exp,
	}).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jwt": token,
		"customer": customerView{
			ID:           customer.ID,
			Email:        customer.Email,
			MobileNumber: customer.MobileNumber,
			Name:         customer.Name,
		},
	})
}

// --- GET /me ---

func (h *AuthHandler) Me(c *gin.Context) {
	customerID := c.GetInt64("customer_id")
	var c2 models.Customer
	if err := h.DB.First(&c2, customerID).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, customerView{
		ID:           c2.ID,
		Email:        c2.Email,
		MobileNumber: c2.MobileNumber,
		Name:         c2.Name,
	})
}

// --- POST /auth/logout ---

func (h *AuthHandler) Logout(c *gin.Context) {
	customerID := c.GetInt64("customer_id")
	now := time.Now()
	if err := h.DB.Model(&models.AuthSession{}).
		Where("customer_id = ? AND revoked_at IS NULL", customerID).
		UpdateColumn("revoked_at", &now).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
