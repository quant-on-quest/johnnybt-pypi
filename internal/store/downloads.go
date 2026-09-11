package store

import (
	"context"
	"database/sql"
	"time"
)

type Download struct {
	ID          int64     `json:"id"`
	UserID      *int64    `json:"user_id"`
	TokenID     *int64    `json:"token_id"`
	FileID      *int64    `json:"file_id"`
	PackageID   *int64    `json:"package_id"`
	Filename    string    `json:"filename"`
	IP          string    `json:"ip"`
	UserAgent   string    `json:"user_agent"`
	At          time.Time `json:"at"`
	Username    string    `json:"username"`
	PackageName string    `json:"package_name"`
}

func (s *Store) LogDownload(ctx context.Context, d *Download) error {
	return s.exec(ctx, `
		INSERT INTO downloads (user_id, token_id, file_id, package_id, filename, ip, user_agent, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.UserID, d.TokenID, d.FileID, d.PackageID, d.Filename, d.IP, d.UserAgent, now())
}

func (s *Store) ListDownloads(ctx context.Context, limit int) ([]Download, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.user_id, d.token_id, d.file_id, d.package_id, d.filename, d.ip, d.user_agent, d.at,
		       COALESCE(u.username, ''), COALESCE(p.name, '')
		FROM downloads d LEFT JOIN users u ON u.id = d.user_id LEFT JOIN packages p ON p.id = d.package_id
		ORDER BY d.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Download{}
	for rows.Next() {
		var d Download
		var userID, tokenID, fileID, packageID sql.NullInt64
		var at string
		if err := rows.Scan(&d.ID, &userID, &tokenID, &fileID, &packageID, &d.Filename, &d.IP, &d.UserAgent, &at, &d.Username, &d.PackageName); err != nil {
			return nil, err
		}
		d.UserID, d.TokenID, d.FileID, d.PackageID = nullInt(userID), nullInt(tokenID), nullInt(fileID), nullInt(packageID)
		d.At = parseTime(at)
		out = append(out, d)
	}
	return out, rows.Err()
}

// DownloadCounts returns the number of downloads per package id.
func (s *Store) DownloadCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT package_id, COUNT(*) FROM downloads WHERE package_id IS NOT NULL GROUP BY package_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
