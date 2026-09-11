// Package pkgmeta understands Python distribution filenames and core metadata
// just enough to index wheels and sdists.
package pkgmeta

import (
	"errors"
	"regexp"
	"strings"

	pep440 "github.com/aquasecurity/go-pep440-version"
)

var (
	normalizeRe    = regexp.MustCompile(`[-_.]+`)
	validNameRe    = regexp.MustCompile(`(?i)^([a-z0-9]|[a-z0-9][a-z0-9._-]*[a-z0-9])$`)
	safeFilenameRe = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
)

// NormalizeName applies PEP 503 normalization: lowercase, runs of -_. become -.
func NormalizeName(name string) string {
	return strings.ToLower(normalizeRe.ReplaceAllString(name, "-"))
}

// ValidName reports whether name is a legal project name (PEP 508).
func ValidName(name string) bool { return validNameRe.MatchString(name) }

// SafeFilename rejects anything that is not a plain distribution filename.
func SafeFilename(fn string) bool {
	return fn != "" && fn != "." && fn != ".." && safeFilenameRe.MatchString(fn)
}

type Kind int

const (
	KindUnknown Kind = iota
	KindWheel
	KindSdist
)

// KindOf classifies a filename by extension.
func KindOf(fn string) Kind {
	switch {
	case strings.HasSuffix(fn, ".whl"):
		return KindWheel
	case strings.HasSuffix(fn, ".tar.gz"), strings.HasSuffix(fn, ".zip"):
		return KindSdist
	}
	return KindUnknown
}

var ErrBadFilename = errors.New("unrecognised distribution filename")

// ParseFilename extracts the project name and version from a wheel or sdist
// filename. The returned name is as written in the filename, not normalized.
func ParseFilename(fn string) (name, version string, err error) {
	switch KindOf(fn) {
	case KindWheel:
		// {distribution}-{version}(-{build})?-{python}-{abi}-{platform}.whl
		parts := strings.Split(strings.TrimSuffix(fn, ".whl"), "-")
		if len(parts) != 5 && len(parts) != 6 {
			return "", "", ErrBadFilename
		}
		return parts[0], parts[1], nil
	case KindSdist:
		base := strings.TrimSuffix(strings.TrimSuffix(fn, ".tar.gz"), ".zip")
		i := strings.LastIndex(base, "-")
		if i <= 0 || i == len(base)-1 {
			return "", "", ErrBadFilename
		}
		return base[:i], base[i+1:], nil
	}
	return "", "", ErrBadFilename
}

// CompareVersions orders two PEP 440 versions. Unparseable versions sort
// before parseable ones and fall back to string comparison among themselves.
func CompareVersions(a, b string) int {
	va, errA := pep440.Parse(a)
	vb, errB := pep440.Parse(b)
	switch {
	case errA == nil && errB == nil:
		return va.Compare(vb)
	case errA == nil:
		return 1
	case errB == nil:
		return -1
	}
	return strings.Compare(a, b)
}

// ValidVersion reports whether v parses as PEP 440.
func ValidVersion(v string) bool {
	_, err := pep440.Parse(v)
	return err == nil
}
