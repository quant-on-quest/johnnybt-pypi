package store

import (
	"context"
	"database/sql"
	"time"
)

type Entitlement struct {
	UserID    int64      `json:"user_id"`
	PackageID int64      `json:"package_id"`
	ExpiresAt *time.Time `json:"expires_at"`
	GrantedBy *int64     `json:"granted_by"`
	GrantedAt time.Time  `json:"granted_at"`
	// Active is false once ExpiresAt has passed.
	Active bool `json:"active"`
}

// UserEntitlement is an entitlement seen from the user's side.
type UserEntitlement struct {
	Entitlement
	PackageName    string `json:"package_name"`
	NormalizedName string `json:"normalized_name"`
	LatestVersion  string `json:"latest_version"`
}

// PackageEntitlement is an entitlement seen from the package's side.
type PackageEntitlement struct {
	Entitlement
	Username string `json:"username"`
	Note     string `json:"note"`
}

func scanEntitlement(e *Entitlement, expires sql.NullString, grantedBy sql.NullInt64, granted string) {
	e.ExpiresAt = parseTimePtr(expires)
	e.GrantedBy = nullInt(grantedBy)
	e.GrantedAt = parseTime(granted)
	e.Active = e.ExpiresAt == nil || e.ExpiresAt.After(time.Now())
}

// GrantEntitlement creates or updates a user's access to a package.
func (s *Store) GrantEntitlement(ctx context.Context, userID, packageID int64, expiresAt *time.Time, grantedBy int64) error {
	return s.exec(ctx, `
		INSERT INTO entitlements (user_id, package_id, expires_at, granted_by, granted_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id, package_id) DO UPDATE SET expires_at = excluded.expires_at,
			granted_by = excluded.granted_by, granted_at = excluded.granted_at`,
		userID, packageID, formatTimePtr(expiresAt), grantedBy, now())
}

func (s *Store) RevokeEntitlement(ctx context.Context, userID, packageID int64) error {
	return s.exec(ctx, `DELETE FROM entitlements WHERE user_id = ? AND package_id = ?`, userID, packageID)
}

// HasAccess reports whether the user holds an unexpired entitlement.
func (s *Store) HasAccess(ctx context.Context, userID, packageID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entitlements
		WHERE user_id = ? AND package_id = ? AND (expires_at IS NULL OR expires_at > ?)`,
		userID, packageID, now()).Scan(&n)
	return n > 0, err
}

func (s *Store) ListUserEntitlements(ctx context.Context, userID int64) ([]UserEntitlement, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.user_id, e.package_id, e.expires_at, e.granted_by, e.granted_at, p.name, p.normalized_name
		FROM entitlements e JOIN packages p ON p.id = e.package_id
		WHERE e.user_id = ? ORDER BY p.normalized_name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserEntitlement{}
	for rows.Next() {
		var ue UserEntitlement
		var expires sql.NullString
		var grantedBy sql.NullInt64
		var granted string
		if err := rows.Scan(&ue.UserID, &ue.PackageID, &expires, &grantedBy, &granted, &ue.PackageName, &ue.NormalizedName); err != nil {
			return nil, err
		}
		scanEntitlement(&ue.Entitlement, expires, grantedBy, granted)
		out = append(out, ue)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		v, err := s.LatestVersion(ctx, out[i].PackageID)
		if err != nil {
			return nil, err
		}
		out[i].LatestVersion = v
	}
	return out, nil
}

func (s *Store) ListPackageEntitlements(ctx context.Context, packageID int64) ([]PackageEntitlement, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.user_id, e.package_id, e.expires_at, e.granted_by, e.granted_at, u.username, u.note
		FROM entitlements e JOIN users u ON u.id = e.user_id
		WHERE e.package_id = ? ORDER BY u.username`, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PackageEntitlement{}
	for rows.Next() {
		var pe PackageEntitlement
		var expires sql.NullString
		var grantedBy sql.NullInt64
		var granted string
		if err := rows.Scan(&pe.UserID, &pe.PackageID, &expires, &grantedBy, &granted, &pe.Username, &pe.Note); err != nil {
			return nil, err
		}
		scanEntitlement(&pe.Entitlement, expires, grantedBy, granted)
		out = append(out, pe)
	}
	return out, rows.Err()
}
