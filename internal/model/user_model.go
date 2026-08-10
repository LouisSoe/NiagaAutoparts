package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

type UserRole string

const (
	RoleAdmin    UserRole = "admin"
	RoleStaff    UserRole = "staff"
	RoleManager  UserRole = "manager"
	RoleCustomer UserRole = "customer"
	RoleCashier  UserRole = "cashier"
)

type User struct {
	ID           int64          `db:"id" json:"id"`
	Email        string         `db:"email" json:"email"`
	PasswordHash string         `db:"password_hash" json:"-"`
	Name         string         `db:"name" json:"name"`
	Role         UserRole       `db:"role" json:"role"`
	Phone        sql.NullString `db:"phone" json:"phone"`
	IsActive     bool           `db:"is_active" json:"is_active"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at" json:"updated_at"`
}

func (u User) MarshalJSON() ([]byte, error) {
	type Alias User
	var phone *string
	if u.Phone.Valid {
		phone = &u.Phone.String
	}
	return json.Marshal(&struct {
		Alias
		Phone *string `json:"phone"`
	}{
		Alias: Alias(u),
		Phone: phone,
	})
}

