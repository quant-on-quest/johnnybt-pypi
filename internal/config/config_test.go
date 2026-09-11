package config

import (
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	t.Setenv("PYPI_ADDR", "")
	t.Setenv("PYPI_TLS_DOMAINS", "")
	c := Load()
	if c.Addr != ":8080" || c.DataDir != "./data" || c.SessionTTL != 7*24*time.Hour || c.MaxUploadSize != 512<<20 || len(c.TLSDomains) != 0 {
		t.Errorf("defaults: %+v", c)
	}
}

func TestTLSDomainsSwitchDefaults(t *testing.T) {
	t.Setenv("PYPI_ADDR", "")
	t.Setenv("PYPI_BASE_URL", "")
	t.Setenv("PYPI_TLS_DOMAINS", "pypi.example.com, mirror.example.com,")
	c := Load()
	if len(c.TLSDomains) != 2 || c.TLSDomains[1] != "mirror.example.com" {
		t.Errorf("TLSDomains = %v", c.TLSDomains)
	}
	if c.Addr != ":443" || c.HTTPAddr != ":80" || c.BaseURL != "https://pypi.example.com" {
		t.Errorf("tls defaults: %+v", c)
	}
	t.Setenv("PYPI_BASE_URL", "https://custom.example.com/")
	if c := Load(); c.BaseURL != "https://custom.example.com" {
		t.Errorf("explicit base url should win and lose its trailing slash: %q", c.BaseURL)
	}
}

func TestEnvParsing(t *testing.T) {
	t.Setenv("PYPI_SESSION_TTL", "48h")
	t.Setenv("PYPI_MAX_UPLOAD_MB", "10")
	c := Load()
	if c.SessionTTL != 48*time.Hour || c.MaxUploadSize != 10<<20 {
		t.Errorf("parsed: %+v", c)
	}
	t.Setenv("PYPI_SESSION_TTL", "garbage")
	t.Setenv("PYPI_MAX_UPLOAD_MB", "ten")
	c = Load()
	if c.SessionTTL != 7*24*time.Hour || c.MaxUploadSize != 512<<20 {
		t.Errorf("bad values should fall back to defaults: %+v", c)
	}
}

func TestAdminPath(t *testing.T) {
	t.Setenv("PYPI_ADMIN_PATH", "")
	if c := Load(); c.AdminPath != "/admin" {
		t.Errorf("default AdminPath = %q", c.AdminPath)
	}
	cases := map[string]string{
		"manage":       "/manage",
		"/manage/":     "/manage",
		"/x/y/":        "/x/y",
		"  /secret-7 ": "/secret-7",
	}
	for in, want := range cases {
		t.Setenv("PYPI_ADMIN_PATH", in)
		if c := Load(); c.AdminPath != want {
			t.Errorf("PYPI_ADMIN_PATH=%q → %q, want %q", in, c.AdminPath, want)
		}
	}
}

func TestValidateRejectsReservedAdminPaths(t *testing.T) {
	for _, bad := range []string{"/", "/api", "/api/x", "/simple", "/files", "/legacy", "/assets", "/me"} {
		t.Setenv("PYPI_ADMIN_PATH", bad)
		if err := Load().Validate(); err == nil {
			t.Errorf("AdminPath %q should be rejected", bad)
		}
	}
	t.Setenv("PYPI_ADMIN_PATH", "/admin")
	if err := Load().Validate(); err != nil {
		t.Errorf("default should validate: %v", err)
	}
}

func TestBlobSettings(t *testing.T) {
	t.Setenv("PYPI_BLOB_URL", "")
	t.Setenv("PYPI_BLOB_SIGNED_URLS", "")
	c := Load()
	if c.BlobURL != "" || !c.BlobSignedURLs {
		t.Errorf("defaults: %+v", c)
	}
	t.Setenv("PYPI_BLOB_URL", "s3://bucket?region=oss-cn-hangzhou")
	t.Setenv("PYPI_BLOB_SIGNED_URLS", "false")
	c = Load()
	if c.BlobURL != "s3://bucket?region=oss-cn-hangzhou" || c.BlobSignedURLs {
		t.Errorf("parsed: %+v", c)
	}
}
