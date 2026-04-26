package models

import "time"

// Customer is global across orgs. Email is the login key (sparse-unique).
// Mobile number is optional metadata captured at signup.
type Customer struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	Email        *string   `gorm:"column:email" json:"email,omitempty"`
	MobileNumber *string   `gorm:"column:mobile_number" json:"mobile_number,omitempty"`
	Name         *string   `json:"name,omitempty"`
	CreatedAt    time.Time `json:"-"`
	UpdatedAt    time.Time `json:"-"`
}

// AuthSession holds the lifecycle of one OTP attempt -> session.
// On creation it carries only the OTP fields. After successful verify, the same
// row is updated with customer_id, verified_at, jwt_id, session_expires_at.
type AuthSession struct {
	ID                int64      `gorm:"primaryKey"`
	Email             *string    `gorm:"column:email"`
	MobileNumber      *string    `gorm:"column:mobile_number"`
	CustomerID        *int64     `gorm:"column:customer_id"`
	StaffID           *int64     `gorm:"column:staff_id"`
	CodeHash          string     `gorm:"column:code_hash"`
	CodeExpiresAt     time.Time  `gorm:"column:code_expires_at"`
	Attempts          int        `gorm:"column:attempts"`
	VerifiedAt        *time.Time `gorm:"column:verified_at"`
	JwtID             *string    `gorm:"column:jwt_id"`
	SessionExpiresAt  *time.Time `gorm:"column:session_expires_at"`
	RevokedAt         *time.Time `gorm:"column:revoked_at"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (AuthSession) TableName() string { return "auth_sessions" }

// StaffUser is a cafe-side user. Roles:
//   super_admin — global; not scoped to an org; creates new orgs + their first manager.
//   manager     — branch admin; full control of one org's menu, tables, staff, and orders.
//   staff       — order operations only within one org.
type StaffUser struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	OrgID        *int64    `gorm:"column:org_id" json:"org_id,omitempty"`
	Email        *string   `gorm:"column:email" json:"email,omitempty"`
	MobileNumber *string   `gorm:"column:mobile_number" json:"mobile_number,omitempty"`
	PasswordHash *string   `gorm:"column:password_hash" json:"-"`
	Name         *string   `json:"name,omitempty"`
	Role         string    `json:"role"`
	IsActive     bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt    time.Time `json:"-"`
	UpdatedAt    time.Time `json:"-"`
}

func (StaffUser) TableName() string { return "staff_users" }

// Role constants.
const (
	RoleSuperAdmin = "super_admin"
	RoleManager    = "manager"
	RoleStaff      = "staff"
)

// IsAdminTier reports whether the role has full branch privileges
// (manager and super_admin both qualify).
func (s StaffUser) IsAdminTier() bool {
	return s.Role == RoleSuperAdmin || s.Role == RoleManager
}
