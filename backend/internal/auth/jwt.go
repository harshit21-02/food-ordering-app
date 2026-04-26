package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Audience values for the two JWT flavours.
const (
	AudCustomer = "customer"
	AudStaff    = "staff"
)

// Claims is what we encode into every JWT.
type Claims struct {
	jwt.RegisteredClaims
	Aud    string `json:"aud_kind"`         // "customer" or "staff"
	UserID int64  `json:"uid"`              // customer_id or staff_id
	OrgID  int64  `json:"org_id,omitempty"` // staff only
	Role   string `json:"role,omitempty"`   // staff only: manager | staff | super_admin
}

// JWTConfig is the issuer config: key + lifetime.
type JWTConfig struct {
	Secret []byte
	TTL    time.Duration
}

// NewJTI generates a 16-byte hex random id used as the JWT's jti claim
// and stored on auth_sessions.jwt_id for revocation.
func NewJTI() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Sign issues a JWT for the given audience+user. Returns (token, jti, expiresAt).
// `role` is only meaningful for staff JWTs ("manager" / "staff" / "super_admin").
// Customer JWTs should pass an empty role string.
func Sign(cfg JWTConfig, aud string, userID int64, orgID int64, role string) (string, string, time.Time, error) {
	jti, err := NewJTI()
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("jti: %w", err)
	}
	exp := time.Now().Add(cfg.TTL)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Aud:    aud,
		UserID: userID,
		OrgID:  orgID,
		Role:   role,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(cfg.Secret)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return signed, jti, exp, nil
}

// Parse decodes + validates the token and returns its claims.
func Parse(cfg JWTConfig, tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return cfg.Secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
