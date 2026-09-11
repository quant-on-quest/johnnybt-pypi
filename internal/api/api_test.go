package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/quant-on-quest/johnnybt-pypi/internal/auth"
	"github.com/quant-on-quest/johnnybt-pypi/internal/blob"
	"github.com/quant-on-quest/johnnybt-pypi/internal/config"
	"github.com/quant-on-quest/johnnybt-pypi/internal/pypi"
	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
	"github.com/quant-on-quest/johnnybt-pypi/internal/testutil"
)

const adminPassword = "admin-secret-1"

type fixture struct {
	t      *testing.T
	dir    string
	srv    *httptest.Server
	store  *store.Store
	client *http.Client // carries the session cookie
	anon   *http.Client
	admin  *store.User
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	blobs, _ := blob.NewLocal(filepath.Join(dir, "blobs"))
	cfg := config.Config{DataDir: dir, SessionTTL: time.Hour, MaxUploadSize: 1 << 20, AdminPath: "/admin"}
	ing := &pypi.Ingester{Store: st, Blobs: blobs, TmpDir: filepath.Join(dir, "tmp"), MaxSize: cfg.MaxUploadSize}
	a := &API{Store: st, Blobs: blobs, Ingester: ing, Cfg: cfg, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	mux := http.NewServeMux()
	a.Register(mux)
	// Same wrapping as production so cross-origin protection is under test.
	srv := httptest.NewServer(http.NewCrossOriginProtection().Handler(mux))
	t.Cleanup(srv.Close)

	hash, _ := auth.HashPassword(adminPassword)
	admin, err := st.CreateUser(context.Background(), "admin", "", true, hash)
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	return &fixture{t: t, dir: dir, srv: srv, store: st, client: &http.Client{Jar: jar}, anon: &http.Client{}, admin: admin}
}

func (f *fixture) req(c *http.Client, method, path string, body any, hdr map[string]string) (int, []byte) {
	f.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, f.srv.URL+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// A same-origin browser request; tests that care set their own value.
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := c.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (f *fixture) login() {
	f.t.Helper()
	status, body := f.req(f.client, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": adminPassword}, nil)
	if status != 200 {
		f.t.Fatalf("login: %d %s", status, body)
	}
}

func (f *fixture) mustJSON(b []byte, v any) {
	f.t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		f.t.Fatalf("bad json %q: %v", b, err)
	}
}

func demoWheel(t *testing.T, version string) (string, []byte) {
	md := testutil.Metadata("johnnybt-demo", version, map[string]string{"Summary": "Demo"}, "readme")
	return "johnnybt_demo-" + version + "-py3-none-any.whl", testutil.WheelBytes(t, "johnnybt_demo", version, md)
}

func (f *fixture) uploadUI(files map[string][]byte) (int, []byte) {
	f.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, content := range files {
		w, _ := mw.CreateFormFile("files", name)
		w.Write(content)
	}
	mw.Close()
	req, _ := http.NewRequest("POST", f.srv.URL+"/api/v1/packages/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	res, err := f.client.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

// --- auth -------------------------------------------------------------------

func TestLoginLogout(t *testing.T) {
	f := newFixture(t)
	if status, _ := f.req(f.client, "GET", "/api/v1/auth/me", nil, nil); status != 401 {
		t.Errorf("me before login: %d", status)
	}
	if status, _ := f.req(f.client, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "wrong"}, nil); status != 401 {
		t.Errorf("wrong password: %d", status)
	}
	if status, _ := f.req(f.client, "POST", "/api/v1/auth/login", map[string]string{"username": "nobody", "password": "x"}, nil); status != 401 {
		t.Errorf("unknown user: %d", status)
	}
	f.login()
	status, body := f.req(f.client, "GET", "/api/v1/auth/me", nil, nil)
	var me store.User
	f.mustJSON(body, &me)
	if status != 200 || me.Username != "admin" || !me.IsAdmin {
		t.Errorf("me after login: %d %s", status, body)
	}
	if strings.Contains(string(body), "password_hash") {
		t.Error("password hash must never be serialised")
	}
	if status, _ := f.req(f.client, "POST", "/api/v1/auth/logout", nil, nil); status != 204 {
		t.Errorf("logout: %d", status)
	}
	if status, _ := f.req(f.client, "GET", "/api/v1/auth/me", nil, nil); status != 401 {
		t.Errorf("me after logout: %d", status)
	}
}

func TestCustomersCannotLogIn(t *testing.T) {
	f := newFixture(t)
	hash, _ := auth.HashPassword("customer-pass")
	f.store.CreateUser(context.Background(), "alice", "", false, hash)
	if status, _ := f.req(f.client, "POST", "/api/v1/auth/login", map[string]string{"username": "alice", "password": "customer-pass"}, nil); status != 403 {
		t.Errorf("customer login: %d, want 403", status)
	}
}

func TestAdminRoutesNeedSession(t *testing.T) {
	f := newFixture(t)
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/packages"}, {"GET", "/api/v1/users"}, {"POST", "/api/v1/users"},
		{"GET", "/api/v1/downloads"}, {"GET", "/api/v1/config"}, {"PUT", "/api/v1/auth/password"},
	} {
		if status, _ := f.req(f.anon, c.method, c.path, map[string]string{}, nil); status != 401 {
			t.Errorf("%s %s anonymous: %d, want 401", c.method, c.path, status)
		}
	}
}

func TestCrossSiteRequestsAreRejected(t *testing.T) {
	f := newFixture(t)
	f.login()
	status, _ := f.req(f.client, "POST", "/api/v1/users", map[string]string{"username": "evil"}, map[string]string{"Sec-Fetch-Site": "cross-site"})
	if status != http.StatusForbidden {
		t.Errorf("cross-site POST with a valid cookie: %d, want 403", status)
	}
	if _, err := f.store.GetUserByUsername(context.Background(), "evil"); err == nil {
		t.Error("cross-site request must not have side effects")
	}
}

func TestChangePassword(t *testing.T) {
	f := newFixture(t)
	f.login()
	if status, _ := f.req(f.client, "PUT", "/api/v1/auth/password", map[string]string{"current": "wrong", "new": "new-password-1"}, nil); status != 403 {
		t.Errorf("wrong current: %d", status)
	}
	if status, _ := f.req(f.client, "PUT", "/api/v1/auth/password", map[string]string{"current": adminPassword, "new": "short"}, nil); status != 400 {
		t.Errorf("weak new: %d", status)
	}
	if status, _ := f.req(f.client, "PUT", "/api/v1/auth/password", map[string]string{"current": adminPassword, "new": "new-password-1"}, nil); status != 204 {
		t.Errorf("change: %d", status)
	}
	u, _ := f.store.GetUser(context.Background(), f.admin.ID)
	if !auth.VerifyPassword(u.PasswordHash, "new-password-1") {
		t.Error("new password not stored")
	}
}

// --- users / entitlements / tokens ------------------------------------------

func TestUserLifecycle(t *testing.T) {
	f := newFixture(t)
	f.login()

	status, body := f.req(f.client, "POST", "/api/v1/users", map[string]string{"username": "alice", "note": "微信 alice"}, nil)
	if status != 201 {
		t.Fatalf("create: %d %s", status, body)
	}
	var alice store.User
	f.mustJSON(body, &alice)
	if alice.IsAdmin || alice.Note != "微信 alice" {
		t.Errorf("created user wrong: %+v", alice)
	}
	if status, _ := f.req(f.client, "POST", "/api/v1/users", map[string]string{"username": "ALICE"}, nil); status != 409 {
		t.Errorf("duplicate (case-insensitive): %d, want 409", status)
	}
	for _, bad := range []string{"", "has space", "a:b", "a/b", strings.Repeat("x", 65)} {
		if status, _ := f.req(f.client, "POST", "/api/v1/users", map[string]string{"username": bad}, nil); status != 400 {
			t.Errorf("username %q: %d, want 400", bad, status)
		}
	}

	status, body = f.req(f.client, "GET", "/api/v1/users", nil, nil)
	var list []store.UserSummary
	f.mustJSON(body, &list)
	if status != 200 || len(list) != 2 || list[0].Username != "admin" || list[1].Username != "alice" {
		t.Errorf("list: %d %s", status, body)
	}

	path := "/api/v1/users/" + itoa(alice.ID)
	if status, _ := f.req(f.client, "PUT", path, map[string]string{"username": "alice2", "note": "renamed"}, nil); status != 200 {
		t.Errorf("update: %d", status)
	}
	if status, _ := f.req(f.client, "PUT", path, map[string]string{"username": "admin"}, nil); status != 409 {
		t.Errorf("rename onto existing: %d, want 409", status)
	}
	status, body = f.req(f.client, "GET", path, nil, nil)
	var detail struct {
		User         store.User              `json:"user"`
		Entitlements []store.UserEntitlement `json:"entitlements"`
		Tokens       []store.Token           `json:"tokens"`
	}
	f.mustJSON(body, &detail)
	if status != 200 || detail.User.Username != "alice2" || detail.Entitlements == nil || detail.Tokens == nil {
		t.Errorf("detail: %d %s", status, body)
	}

	if status, _ := f.req(f.client, "DELETE", "/api/v1/users/"+itoa(f.admin.ID), nil, nil); status != 400 {
		t.Errorf("delete self: %d, want 400", status)
	}
	if status, _ := f.req(f.client, "DELETE", path, nil, nil); status != 204 {
		t.Errorf("delete: %d", status)
	}
	if status, _ := f.req(f.client, "GET", path, nil, nil); status != 404 {
		t.Errorf("after delete: %d, want 404", status)
	}
	if status, _ := f.req(f.client, "GET", "/api/v1/users/abc", nil, nil); status != 400 {
		t.Errorf("bad id: %d, want 400", status)
	}
}

func TestEntitlementsAndTokens(t *testing.T) {
	f := newFixture(t)
	f.login()
	name, whl := demoWheel(t, "0.1.0")
	if status, body := f.uploadUI(map[string][]byte{name: whl}); status != 200 {
		t.Fatalf("upload: %d %s", status, body)
	}
	_, body := f.req(f.client, "POST", "/api/v1/users", map[string]string{"username": "alice"}, nil)
	var alice store.User
	f.mustJSON(body, &alice)
	base := "/api/v1/users/" + itoa(alice.ID)

	// Grant with a date, revoke, grant forever; bad inputs rejected.
	if status, body := f.req(f.client, "PUT", base+"/entitlements/JohnnyBT_Demo", map[string]string{"expires_at": "2099-12-31"}, nil); status != 204 {
		t.Fatalf("grant: %d %s", status, body)
	}
	if status, _ := f.req(f.client, "PUT", base+"/entitlements/johnnybt-demo", map[string]string{"expires_at": "next week"}, nil); status != 400 {
		t.Errorf("bad expiry: %d, want 400", status)
	}
	if status, _ := f.req(f.client, "PUT", base+"/entitlements/nope", map[string]string{}, nil); status != 404 {
		t.Errorf("unknown package: %d, want 404", status)
	}
	_, body = f.req(f.client, "GET", base, nil, nil)
	var detail struct {
		Entitlements []store.UserEntitlement `json:"entitlements"`
	}
	f.mustJSON(body, &detail)
	if len(detail.Entitlements) != 1 || detail.Entitlements[0].ExpiresAt == nil || !detail.Entitlements[0].Active || detail.Entitlements[0].LatestVersion != "0.1.0" {
		t.Errorf("entitlements: %s", body)
	}
	if status, _ := f.req(f.client, "DELETE", base+"/entitlements/johnnybt-demo", nil, nil); status != 204 {
		t.Errorf("revoke: %d", status)
	}
	if status, _ := f.req(f.client, "PUT", base+"/entitlements/johnnybt-demo", map[string]string{}, nil); status != 204 {
		t.Errorf("grant forever: %d", status)
	}

	// Tokens: customer gets read only; plaintext returned once; revocation works.
	if status, _ := f.req(f.client, "POST", base+"/tokens", map[string]string{"name": "x", "scope": "write"}, nil); status != 400 {
		t.Errorf("customer write token: %d, want 400", status)
	}
	status, body := f.req(f.client, "POST", base+"/tokens", map[string]string{"name": "laptop"}, nil)
	var created struct {
		Token    string      `json:"token"`
		Info     store.Token `json:"info"`
		IndexURL string      `json:"index_url"`
	}
	f.mustJSON(body, &created)
	if status != 201 || !auth.LooksLikeToken(created.Token) || created.Info.Scope != "read" || created.Info.Name != "laptop" || !strings.HasSuffix(created.IndexURL, "/simple/") {
		t.Fatalf("create token: %d %s", status, body)
	}
	if _, _, err := f.store.ActiveTokenByHash(context.Background(), auth.HashToken(created.Token)); err != nil {
		t.Error("token not usable:", err)
	}
	_, body = f.req(f.client, "GET", base, nil, nil)
	if strings.Contains(string(body), created.Token) {
		t.Error("plaintext token must not be retrievable later")
	}
	if status, _ := f.req(f.client, "DELETE", "/api/v1/tokens/"+itoa(created.Info.ID), nil, nil); status != 204 {
		t.Errorf("revoke token: %d", status)
	}
	if _, _, err := f.store.ActiveTokenByHash(context.Background(), auth.HashToken(created.Token)); err == nil {
		t.Error("revoked token still active")
	}

	// Admin can mint a write token for uploads.
	status, body = f.req(f.client, "POST", "/api/v1/users/"+itoa(f.admin.ID)+"/tokens", map[string]string{"name": "ci", "scope": "write"}, nil)
	f.mustJSON(body, &created)
	if status != 201 || created.Info.Scope != "write" {
		t.Errorf("admin write token: %d %s", status, body)
	}
}

// --- packages ---------------------------------------------------------------

func TestUploadAndPackageViews(t *testing.T) {
	f := newFixture(t)
	f.login()
	n1, w1 := demoWheel(t, "0.1.0")
	n2, w2 := demoWheel(t, "0.2.0")

	status, body := f.uploadUI(map[string][]byte{n1: w1, n2: w2, "junk.txt": []byte("x")})
	var results []struct {
		Filename, Package, Version, Error string
	}
	f.mustJSON(body, &results)
	if status != 200 || len(results) != 3 {
		t.Fatalf("upload: %d %s", status, body)
	}
	okCount, errCount := 0, 0
	for _, r := range results {
		if r.Error != "" {
			errCount++
			if r.Filename != "junk.txt" {
				t.Errorf("unexpected failure: %+v", r)
			}
		} else {
			okCount++
			if r.Package != "johnnybt-demo" {
				t.Errorf("unexpected result: %+v", r)
			}
		}
	}
	if okCount != 2 || errCount != 1 {
		t.Errorf("mixed upload: %d ok, %d failed", okCount, errCount)
	}
	if status, _ := f.uploadUI(map[string][]byte{"junk.txt": []byte("x")}); status != 400 {
		t.Errorf("all-failed upload: %d, want 400", status)
	}
	if status, _ := f.uploadUI(map[string][]byte{n1: w1}); status != 400 {
		t.Errorf("duplicate upload: %d, want 400", status)
	}

	status, body = f.req(f.client, "GET", "/api/v1/packages", nil, nil)
	var list []struct {
		store.PackageSummary
		DownloadCount int `json:"download_count"`
	}
	f.mustJSON(body, &list)
	if status != 200 || len(list) != 1 || list[0].LatestVersion != "0.2.0" || list[0].FileCount != 2 {
		t.Errorf("list: %d %s", status, body)
	}

	status, body = f.req(f.client, "GET", "/api/v1/packages/JohnnyBT.Demo", nil, nil)
	var detail struct {
		Package       store.Package `json:"package"`
		LatestVersion string        `json:"latest_version"`
		Releases      []struct {
			store.Release
			Files []store.File `json:"files"`
		} `json:"releases"`
		Users []store.PackageEntitlement `json:"users"`
	}
	f.mustJSON(body, &detail)
	if status != 200 || detail.LatestVersion != "0.2.0" || len(detail.Releases) != 2 || detail.Releases[0].Version != "0.2.0" || len(detail.Releases[0].Files) != 1 || detail.Users == nil {
		t.Errorf("detail: %d %s", status, body)
	}
	if strings.Contains(string(body), "blob_key") {
		t.Error("blob keys are internal and must not be exposed")
	}

	if status, _ := f.req(f.client, "POST", "/api/v1/packages/johnnybt-demo/releases/0.2.0/yank", map[string]any{"yanked": true, "reason": "oops"}, nil); status != 204 {
		t.Errorf("yank: %d", status)
	}
	_, body = f.req(f.client, "GET", "/api/v1/packages/johnnybt-demo", nil, nil)
	f.mustJSON(body, &detail)
	if detail.LatestVersion != "0.1.0" || !detail.Releases[0].Yanked || detail.Releases[0].YankedReason != "oops" {
		t.Errorf("after yank: %s", body)
	}
	if status, _ := f.req(f.client, "DELETE", "/api/v1/packages/johnnybt-demo/releases/0.2.0", nil, nil); status != 204 {
		t.Errorf("delete release: %d", status)
	}
	if status, _ := f.req(f.client, "DELETE", "/api/v1/packages/johnnybt-demo/releases/9.9.9", nil, nil); status != 404 {
		t.Errorf("delete missing release: %d, want 404", status)
	}
	if status, _ := f.req(f.client, "DELETE", "/api/v1/packages/johnnybt-demo", nil, nil); status != 204 {
		t.Errorf("delete package: %d", status)
	}
	if status, _ := f.req(f.client, "GET", "/api/v1/packages/johnnybt-demo", nil, nil); status != 404 {
		t.Errorf("after delete: %d, want 404", status)
	}
	// Blobs are gone too.
	if entries, _ := filepath.Glob(filepath.Join(f.dir, "blobs", "*", "*")); len(entries) != 0 {
		t.Errorf("blobs left behind: %v", entries)
	}
}

// --- self check -------------------------------------------------------------

func TestTokenSelfCheck(t *testing.T) {
	f := newFixture(t)
	f.login()
	name, whl := demoWheel(t, "0.1.0")
	f.uploadUI(map[string][]byte{name: whl})
	_, body := f.req(f.client, "POST", "/api/v1/users", map[string]string{"username": "alice", "note": "vip"}, nil)
	var alice store.User
	f.mustJSON(body, &alice)
	f.req(f.client, "PUT", "/api/v1/users/"+itoa(alice.ID)+"/entitlements/johnnybt-demo", map[string]string{"expires_at": "2099-01-01"}, nil)
	_, body = f.req(f.client, "POST", "/api/v1/users/"+itoa(alice.ID)+"/tokens", map[string]string{"name": "laptop"}, nil)
	var created struct {
		Token string `json:"token"`
	}
	f.mustJSON(body, &created)

	// Anonymous, no cookie, no Sec-Fetch-Site: works from any client.
	req, _ := http.NewRequest("POST", f.srv.URL+"/api/v1/me", strings.NewReader(`{"token":"`+created.Token+`"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := f.anon.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	var out struct {
		Username string                  `json:"username"`
		Note     string                  `json:"note"`
		IsAdmin  bool                    `json:"is_admin"`
		Packages []store.UserEntitlement `json:"packages"`
		IndexURL string                  `json:"index_url"`
	}
	if err := json.Unmarshal(b, &out); err != nil || res.StatusCode != 200 {
		t.Fatalf("self check: %d %s", res.StatusCode, b)
	}
	if out.Username != "alice" || out.Note != "vip" || out.IsAdmin || len(out.Packages) != 1 || out.Packages[0].NormalizedName != "johnnybt-demo" || !strings.HasSuffix(out.IndexURL, "/simple/") {
		t.Errorf("self check payload: %s", b)
	}
	for _, bad := range []string{"", "garbage", "jbt_" + strings.Repeat("z", 40)} {
		if status, _ := f.req(f.anon, "POST", "/api/v1/me", map[string]string{"token": bad}, nil); status != 401 {
			t.Errorf("token %q: %d, want 401", bad, status)
		}
	}
}

func TestParseExpiry(t *testing.T) {
	if got, err := parseExpiry(""); err != nil || got != nil {
		t.Errorf("empty: %v %v", got, err)
	}
	got, err := parseExpiry("2026-12-31")
	if err != nil || got == nil {
		t.Fatalf("date: %v %v", got, err)
	}
	// End of that day in server-local time == start of the next day.
	want := time.Date(2027, 1, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("date expiry = %v, want %v", got, want)
	}
	if got, err := parseExpiry("2026-12-31T10:00:00Z"); err != nil || !got.Equal(time.Date(2026, 12, 31, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("rfc3339: %v %v", got, err)
	}
	if _, err := parseExpiry("tomorrow"); err == nil {
		t.Error("garbage accepted")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
