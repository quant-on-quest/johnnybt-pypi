package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quant-on-quest/johnnybt-pypi/internal/auth"
	"github.com/quant-on-quest/johnnybt-pypi/internal/config"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{DataDir: t.TempDir(), Addr: "127.0.0.1:0", SessionTTL: time.Hour, MaxUploadSize: 1 << 20, AdminPath: "/admin", BlobSignedURLs: true}
}

func newServer(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	s, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestEnsureAdminBootstrapsOnce(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	s := newServer(t, cfg)

	user, pw, created, err := s.EnsureAdmin(ctx)
	if err != nil || !created || user != "admin" || len(pw) != 19 {
		t.Fatalf("first EnsureAdmin = %q %q %v %v", user, pw, created, err)
	}
	u, err := s.Store().GetUserByUsername(ctx, "admin")
	if err != nil || !u.IsAdmin || !auth.VerifyPassword(u.PasswordHash, pw) {
		t.Errorf("admin not persisted correctly: %+v %v", u, err)
	}

	if _, _, created, err := s.EnsureAdmin(ctx); err != nil || created {
		t.Errorf("second EnsureAdmin should be a no-op, got created=%v err=%v", created, err)
	}

	path, err := WriteInitialPasswordFile(cfg, user, pw)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	st, _ := os.Stat(path)
	if !strings.Contains(string(b), pw) || st.Mode().Perm() != 0o600 {
		t.Errorf("password file: %q mode %v", b, st.Mode())
	}
}

func TestResetAdminPassword(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	if _, _, err := ResetAdminPassword(ctx, cfg); err == nil {
		t.Error("reset before bootstrap should fail")
	}
	s := newServer(t, cfg)
	_, oldPW, _, _ := s.EnsureAdmin(ctx)
	sid, _ := s.Store().CreateSession(ctx, 1, time.Hour)
	s.Close() // reset opens its own connection

	user, newPW, err := ResetAdminPassword(ctx, cfg)
	if err != nil || user != "admin" || newPW == oldPW {
		t.Fatalf("reset = %q %q %v", user, newPW, err)
	}
	s2 := newServer(t, cfg)
	u, _ := s2.Store().GetUserByUsername(ctx, "admin")
	if !auth.VerifyPassword(u.PasswordHash, newPW) || auth.VerifyPassword(u.PasswordHash, oldPW) {
		t.Error("password not rotated")
	}
	if _, err := s2.Store().UserBySession(ctx, sid); err == nil {
		t.Error("existing sessions should be invalidated on reset")
	}
}

func TestRoutingAndSPAFallback(t *testing.T) {
	cfg := testConfig(t)
	s := newServer(t, cfg)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	get := func(path string, hdr map[string]string) (int, string, http.Header) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b), res.Header
	}

	// Repository routes are mounted and require auth.
	if status, _, hdr := get("/simple/", nil); status != 401 || hdr.Get("WWW-Authenticate") == "" {
		t.Errorf("/simple/: %d", status)
	}
	// API routes are mounted.
	if status, _, _ := get("/api/v1/auth/me", nil); status != 401 {
		t.Errorf("/api/v1/auth/me: %d", status)
	}
	// SPA: root and deep links both serve index.html.
	for _, p := range []string{"/", "/users/3", "/packages/johnnybt-demo"} {
		status, body, _ := get(p, nil)
		if status != 200 || !strings.Contains(body, "<html") {
			t.Errorf("%s: %d %q", p, status, body)
		}
	}
	// Unknown API paths are 404, not the SPA.
	if status, body, _ := get("/api/v1/nope", nil); status != 404 || strings.Contains(body, "<html") {
		t.Errorf("/api/v1/nope: %d %q", status, body)
	}
}

func TestPanicIsRecovered(t *testing.T) {
	cfg := testConfig(t)
	s := newServer(t, cfg)
	s.mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	res, err := http.Get(srv.URL + "/boom")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 500 {
		t.Errorf("panic → %d, want 500", res.StatusCode)
	}
}

func TestRunServesAndStopsOnCancel(t *testing.T) {
	cfg := testConfig(t)
	cfg.Addr = "127.0.0.1:0"
	// Bind a free port ourselves so we know where to connect.
	ln, err := freePort()
	if err != nil {
		t.Skip("no free port:", err)
	}
	cfg.Addr = ln
	s := newServer(t, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	var res *http.Response
	for i := 0; i < 50; i++ {
		res, err = http.Get("http://" + cfg.Addr + "/api/v1/auth/me")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("server never came up:", err)
	}
	res.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Run did not stop after cancel")
	}
}

func TestHTTPSRedirectHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://pypi.example.com:80/simple/x/?a=1", nil)
	redirectHTTPS(rec, req)
	if rec.Code != 301 || rec.Header().Get("Location") != "https://pypi.example.com/simple/x/?a=1" {
		t.Errorf("redirect = %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestDataDirIsCreated(t *testing.T) {
	cfg := testConfig(t)
	cfg.DataDir = filepath.Join(cfg.DataDir, "nested", "data")
	newServer(t, cfg)
	for _, d := range []string{cfg.BlobDir(), cfg.TmpDir()} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Errorf("%s not created", d)
		}
	}
}

func freePort() (string, error) {
	l, err := netListen()
	if err != nil {
		return "", err
	}
	addr := l.Addr().String()
	l.Close()
	return addr, nil
}

func TestOpenBlobsSelectsBackend(t *testing.T) {
	cfg := testConfig(t)
	local, err := OpenBlobs(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	local.Close()
	if _, err := os.Stat(cfg.BlobDir()); err != nil {
		t.Error("default backend should create the local blob dir")
	}
	cfg.BlobURL = "mem://"
	mem, err := OpenBlobs(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	mem.Close()
	cfg.BlobURL = "bogus://x"
	if _, err := OpenBlobs(context.Background(), cfg); err == nil {
		t.Error("unknown scheme should fail fast")
	}
}

func TestNewRejectsInvalidAdminPath(t *testing.T) {
	cfg := testConfig(t)
	cfg.AdminPath = "/api"
	if _, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Error("server must refuse a reserved admin path")
	}
}

func TestIndexCarriesAdminPath(t *testing.T) {
	cfg := testConfig(t)
	cfg.AdminPath = "/manage-9"
	s := newServer(t, cfg)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), `"adminPath":"/manage-9"`) {
		t.Errorf("index missing admin path:\n%s", b)
	}
}
