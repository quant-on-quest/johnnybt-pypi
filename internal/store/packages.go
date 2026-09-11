package store

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/quant-on-quest/johnnybt-pypi/internal/pkgmeta"
)

type Package struct {
	ID                     int64     `json:"id"`
	Name                   string    `json:"name"`
	NormalizedName         string    `json:"normalized_name"`
	Summary                string    `json:"summary"`
	Description            string    `json:"description"`
	DescriptionContentType string    `json:"description_content_type"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type PackageSummary struct {
	Package
	LatestVersion string `json:"latest_version"`
	ReleaseCount  int    `json:"release_count"`
	FileCount     int    `json:"file_count"`
	UserCount     int    `json:"user_count"`
}

type Release struct {
	ID           int64     `json:"id"`
	PackageID    int64     `json:"package_id"`
	Version      string    `json:"version"`
	Yanked       bool      `json:"yanked"`
	YankedReason string    `json:"yanked_reason"`
	CreatedAt    time.Time `json:"created_at"`
}

type File struct {
	ID             int64     `json:"id"`
	ReleaseID      int64     `json:"release_id"`
	Filename       string    `json:"filename"`
	SHA256         string    `json:"sha256"`
	Size           int64     `json:"size"`
	BlobKey        string    `json:"-"`
	RequiresPython string    `json:"requires_python"`
	MetadataSHA256 string    `json:"metadata_sha256"`
	UploadedBy     *int64    `json:"uploaded_by"`
	UploadedAt     time.Time `json:"uploaded_at"`
}

// ReleaseFile is a file joined with its release, as the simple index needs it.
type ReleaseFile struct {
	File
	Version      string `json:"version"`
	Yanked       bool   `json:"yanked"`
	YankedReason string `json:"yanked_reason"`
}

const packageCols = `p.id, p.name, p.normalized_name, p.summary, p.description, p.description_content_type, p.created_at, p.updated_at`
const releaseCols = `r.id, r.package_id, r.version, r.yanked, r.yanked_reason, r.created_at`
const fileCols = `f.id, f.release_id, f.filename, f.sha256, f.size, f.blob_key, f.requires_python, f.metadata_sha256, f.uploaded_by, f.uploaded_at`

func scanPackage(row scanner) (*Package, error) {
	var p Package
	var created, updated string
	if err := row.Scan(&p.ID, &p.Name, &p.NormalizedName, &p.Summary, &p.Description, &p.DescriptionContentType, &created, &updated); err != nil {
		return nil, notFound(err)
	}
	p.CreatedAt, p.UpdatedAt = parseTime(created), parseTime(updated)
	return &p, nil
}

func scanRelease(row scanner) (*Release, error) {
	var r Release
	var created string
	if err := row.Scan(&r.ID, &r.PackageID, &r.Version, &r.Yanked, &r.YankedReason, &created); err != nil {
		return nil, notFound(err)
	}
	r.CreatedAt = parseTime(created)
	return &r, nil
}

func scanFileInto(row scanner, f *File, extra ...any) error {
	var uploadedBy sql.NullInt64
	var uploaded string
	dest := []any{&f.ID, &f.ReleaseID, &f.Filename, &f.SHA256, &f.Size, &f.BlobKey, &f.RequiresPython, &f.MetadataSHA256, &uploadedBy, &uploaded}
	dest = append(dest, extra...)
	if err := row.Scan(dest...); err != nil {
		return notFound(err)
	}
	f.UploadedBy = nullInt(uploadedBy)
	f.UploadedAt = parseTime(uploaded)
	return nil
}

// --- packages ---------------------------------------------------------------

func (s *Store) GetPackage(ctx context.Context, normalized string) (*Package, error) {
	return scanPackage(s.db.QueryRowContext(ctx, `SELECT `+packageCols+` FROM packages p WHERE p.normalized_name = ?`, normalized))
}

func (s *Store) GetPackageByID(ctx context.Context, id int64) (*Package, error) {
	return scanPackage(s.db.QueryRowContext(ctx, `SELECT `+packageCols+` FROM packages p WHERE p.id = ?`, id))
}

// UpsertPackage creates the package if needed. When updateMeta is set the
// summary/description are refreshed from the incoming distribution.
func (s *Store) UpsertPackage(ctx context.Context, name, normalized, summary, description, contentType string, updateMeta bool) (*Package, error) {
	ts := now()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO packages (name, normalized_name, summary, description, description_content_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (normalized_name) DO UPDATE SET updated_at = excluded.updated_at`,
		name, normalized, summary, description, contentType, ts, ts); err != nil {
		return nil, err
	}
	if updateMeta {
		if err := s.exec(ctx, `UPDATE packages SET name = ?, summary = ?, description = ?, description_content_type = ? WHERE normalized_name = ?`,
			name, summary, description, contentType, normalized); err != nil {
			return nil, err
		}
	}
	return s.GetPackage(ctx, normalized)
}

func (s *Store) DeletePackage(ctx context.Context, id int64) error {
	return s.exec(ctx, `DELETE FROM packages WHERE id = ?`, id)
}

// ListPackages returns every package with counts and the highest version.
func (s *Store) ListPackages(ctx context.Context) ([]PackageSummary, error) {
	return s.listPackages(ctx, `SELECT `+packageCols+`,
		(SELECT COUNT(*) FROM releases r WHERE r.package_id = p.id),
		(SELECT COUNT(*) FROM files f JOIN releases r ON r.id = f.release_id WHERE r.package_id = p.id),
		(SELECT COUNT(*) FROM entitlements e WHERE e.package_id = p.id)
		FROM packages p ORDER BY p.normalized_name`)
}

// ListPackagesForUser returns packages the user currently holds an unexpired
// entitlement for.
func (s *Store) ListPackagesForUser(ctx context.Context, userID int64) ([]PackageSummary, error) {
	return s.listPackages(ctx, `SELECT `+packageCols+`,
		(SELECT COUNT(*) FROM releases r WHERE r.package_id = p.id),
		(SELECT COUNT(*) FROM files f JOIN releases r ON r.id = f.release_id WHERE r.package_id = p.id),
		(SELECT COUNT(*) FROM entitlements e WHERE e.package_id = p.id)
		FROM packages p JOIN entitlements e ON e.package_id = p.id
		WHERE e.user_id = ? AND (e.expires_at IS NULL OR e.expires_at > ?)
		ORDER BY p.normalized_name`, userID, now())
}

func (s *Store) listPackages(ctx context.Context, query string, args ...any) ([]PackageSummary, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PackageSummary{}
	for rows.Next() {
		var ps PackageSummary
		var created, updated string
		if err := rows.Scan(&ps.ID, &ps.Name, &ps.NormalizedName, &ps.Summary, &ps.Description, &ps.DescriptionContentType,
			&created, &updated, &ps.ReleaseCount, &ps.FileCount, &ps.UserCount); err != nil {
			return nil, err
		}
		ps.CreatedAt, ps.UpdatedAt = parseTime(created), parseTime(updated)
		out = append(out, ps)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillLatestVersions(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) fillLatestVersions(ctx context.Context, pkgs []PackageSummary) error {
	if len(pkgs) == 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT package_id, version FROM releases WHERE yanked = 0`)
	if err != nil {
		return err
	}
	defer rows.Close()
	latest := map[int64]string{}
	for rows.Next() {
		var id int64
		var v string
		if err := rows.Scan(&id, &v); err != nil {
			return err
		}
		if cur, ok := latest[id]; !ok || pkgmeta.CompareVersions(v, cur) > 0 {
			latest[id] = v
		}
	}
	for i := range pkgs {
		pkgs[i].LatestVersion = latest[pkgs[i].ID]
	}
	return rows.Err()
}

// LatestVersion returns the highest non-yanked version of a package ("" if none).
func (s *Store) LatestVersion(ctx context.Context, packageID int64) (string, error) {
	rels, err := s.ListReleases(ctx, packageID)
	if err != nil {
		return "", err
	}
	for _, r := range rels { // sorted descending
		if !r.Yanked {
			return r.Version, nil
		}
	}
	return "", nil
}

// --- releases ---------------------------------------------------------------

func (s *Store) GetRelease(ctx context.Context, packageID int64, version string) (*Release, error) {
	return scanRelease(s.db.QueryRowContext(ctx, `SELECT `+releaseCols+` FROM releases r WHERE r.package_id = ? AND r.version = ?`, packageID, version))
}

func (s *Store) GetOrCreateRelease(ctx context.Context, packageID int64, version string) (*Release, error) {
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO releases (package_id, version, created_at) VALUES (?, ?, ?)`,
		packageID, version, now()); err != nil {
		return nil, err
	}
	return s.GetRelease(ctx, packageID, version)
}

// ListReleases returns releases sorted newest version first.
func (s *Store) ListReleases(ctx context.Context, packageID int64) ([]Release, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+releaseCols+` FROM releases r WHERE r.package_id = ?`, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Release{}
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return pkgmeta.CompareVersions(out[i].Version, out[j].Version) > 0 })
	return out, nil
}

func (s *Store) SetYanked(ctx context.Context, releaseID int64, yanked bool, reason string) error {
	if !yanked {
		reason = ""
	}
	return s.exec(ctx, `UPDATE releases SET yanked = ?, yanked_reason = ? WHERE id = ?`, yanked, reason, releaseID)
}

func (s *Store) DeleteRelease(ctx context.Context, releaseID int64) error {
	return s.exec(ctx, `DELETE FROM releases WHERE id = ?`, releaseID)
}

// --- files ------------------------------------------------------------------

func (s *Store) CreateFile(ctx context.Context, f *File) error {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO files (release_id, filename, sha256, size, blob_key, requires_python, metadata_sha256, uploaded_by, uploaded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ReleaseID, f.Filename, f.SHA256, f.Size, f.BlobKey, f.RequiresPython, f.MetadataSHA256, f.UploadedBy, now())
	if err != nil {
		return err
	}
	f.ID, _ = res.LastInsertId()
	f.UploadedAt = time.Now().UTC()
	return nil
}

func (s *Store) FileExists(ctx context.Context, filename string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM files WHERE filename = ?`, filename).Scan(&n)
	return n > 0, err
}

// FileByName resolves a filename to the file, its release and its package.
func (s *Store) FileByName(ctx context.Context, filename string) (*File, *Release, *Package, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+fileCols+`, `+releaseCols+`, `+packageCols+`
		FROM files f JOIN releases r ON r.id = f.release_id JOIN packages p ON p.id = r.package_id
		WHERE f.filename = ?`, filename)
	var f File
	var r Release
	var p Package
	var rCreated, pCreated, pUpdated string
	if err := scanFileInto(row, &f,
		&r.ID, &r.PackageID, &r.Version, &r.Yanked, &r.YankedReason, &rCreated,
		&p.ID, &p.Name, &p.NormalizedName, &p.Summary, &p.Description, &p.DescriptionContentType, &pCreated, &pUpdated); err != nil {
		return nil, nil, nil, err
	}
	r.CreatedAt = parseTime(rCreated)
	p.CreatedAt, p.UpdatedAt = parseTime(pCreated), parseTime(pUpdated)
	return &f, &r, &p, nil
}

// ListPackageFiles returns every file of a package, newest version first.
func (s *Store) ListPackageFiles(ctx context.Context, packageID int64) ([]ReleaseFile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+fileCols+`, r.version, r.yanked, r.yanked_reason
		FROM files f JOIN releases r ON r.id = f.release_id
		WHERE r.package_id = ? ORDER BY f.filename`, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReleaseFile{}
	for rows.Next() {
		var rf ReleaseFile
		if err := scanFileInto(rows, &rf.File, &rf.Version, &rf.Yanked, &rf.YankedReason); err != nil {
			return nil, err
		}
		out = append(out, rf)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if c := pkgmeta.CompareVersions(out[i].Version, out[j].Version); c != 0 {
			return c > 0
		}
		return out[i].Filename < out[j].Filename
	})
	return out, nil
}

func (s *Store) ListReleaseFiles(ctx context.Context, releaseID int64) ([]File, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+fileCols+` FROM files f WHERE f.release_id = ? ORDER BY f.filename`, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []File{}
	for rows.Next() {
		var f File
		if err := scanFileInto(rows, &f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
