package models

import "time"

// User represents a platform user.
type User struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	PasswordHash  string     `json:"-"` // never expose in JSON
	Name          string     `json:"name"`
	AvatarURL     string     `json:"avatar_url"`
	Status        string     `json:"status"`
	TOTPSecretEnc string     `json:"-"`
	TOTPEnabled   bool       `json:"totp_enabled"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
