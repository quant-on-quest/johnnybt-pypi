package store

import (
	"context"
	"time"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	IsAdmin      bool      `json:"is_admin"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserSummary struct {
	User
	EntitlementCount int `json:"entitlement_count"`
	TokenCount       int `json:"token_count"`
}

const userCols = `u.id, u.username, u.password_hash, u.is_admin, u.note, u.created_at`

func scanUser(row scanner) (*User, error) {
	var u User
	var created string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.Note, &created); err != nil {
		return nil, notFound(err)
	}
	u.CreatedAt = parseTime(created)
	return &u, nil
}

func (s *Store) CreateUser(ctx context.Context, username, note string, isAdmin bool, passwordHash string) (*User, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, is_admin, note, created_at) VALUES (?, ?, ?, ?, ?)`,
		username, passwordHash, isAdmin, note, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetUser(ctx, id)
}

func (s *Store) GetUser(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users u WHERE u.id = ?`, id))
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users u WHERE u.username = ?`, username))
}

func (s *Store) ListUsers(ctx context.Context) ([]UserSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+userCols+`,
		       (SELECT COUNT(*) FROM entitlements e WHERE e.user_id = u.id),
		       (SELECT COUNT(*) FROM tokens t WHERE t.user_id = u.id AND t.revoked_at IS NULL)
		FROM users u ORDER BY u.is_admin DESC, u.username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserSummary{}
	for rows.Next() {
		var u UserSummary
		var created string
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.Note, &created,
			&u.EntitlementCount, &u.TokenCount); err != nil {
			return nil, err
		}
		u.CreatedAt = parseTime(created)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, id int64, username, note string) error {
	return s.exec(ctx, `UPDATE users SET username = ?, note = ? WHERE id = ?`, username, note, id)
}

func (s *Store) SetPassword(ctx context.Context, id int64, hash string) error {
	return s.exec(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	return s.exec(ctx, `DELETE FROM users WHERE id = ?`, id)
}

// AdminUser returns the first admin account, or ErrNotFound when none exists.
func (s *Store) AdminUser(ctx context.Context) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users u WHERE u.is_admin = 1 ORDER BY u.id LIMIT 1`))
}
