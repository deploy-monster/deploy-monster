package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

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
	var totpSecret sql.NullString
	var backupCodes sql.NullString
	err := s.QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, avatar_url, status, totp_enabled, totp_secret_enc, totp_backup_codes_json, last_login_at, created_at, updated_at
		 FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Status,
		&u.TOTPEnabled, &totpSecret, &backupCodes, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.TOTPSecret = totpSecret.String
	u.TOTPBackupCodes = decodeTOTPBackupCodes(backupCodes.String)
	return u, nil
}

// GetUserByEmail retrieves a user by email address.
func (s *SQLiteDB) GetUserByEmail(ctx context.Context, email string) (*core.User, error) {
	u := &core.User{}
	var totpSecret sql.NullString
	var backupCodes sql.NullString
	err := s.QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, avatar_url, status, totp_enabled, totp_secret_enc, totp_backup_codes_json, last_login_at, created_at, updated_at
		 FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Status,
		&u.TOTPEnabled, &totpSecret, &backupCodes, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.TOTPSecret = totpSecret.String
	u.TOTPBackupCodes = decodeTOTPBackupCodes(backupCodes.String)
	return u, nil
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

// UpdateTOTPEnabled enables or disables TOTP for a user.
// When enabling, totpSecretEnc should be the encrypted TOTP secret.
// When disabling, pass empty string for totpSecretEnc.
func (s *SQLiteDB) UpdateTOTPEnabled(ctx context.Context, userID string, enabled bool, totpSecretEnc string) error {
	totpEnabled := 0
	if enabled {
		totpEnabled = 1
	}
	_, err := s.ExecContext(ctx,
		`UPDATE users SET totp_enabled=?, totp_secret_enc=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		totpEnabled, totpSecretEnc, userID,
	)
	return err
}

// UpdateTOTPBackupCodes replaces the user's hashed TOTP backup codes.
func (s *SQLiteDB) UpdateTOTPBackupCodes(ctx context.Context, userID string, hashes []string) error {
	encoded, err := json.Marshal(hashes)
	if err != nil {
		return fmt.Errorf("marshal totp backup codes: %w", err)
	}
	_, err = s.ExecContext(ctx,
		`UPDATE users SET totp_backup_codes_json=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		string(encoded), userID,
	)
	return err
}

func decodeTOTPBackupCodes(encoded string) []string {
	if encoded == "" {
		return nil
	}
	var hashes []string
	if err := json.Unmarshal([]byte(encoded), &hashes); err != nil {
		return nil
	}
	return hashes
}

// GetUsersByIDs retrieves multiple users in a single query.
func (s *SQLiteDB) GetUsersByIDs(ctx context.Context, ids []string) ([]core.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(
		`SELECT id, email, password_hash, name, avatar_url, status, totp_enabled, totp_secret_enc, totp_backup_codes_json, last_login_at, created_at, updated_at
		 FROM users WHERE id IN (%s)`,
		placeholders,
	)
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []core.User
	for rows.Next() {
		var u core.User
		var totpSecret sql.NullString
		var backupCodes sql.NullString
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Status,
			&u.TOTPEnabled, &totpSecret, &backupCodes, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.TOTPSecret = totpSecret.String
		u.TOTPBackupCodes = decodeTOTPBackupCodes(backupCodes.String)
		users = append(users, u)
	}
	return users, rows.Err()
}

// CountUsers returns the total number of users.
func (s *SQLiteDB) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}
