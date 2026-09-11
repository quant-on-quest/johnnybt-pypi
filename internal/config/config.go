// Package config loads server settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	// Addr is the listen address, e.g. ":8080".
	Addr string
	// DataDir holds the SQLite database, blobs and the initial admin password file.
	DataDir string
	// BaseURL is the public URL clients use to reach the server, without a
	// trailing slash. When empty it is derived from each request.
	BaseURL string
	// SessionTTL is how long an admin login stays valid.
	SessionTTL time.Duration
	// MaxUploadSize caps a single uploaded distribution file.
	MaxUploadSize int64
	// TLSDomains enables built-in Let's Encrypt certificates for these hosts.
	// When set, Addr defaults to ":443" and HTTPAddr serves ACME + redirects.
	TLSDomains []string
	// TLSEmail is the optional ACME account contact.
	TLSEmail string
	// HTTPAddr is the plain-HTTP listener used only when TLS is enabled.
	HTTPAddr string
	// AdminPath is the URL prefix of the admin UI ("/admin"). The root serves
	// the customer self-check page, so an obscure prefix keeps the login form
	// out of sight.
	AdminPath string
	// BlobURL selects the object store, e.g. "s3://bucket?region=…". Empty
	// means the local blobs/ directory under DataDir.
	BlobURL string
	// BlobSignedURLs redirects downloads to short-lived signed URLs when the
	// backend supports them (S3-compatible stores); false streams through us.
	BlobSignedURLs bool
}

func (c Config) DBPath() string      { return filepath.Join(c.DataDir, "pypi.db") }
func (c Config) BlobDir() string     { return filepath.Join(c.DataDir, "blobs") }
func (c Config) TmpDir() string      { return filepath.Join(c.DataDir, "tmp") }
func (c Config) AdminPWFile() string { return filepath.Join(c.DataDir, "initial_admin_password") }
func (c Config) CertDir() string     { return filepath.Join(c.DataDir, "certs") }

func Load() Config {
	c := Config{
		DataDir:        env("PYPI_DATA_DIR", "./data"),
		BaseURL:        strings.TrimRight(env("PYPI_BASE_URL", ""), "/"),
		SessionTTL:     envDuration("PYPI_SESSION_TTL", 7*24*time.Hour),
		MaxUploadSize:  envInt64("PYPI_MAX_UPLOAD_MB", 512) << 20,
		TLSDomains:     splitList(env("PYPI_TLS_DOMAINS", "")),
		TLSEmail:       env("PYPI_TLS_EMAIL", ""),
		HTTPAddr:       env("PYPI_HTTP_ADDR", ":80"),
		AdminPath:      normalizePath(env("PYPI_ADMIN_PATH", "/admin")),
		BlobURL:        strings.TrimSpace(env("PYPI_BLOB_URL", "")),
		BlobSignedURLs: envBool("PYPI_BLOB_SIGNED_URLS", true),
	}
	if len(c.TLSDomains) > 0 {
		c.Addr = env("PYPI_ADDR", ":443")
		if c.BaseURL == "" {
			c.BaseURL = "https://" + c.TLSDomains[0]
		}
	} else {
		c.Addr = env("PYPI_ADDR", ":8080")
	}
	return c
}

// reservedPrefixes are owned by the API, the repository or static assets.
var reservedPrefixes = []string{"/api", "/simple", "/files", "/legacy", "/assets", "/me"}

// Validate rejects settings that would make the server misroute requests.
func (c Config) Validate() error {
	if c.AdminPath == "/" || c.AdminPath == "" {
		return errors.New("PYPI_ADMIN_PATH must not be the site root")
	}
	for _, p := range reservedPrefixes {
		if c.AdminPath == p || strings.HasPrefix(c.AdminPath, p+"/") {
			return fmt.Errorf("PYPI_ADMIN_PATH %q collides with reserved prefix %q", c.AdminPath, p)
		}
	}
	return nil
}

// normalizePath gives "/x/y" for "x/y/", " /x/y ", etc.
func normalizePath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	return "/" + p
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		var n int64
		for _, ch := range v {
			if ch < '0' || ch > '9' {
				return def
			}
			n = n*10 + int64(ch-'0')
		}
		return n
	}
	return def
}
