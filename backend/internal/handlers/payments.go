package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/models"
)

const gstRate = 0.05

type PaymentHandler struct {
	DB          *gorm.DB
	CFClientID  string
	CFSecretKey string
	CFBaseURL   string
}

func NewPaymentHandler(db *gorm.DB, clientID, secretKey, baseURL string) *PaymentHandler {
	return &PaymentHandler{DB: db, CFClientID: clientID, CFSecretKey: secretKey, CFBaseURL: baseURL}
}

// POST /orders/:public_code/pay
// Creates a Cashfree payment session for the given order.
func (h *PaymentHandler) CreateSession(c *gin.Context) {
	if h.CFClientID == "" {
		RespondError(c, http.StatusServiceUnavailable, "cf_disabled", "payment gateway not configured")
		return
	}

	code := c.Param("public_code")
	customerID := c.GetInt64("customer_id")

	var body struct {
		ReturnURL string `json:"return_url"`
	}
	c.ShouldBindJSON(&body)

	var order models.Order
	if err := h.DB.Where("public_code = ?", code).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "order_not_found", "order not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	if order.CustomerID == nil || *order.CustomerID != customerID {
		RespondError(c, http.StatusForbidden, "not_your_order", "not your order")
		return
	}

	if order.IsPaid {
		RespondError(c, http.StatusBadRequest, "already_paid", "order is already paid")
		return
	}

	var customer models.Customer
	if err := h.DB.First(&customer, customerID).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	phone := "9999999999"
	if customer.MobileNumber != nil && *customer.MobileNumber != "" {
		phone = *customer.MobileNumber
	}
	email := ""
	if customer.Email != nil {
		email = *customer.Email
	}

	amount := math.Round(order.TotalAmount*(1+gstRate)*100) / 100

	sessionID, err := h.createCFOrder(order.PublicCode, amount, customerID, phone, email, body.ReturnURL)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "cf_error", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"payment_session_id": sessionID})
}

// POST /payments/cashfree-webhook (public — no auth)
func (h *PaymentHandler) Webhook(c *gin.Context) {
	timestamp := c.GetHeader("x-webhook-timestamp")
	sig := c.GetHeader("x-webhook-signature")

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	// Verify HMAC-SHA256 signature.
	mac := hmac.New(sha256.New, []byte(h.CFSecretKey))
	mac.Write([]byte(timestamp + string(body)))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		c.Status(http.StatusUnauthorized)
		return
	}

	var payload struct {
		Data struct {
			Order struct {
				OrderID string `json:"order_id"`
			} `json:"order"`
			Payment struct {
				PaymentStatus string `json:"payment_status"`
			} `json:"payment"`
		} `json:"data"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	if payload.Data.Payment.PaymentStatus == "SUCCESS" {
		h.DB.Model(&models.Order{}).
			Where("public_code = ?", payload.Data.Order.OrderID).
			UpdateColumn("is_paid", true)
	}

	c.Status(http.StatusOK)
}

type cfOrderResp struct {
	PaymentSessionID string `json:"payment_session_id"`
}

func (h *PaymentHandler) createCFOrder(orderID string, amount float64, customerID int64, phone, email, returnURL string) (string, error) {
	payload := map[string]interface{}{
		"order_id":       orderID,
		"order_amount":   amount,
		"order_currency": "INR",
		"customer_details": map[string]string{
			"customer_id":    fmt.Sprintf("cust_%d", customerID),
			"customer_phone": phone,
			"customer_email": email,
		},
		"order_meta": map[string]string{
			"return_url": returnURL,
		},
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, h.CFBaseURL+"/orders", bytes.NewBuffer(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-client-id", h.CFClientID)
	req.Header.Set("x-client-secret", h.CFSecretKey)
	req.Header.Set("x-api-version", "2023-08-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cashfree %d: %s", resp.StatusCode, string(respBody))
	}

	var result cfOrderResp
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	return result.PaymentSessionID, nil
}
