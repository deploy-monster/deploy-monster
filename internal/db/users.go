package db

import (
	"context"
	"database/sql"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CreateUser inserts a new user.
func (s *SQLiteDB) CreateUser(ctx context.Context, u *core.User) error {
	if u.ID == "" {
		u.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO users (id, email, password_hash, name, avatar_url, status)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			u.ID, u.Email, u.PasswordHash, u.Name, u.AvatarURL, u.Status,
		)
		return err
	})
}

// GetUser retrieves a user by ID.
func (s *SQLiteDB) GetUser(ctx context.Context, id string) (*core.User, error) {
	u := &core.User{}
	err := s.QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, avatar_url, status, totp_enabled, last_login_at, created_at, updated_at
		 FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Status,
		&u.TOTPEnabled, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return u, err
}

// GetUserByEmail retrieves a user by email address.
func (s *SQLiteDB) GetUserByEmail(ctx context.Context, email string) (*core.User, error) {
	u := &core.User{}
	err := s.QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, avatar_url, status, totp_enabled, last_login_at, created_at, updated_at
		 FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Status,
		&u.TOTPEnabled, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return u, err
}

// UpdateUser updates a user's mutable fields.
func (s *SQLiteDB) UpdateUser(ctx context.Context, u *core.User) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE users SET email=?, name=?, avatar_url=?, status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			u.Email, u.Name, u.AvatarURL, u.Status, u.ID,
		)
		return err
	})
}

// UpdatePassword updates a user's password hash.
func (s *SQLiteDB) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE users SET password_hash=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			passwordHash, userID,
		)
		return err
	})
}

// UpdateLastLogin updates the user's last login timestamp.
func (s *SQLiteDB) UpdateLastLogin(ctx context.Context, userID string) error {
	_, err := s.ExecContext(ctx,
		`UPDATE users SET last_login_at=CURRENT_TIMESTAMP WHERE id=?`, userID,
	)
	return err
}

// CountUsers returns the total number of users.
func (s *SQLiteDB) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}
