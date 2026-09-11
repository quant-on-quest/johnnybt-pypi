package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	err := s.exec(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		id, userID, formatTime(time.Now().Add(ttl)))
	return id, err
}

// UserBySession resolves a session cookie to its user, or ErrNotFound when the
// session is missing or expired.
func (s *Store) UserBySession(ctx context.Context, id string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
		SELECT `+userCols+` FROM sessions se JOIN users u ON u.id = se.user_id
		WHERE se.id = ? AND se.expires_at > ?`, id, now()))
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return s.exec(ctx, `DELETE FROM sessions WHERE id = ?`, id)
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	return s.exec(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
}

func (s *Store) PurgeExpiredSessions(ctx context.Context) error {
	return s.exec(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now())
}
