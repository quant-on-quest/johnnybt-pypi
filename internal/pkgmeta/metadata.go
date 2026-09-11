package pkgmeta

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"path"
	"strings"
)

// Metadata is the subset of core metadata (PEP 566) the index needs.
type Metadata struct {
	Name                   string
	Version                string
	Summary                string
	RequiresPython         string
	Description            string
	DescriptionContentType string
}

// ParseMetadata parses a METADATA / PKG-INFO document.
func ParseMetadata(raw []byte) (Metadata, error) {
	r := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw)))
	hdr, err := r.ReadMIMEHeader()
	if err != nil && !errors.Is(err, io.EOF) {
		return Metadata{}, fmt.Errorf("parse metadata headers: %w", err)
	}
	body, _ := io.ReadAll(r.R)
	m := Metadata{
		Name:                   hdr.Get("Name"),
		Version:                hdr.Get("Version"),
		Summary:                hdr.Get("Summary"),
		RequiresPython:         hdr.Get("Requires-Python"),
		Description:            hdr.Get("Description"),
		DescriptionContentType: hdr.Get("Description-Content-Type"),
	}
	if m.Description == "" {
		m.Description = strings.TrimSpace(string(body))
	}
	return m, nil
}

// WheelMetadata returns the raw METADATA file from a wheel on disk.
func WheelMetadata(p string) ([]byte, error) {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, fmt.Errorf("open wheel: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		dir, base := path.Split(f.Name)
		if base == "METADATA" && strings.HasSuffix(strings.TrimSuffix(dir, "/"), ".dist-info") && strings.Count(dir, "/") == 1 {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, 16<<20))
		}
	}
	return nil, errors.New("wheel has no .dist-info/METADATA")
}

// SdistMetadata returns the top-level PKG-INFO from a .tar.gz or .zip sdist.
func SdistMetadata(p string) ([]byte, error) {
	if strings.HasSuffix(p, ".zip") {
		zr, err := zip.OpenReader(p)
		if err != nil {
			return nil, fmt.Errorf("open sdist: %w", err)
		}
		defer zr.Close()
		for _, f := range zr.File {
			if isTopLevelPkgInfo(f.Name) {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 16<<20))
			}
		}
		return nil, errors.New("sdist has no PKG-INFO")
	}
	fh, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return nil, fmt.Errorf("open sdist: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && isTopLevelPkgInfo(h.Name) {
			return io.ReadAll(io.LimitReader(tr, 16<<20))
		}
	}
	return nil, errors.New("sdist has no PKG-INFO")
}

// isTopLevelPkgInfo matches "<root>/PKG-INFO" and nothing deeper.
func isTopLevelPkgInfo(name string) bool {
	name = strings.TrimPrefix(name, "./")
	parts := strings.Split(name, "/")
	return len(parts) == 2 && parts[1] == "PKG-INFO"
}
