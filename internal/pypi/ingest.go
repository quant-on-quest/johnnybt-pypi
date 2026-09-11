// Package pypi implements the client-facing repository API: the PEP 503/691
// simple index, file downloads and the legacy upload endpoint.
package pypi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/quant-on-quest/johnnybt-pypi/internal/blob"
	"github.com/quant-on-quest/johnnybt-pypi/internal/pkgmeta"
	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
)

var (
	ErrFileExists     = errors.New("File already exists") // twine matches on this phrase for --skip-existing
	ErrDigestMismatch = errors.New("sha256 digest does not match uploaded content")
	ErrBadFile        = errors.New("invalid distribution file")
	ErrTooLarge       = errors.New("file exceeds the maximum upload size")
)

type IngestResult struct {
	Package *store.Package `json:"package"`
	Release *store.Release `json:"release"`
	File    *store.File    `json:"file"`
}

// Ingester validates an uploaded distribution, stores it and indexes it.
type Ingester struct {
	Store   *store.Store
	Blobs   blob.Store
	TmpDir  string
	MaxSize int64
}

// Ingest reads one distribution file from src. expectedSHA, when non-empty, is
// the hex digest the client claims; a mismatch aborts the upload.
func (in *Ingester) Ingest(ctx context.Context, filename string, src io.Reader, expectedSHA string, uploadedBy int64) (*IngestResult, error) {
	if !pkgmeta.SafeFilename(filename) {
		return nil, fmt.Errorf("%w: unsafe filename %q", ErrBadFile, filename)
	}
	kind := pkgmeta.KindOf(filename)
	if kind == pkgmeta.KindUnknown {
		return nil, fmt.Errorf("%w: only .whl, .tar.gz and .zip are accepted", ErrBadFile)
	}
	fnName, fnVersion, err := pkgmeta.ParseFilename(filename)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
	}
	if exists, err := in.Store.FileExists(ctx, filename); err != nil {
		return nil, err
	} else if exists {
		return nil, ErrFileExists
	}

	// Spool to disk so the archive can be opened with random access.
	if err := os.MkdirAll(in.TmpDir, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(in.TmpDir, "upload-*"+suffixOf(filename))
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	hasher := sha256.New()
	limit := in.MaxSize
	if limit <= 0 {
		limit = 1 << 40
	}
	n, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(src, limit+1))
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, ErrTooLarge
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	if expectedSHA != "" && !strings.EqualFold(expectedSHA, sum) {
		return nil, ErrDigestMismatch
	}

	var raw []byte
	switch kind {
	case pkgmeta.KindWheel:
		raw, err = pkgmeta.WheelMetadata(tmp.Name())
	case pkgmeta.KindSdist:
		raw, err = pkgmeta.SdistMetadata(tmp.Name())
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
	}
	md, err := pkgmeta.ParseMetadata(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadFile, err)
	}
	if md.Name == "" {
		md.Name = fnName
	}
	if md.Version == "" {
		md.Version = fnVersion
	}
	if !pkgmeta.ValidName(md.Name) {
		return nil, fmt.Errorf("%w: invalid project name %q", ErrBadFile, md.Name)
	}
	if !pkgmeta.ValidVersion(md.Version) {
		return nil, fmt.Errorf("%w: invalid version %q", ErrBadFile, md.Version)
	}
	normalized := pkgmeta.NormalizeName(md.Name)
	if pkgmeta.NormalizeName(fnName) != normalized {
		return nil, fmt.Errorf("%w: filename project %q does not match metadata %q", ErrBadFile, fnName, md.Name)
	}
	if pkgmeta.CompareVersions(fnVersion, md.Version) != 0 {
		return nil, fmt.Errorf("%w: filename version %q does not match metadata %q", ErrBadFile, fnVersion, md.Version)
	}

	blobKey := sum + "/" + filename
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if err := in.Blobs.Put(ctx, blobKey, tmp); err != nil {
		return nil, fmt.Errorf("store blob: %w", err)
	}
	metadataSHA := ""
	if kind == pkgmeta.KindWheel { // PEP 658 metadata is only reliable for wheels
		if err := in.Blobs.Put(ctx, blobKey+".metadata", strings.NewReader(string(raw))); err != nil {
			in.Blobs.Delete(ctx, blobKey)
			return nil, fmt.Errorf("store metadata: %w", err)
		}
		h := sha256.Sum256(raw)
		metadataSHA = hex.EncodeToString(h[:])
	}

	// Refresh the package summary/description when this is the newest version.
	updateMeta := true
	if existing, err := in.Store.GetPackage(ctx, normalized); err == nil {
		if latest, err := in.Store.LatestVersion(ctx, existing.ID); err == nil && latest != "" {
			updateMeta = pkgmeta.CompareVersions(md.Version, latest) >= 0
		}
	}
	pkg, err := in.Store.UpsertPackage(ctx, md.Name, normalized, md.Summary, md.Description, md.DescriptionContentType, updateMeta)
	if err != nil {
		return nil, in.rollbackBlob(ctx, blobKey, metadataSHA != "", err)
	}
	rel, err := in.Store.GetOrCreateRelease(ctx, pkg.ID, md.Version)
	if err != nil {
		return nil, in.rollbackBlob(ctx, blobKey, metadataSHA != "", err)
	}
	f := &store.File{
		ReleaseID:      rel.ID,
		Filename:       filename,
		SHA256:         sum,
		Size:           n,
		BlobKey:        blobKey,
		RequiresPython: md.RequiresPython,
		MetadataSHA256: metadataSHA,
		UploadedBy:     &uploadedBy,
	}
	if err := in.Store.CreateFile(ctx, f); err != nil {
		return nil, in.rollbackBlob(ctx, blobKey, metadataSHA != "", err)
	}
	return &IngestResult{Package: pkg, Release: rel, File: f}, nil
}

func (in *Ingester) rollbackBlob(ctx context.Context, key string, hasMeta bool, cause error) error {
	in.Blobs.Delete(ctx, key)
	if hasMeta {
		in.Blobs.Delete(ctx, key+".metadata")
	}
	return cause
}

// DeleteFileBlobs removes the stored objects behind a file record.
func DeleteFileBlobs(ctx context.Context, blobs blob.Store, f *store.File) {
	blobs.Delete(ctx, f.BlobKey)
	if f.MetadataSHA256 != "" {
		blobs.Delete(ctx, f.BlobKey+".metadata")
	}
}

func suffixOf(filename string) string {
	switch {
	case strings.HasSuffix(filename, ".tar.gz"):
		return ".tar.gz"
	case strings.HasSuffix(filename, ".zip"):
		return ".zip"
	case strings.HasSuffix(filename, ".whl"):
		return ".whl"
	}
	return ""
}
