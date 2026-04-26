package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

// GenerateOTP returns a 6-digit numeric OTP as a string ("000000".."999999").
func GenerateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// HashOTP bcrypts the plain OTP. Cost 10 is fine for 6-digit codes.
func HashOTP(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyOTP returns nil if the plain OTP matches the stored hash.
func VerifyOTP(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
