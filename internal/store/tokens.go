package store

import (
	"context"
	"database/sql"
	"time"
)

type Token struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scope      string     `json:"scope"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

const tokenCols = `t.id, t.user_id, t.name, t.prefix, t.scope, t.created_at, t.last_used_at, t.revoked_at`

func scanToken(row scanner) (*Token, error) {
	var t Token
	var created string
	var lastUsed, revoked sql.NullString
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.Scope, &created, &lastUsed, &revoked); err != nil {
		return nil, notFound(err)
	}
	t.CreatedAt = parseTime(created)
	t.LastUsedAt = parseTimePtr(lastUsed)
	t.RevokedAt = parseTimePtr(revoked)
	return &t, nil
}

func (s *Store) CreateToken(ctx context.Context, userID int64, name, prefix, hash, scope string) (*Token, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO tokens (user_id, name, prefix, hash, scope, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, name, prefix, hash, scope, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetToken(ctx, id)
}

func (s *Store) GetToken(ctx context.Context, id int64) (*Token, error) {
	return scanToken(s.db.QueryRowContext(ctx, `SELECT `+tokenCols+` FROM tokens t WHERE t.id = ?`, id))
}

// ActiveTokenByHash looks up a non-revoked token and its owner.
func (s *Store) ActiveTokenByHash(ctx context.Context, hash string) (*Token, *User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+tokenCols+`, `+userCols+`
		FROM tokens t JOIN users u ON u.id = t.user_id
		WHERE t.hash = ? AND t.revoked_at IS NULL`, hash)
	var t Token
	var u User
	var tCreated, uCreated string
	var lastUsed, revoked sql.NullString
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.Scope, &tCreated, &lastUsed, &revoked,
		&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.Note, &uCreated); err != nil {
		return nil, nil, notFound(err)
	}
	t.CreatedAt = parseTime(tCreated)
	t.LastUsedAt = parseTimePtr(lastUsed)
	t.RevokedAt = parseTimePtr(revoked)
	u.CreatedAt = parseTime(uCreated)
	return &t, &u, nil
}

func (s *Store) ListTokens(ctx context.Context, userID int64) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+tokenCols+` FROM tokens t WHERE t.user_id = ? ORDER BY t.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeToken(ctx context.Context, id int64) error {
	return s.exec(ctx, `UPDATE tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now(), id)
}

func (s *Store) TouchToken(ctx context.Context, id int64) error {
	return s.exec(ctx, `UPDATE tokens SET last_used_at = ? WHERE id = ?`, now(), id)
}
