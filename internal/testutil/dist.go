// Package testutil builds throwaway wheels and sdists for tests.
package testutil

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Metadata renders a minimal core-metadata document.
func Metadata(name, version string, extra map[string]string, description string) string {
	var b strings.Builder
	b.WriteString("Metadata-Version: 2.1\n")
	fmt.Fprintf(&b, "Name: %s\nVersion: %s\n", name, version)
	for _, k := range sortedKeys(extra) {
		fmt.Fprintf(&b, "%s: %s\n", k, extra[k])
	}
	if description != "" {
		b.WriteString("\n" + description + "\n")
	}
	return b.String()
}

// WheelBytes builds a wheel archive in memory. dist is the escaped project
// name as it appears in the filename (underscores).
func WheelBytes(t testing.TB, dist, version, metadata string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	distInfo := fmt.Sprintf("%s-%s.dist-info/", dist, version)
	files := map[string]string{
		dist + "/__init__.py": "__version__ = " + fmt.Sprintf("%q", version) + "\n",
		distInfo + "METADATA": metadata,
		distInfo + "WHEEL":    "Wheel-Version: 1.0\nGenerator: testutil\nRoot-Is-Purelib: true\nTag: py3-none-any\n",
		distInfo + "RECORD":   "",
	}
	for _, name := range sortedKeys(files) {
		content := files[name]
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// SdistBytes builds a .tar.gz sdist with a top-level PKG-INFO.
func SdistBytes(t testing.TB, dist, version, metadata string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	root := fmt.Sprintf("%s-%s/", dist, version)
	files := map[string]string{
		root + "PKG-INFO":       metadata,
		root + "pyproject.toml": "[project]\nname = \"" + dist + "\"\n",
		root + "src/x/PKG-INFO": "Name: decoy\n", // must be ignored: not top-level
	}
	for _, name := range sortedKeys(files) {
		content := files[name]
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// sortedKeys keeps archive layout deterministic so identical inputs give
// byte-identical archives.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// WriteFile drops content into dir and returns the path.
func WriteFile(t testing.TB, dir, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
