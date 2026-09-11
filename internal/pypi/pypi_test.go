package pypi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quant-on-quest/johnnybt-pypi/internal/auth"
	"github.com/quant-on-quest/johnnybt-pypi/internal/blob"
	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
	"github.com/quant-on-quest/johnnybt-pypi/internal/testutil"
)

type fixture struct {
	t        *testing.T
	srv      *httptest.Server
	store    *store.Store
	blobs    *blob.Local
	admin    *store.User
	adminTok string // write scope
	adminRd  string // read scope
	alice    *store.User
	aliceTok string
	bob      *store.User
	bobTok   string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	blobs, err := blob.NewLocal(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	ing := &Ingester{Store: st, Blobs: blobs, TmpDir: filepath.Join(dir, "tmp"), MaxSize: 1 << 20}
	h := &Handler{Store: st, Blobs: blobs, Ingester: ing, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	f := &fixture{t: t, srv: srv, store: st, blobs: blobs}
	f.admin, _ = st.CreateUser(ctx, "admin", "", true, "h")
	f.alice, _ = st.CreateUser(ctx, "alice", "", false, "")
	f.bob, _ = st.CreateUser(ctx, "bob", "", false, "")
	f.adminTok = f.mint(f.admin.ID, auth.ScopeWrite)
	f.adminRd = f.mint(f.admin.ID, auth.ScopeRead)
	f.aliceTok = f.mint(f.alice.ID, auth.ScopeRead)
	f.bobTok = f.mint(f.bob.ID, auth.ScopeRead)
	return f
}

func (f *fixture) mint(userID int64, scope string) string {
	plain := auth.GenerateToken()
	if _, err := f.store.CreateToken(context.Background(), userID, "t", auth.DisplayPrefix(plain), auth.HashToken(plain), scope); err != nil {
		f.t.Fatal(err)
	}
	return plain
}

func (f *fixture) do(method, path, token string, body io.Reader, hdr map[string]string) *http.Response {
	f.t.Helper()
	req, _ := http.NewRequest(method, f.srv.URL+path, body)
	if token != "" {
		req.SetBasicAuth("__token__", token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func (f *fixture) get(path, token, accept string) (int, string, http.Header) {
	f.t.Helper()
	hdr := map[string]string{}
	if accept != "" {
		hdr["Accept"] = accept
	}
	res := f.do("GET", path, token, nil, hdr)
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b), res.Header
}

// uploadLegacy posts a distribution the way twine / uv publish do.
func (f *fixture) uploadLegacy(token, filename string, content []byte, extra map[string]string) (int, string) {
	f.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fields := map[string]string{":action": "file_upload", "protocol_version": "1"}
	for k, v := range extra {
		fields[k] = v
	}
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("content", filename)
	fw.Write(content)
	mw.Close()
	res := f.do("POST", "/legacy/", token, &buf, map[string]string{"Content-Type": mw.FormDataContentType()})
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func demoWheel(t *testing.T, version string) (string, []byte) {
	md := testutil.Metadata("johnnybt-demo", version, map[string]string{
		"Summary":                  "Demo " + version,
		"Requires-Python":          ">=3.10",
		"Description-Content-Type": "text/markdown",
	}, "# demo "+version)
	return "johnnybt_demo-" + version + "-py3-none-any.whl", testutil.WheelBytes(t, "johnnybt_demo", version, md)
}

// --- auth gate --------------------------------------------------------------

func TestSimpleRequiresToken(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{"/simple/", "/simple/johnnybt-demo/", "/files/abc/x.whl"} {
		status, _, hdr := f.get(path, "", "")
		if status != http.StatusUnauthorized {
			t.Errorf("%s without token: %d, want 401", path, status)
		}
		if !strings.HasPrefix(hdr.Get("WWW-Authenticate"), "Basic") {
			t.Errorf("%s: missing WWW-Authenticate challenge", path)
		}
	}
	if status, _, _ := f.get("/simple/", "jbt_"+strings.Repeat("x", 40), ""); status != http.StatusUnauthorized {
		t.Errorf("unknown token: %d, want 401", status)
	}
	if status, _, _ := f.get("/simple/", f.aliceTok, ""); status != http.StatusOK {
		t.Errorf("valid token: %d, want 200", status)
	}
}

func TestTokenUsageIsRecorded(t *testing.T) {
	f := newFixture(t)
	f.get("/simple/", f.aliceTok, "")
	toks, _ := f.store.ListTokens(context.Background(), f.alice.ID)
	if toks[0].LastUsedAt == nil {
		t.Error("last_used_at should be set after a request")
	}
}

// --- upload -----------------------------------------------------------------

func TestLegacyUploadRequiresWriteScope(t *testing.T) {
	f := newFixture(t)
	name, whl := demoWheel(t, "0.1.0")
	if status, _ := f.uploadLegacy(f.aliceTok, name, whl, nil); status != http.StatusForbidden {
		t.Errorf("customer upload: %d, want 403", status)
	}
	if status, _ := f.uploadLegacy(f.adminRd, name, whl, nil); status != http.StatusForbidden {
		t.Errorf("admin read-scope upload: %d, want 403", status)
	}
	if status, body := f.uploadLegacy(f.adminTok, name, whl, nil); status != http.StatusOK {
		t.Errorf("admin write-scope upload: %d %s, want 200", status, body)
	}
}

func TestLegacyUploadIndexesWheel(t *testing.T) {
	f := newFixture(t)
	name, whl := demoWheel(t, "0.1.0")
	status, body := f.uploadLegacy(f.adminTok, name, whl, map[string]string{"name": "johnnybt-demo", "version": "0.1.0"})
	if status != http.StatusOK {
		t.Fatalf("upload: %d %s", status, body)
	}
	ctx := context.Background()
	pkg, err := f.store.GetPackage(ctx, "johnnybt-demo")
	if err != nil {
		t.Fatal("package not indexed:", err)
	}
	if pkg.Summary != "Demo 0.1.0" || pkg.Description != "# demo 0.1.0" || pkg.DescriptionContentType != "text/markdown" {
		t.Errorf("metadata not captured: %+v", pkg)
	}
	file, rel, _, err := f.store.FileByName(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "0.1.0" || file.RequiresPython != ">=3.10" || file.MetadataSHA256 == "" || file.Size != int64(len(whl)) {
		t.Errorf("file record wrong: %+v", file)
	}
	if _, err := f.blobs.Open(ctx, file.BlobKey); err != nil {
		t.Error("blob not stored:", err)
	}
	if _, err := f.blobs.Open(ctx, file.BlobKey+".metadata"); err != nil {
		t.Error("PEP 658 metadata blob not stored:", err)
	}
}

func TestLegacyUploadRejectsDuplicateWithTwineFriendlyMessage(t *testing.T) {
	f := newFixture(t)
	name, whl := demoWheel(t, "0.1.0")
	f.uploadLegacy(f.adminTok, name, whl, nil)
	status, body := f.uploadLegacy(f.adminTok, name, whl, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "File already exists") {
		t.Errorf("duplicate upload: %d %q, want 400 containing 'File already exists'", status, body)
	}
}

func TestLegacyUploadValidation(t *testing.T) {
	f := newFixture(t)
	name, whl := demoWheel(t, "0.1.0")

	status, body := f.uploadLegacy(f.adminTok, name, whl, map[string]string{"sha256_digest": strings.Repeat("0", 64)})
	if status != http.StatusBadRequest || !strings.Contains(body, "digest") {
		t.Errorf("digest mismatch: %d %q", status, body)
	}
	if status, _ := f.uploadLegacy(f.adminTok, "notes.txt", []byte("hi"), nil); status != http.StatusBadRequest {
		t.Errorf("unknown extension: %d", status)
	}
	if status, _ := f.uploadLegacy(f.adminTok, "johnnybt_demo-0.1.0-py3-none-any.whl", []byte("not a zip"), nil); status != http.StatusBadRequest {
		t.Errorf("corrupt wheel: %d", status)
	}
	// Filename says 0.2.0 but METADATA says 0.1.0.
	if status, body := f.uploadLegacy(f.adminTok, "johnnybt_demo-0.2.0-py3-none-any.whl", whl, nil); status != http.StatusBadRequest || !strings.Contains(body, "version") {
		t.Errorf("version mismatch: %d %q", status, body)
	}
	// Filename project differs from METADATA Name.
	if status, _ := f.uploadLegacy(f.adminTok, "other_pkg-0.1.0-py3-none-any.whl", whl, nil); status != http.StatusBadRequest {
		t.Errorf("name mismatch: %d", status)
	}
	if status, _ := f.uploadLegacy(f.adminTok, name, whl, map[string]string{":action": "submit"}); status != http.StatusBadRequest {
		t.Errorf("unsupported action: %d", status)
	}
	if n, _ := f.store.ListPackages(context.Background()); len(n) != 0 {
		t.Errorf("rejected uploads must not create packages, got %+v", n)
	}
}

func TestUploadTooLarge(t *testing.T) {
	f := newFixture(t)
	big := bytes.Repeat([]byte("x"), 2<<20) // fixture MaxSize is 1 MiB
	status, _ := f.uploadLegacy(f.adminTok, "johnnybt_demo-0.1.0-py3-none-any.whl", big, nil)
	if status != http.StatusRequestEntityTooLarge && status != http.StatusBadRequest {
		t.Errorf("oversize upload: %d, want 413 or 400", status)
	}
}

func TestSdistUploadHasNoMetadataFile(t *testing.T) {
	f := newFixture(t)
	md := testutil.Metadata("johnnybt-demo", "0.1.0", nil, "readme")
	status, body := f.uploadLegacy(f.adminTok, "johnnybt_demo-0.1.0.tar.gz", testutil.SdistBytes(t, "johnnybt_demo", "0.1.0", md), nil)
	if status != http.StatusOK {
		t.Fatalf("sdist upload: %d %s", status, body)
	}
	file, _, _, _ := f.store.FileByName(context.Background(), "johnnybt_demo-0.1.0.tar.gz")
	if file.MetadataSHA256 != "" {
		t.Error("sdists must not advertise PEP 658 metadata")
	}
}

func TestNewestUploadRefreshesPackageMetadata(t *testing.T) {
	f := newFixture(t)
	n2, w2 := demoWheel(t, "0.2.0")
	n1, w1 := demoWheel(t, "0.1.0")
	f.uploadLegacy(f.adminTok, n2, w2, nil)
	f.uploadLegacy(f.adminTok, n1, w1, nil) // older version arrives later
	pkg, _ := f.store.GetPackage(context.Background(), "johnnybt-demo")
	if pkg.Summary != "Demo 0.2.0" {
		t.Errorf("older upload must not overwrite summary, got %q", pkg.Summary)
	}
}

// --- simple index -----------------------------------------------------------

func seed(t *testing.T, f *fixture) {
	t.Helper()
	for _, v := range []string{"0.1.0", "0.2.0"} {
		n, w := demoWheel(t, v)
		if status, body := f.uploadLegacy(f.adminTok, n, w, nil); status != 200 {
			t.Fatalf("seed upload %s: %d %s", v, status, body)
		}
	}
	md := testutil.Metadata("other-pkg", "1.0", nil, "")
	if status, body := f.uploadLegacy(f.adminTok, "other_pkg-1.0.tar.gz", testutil.SdistBytes(t, "other_pkg", "1.0", md), nil); status != 200 {
		t.Fatalf("seed other: %d %s", status, body)
	}
	if err := f.store.GrantEntitlement(context.Background(), f.alice.ID, mustPkg(t, f, "johnnybt-demo").ID, nil, f.admin.ID); err != nil {
		t.Fatal(err)
	}
}

func mustPkg(t *testing.T, f *fixture, name string) *store.Package {
	t.Helper()
	p, err := f.store.GetPackage(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSimpleIndexListsOnlyEntitledPackages(t *testing.T) {
	f := newFixture(t)
	seed(t, f)

	_, body, _ := f.get("/simple/", f.adminTok, "")
	if !strings.Contains(body, `href="/simple/johnnybt-demo/"`) || !strings.Contains(body, `href="/simple/other-pkg/"`) {
		t.Errorf("admin should see everything:\n%s", body)
	}
	_, body, _ = f.get("/simple/", f.aliceTok, "")
	if !strings.Contains(body, "johnnybt-demo") || strings.Contains(body, "other-pkg") {
		t.Errorf("alice should only see johnnybt-demo:\n%s", body)
	}
	_, body, _ = f.get("/simple/", f.bobTok, "")
	if strings.Contains(body, "johnnybt-demo") || strings.Contains(body, "other-pkg") {
		t.Errorf("bob has no entitlements and should see an empty index:\n%s", body)
	}
	if !strings.Contains(body, `name="pypi:repository-version"`) {
		t.Error("missing PEP 629 repository-version meta")
	}
}

func TestSimpleIndexJSON(t *testing.T) {
	f := newFixture(t)
	seed(t, f)
	status, body, hdr := f.get("/simple/", f.aliceTok, "application/vnd.pypi.simple.v1+json")
	if status != 200 || hdr.Get("Content-Type") != jsonMediaType {
		t.Fatalf("status %d content-type %q", status, hdr.Get("Content-Type"))
	}
	var doc struct {
		Meta     map[string]string   `json:"meta"`
		Projects []map[string]string `json:"projects"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err, body)
	}
	if doc.Meta["api-version"] != "1.1" || len(doc.Projects) != 1 || doc.Projects[0]["name"] != "johnnybt-demo" {
		t.Errorf("unexpected JSON index: %s", body)
	}
	if hdr.Get("Vary") != "Accept" {
		t.Error("responses must vary on Accept")
	}
}

func TestContentNegotiation(t *testing.T) {
	cases := []struct {
		accept string
		json   bool
	}{
		{"", false},
		{"text/html", false},
		{"*/*", false},
		{"application/vnd.pypi.simple.v1+json", true},
		// pip's actual header prefers JSON over HTML.
		{"application/vnd.pypi.simple.v1+json, application/vnd.pypi.simple.v1+html; q=0.1, text/html; q=0.01", true},
		{"application/vnd.pypi.simple.v1+html, application/vnd.pypi.simple.v1+json; q=0.5", false},
		{"application/vnd.pypi.simple.v1+json;q=0", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/simple/", nil)
		if c.accept != "" {
			r.Header.Set("Accept", c.accept)
		}
		if got := wantsJSON(r); got != c.json {
			t.Errorf("Accept %q → json=%v, want %v", c.accept, got, c.json)
		}
	}
}

func TestSimpleProjectPage(t *testing.T) {
	f := newFixture(t)
	seed(t, f)

	status, body, _ := f.get("/simple/johnnybt-demo/", f.aliceTok, "")
	if status != 200 {
		t.Fatalf("status %d: %s", status, body)
	}
	for _, want := range []string{
		`<a href="` + f.srv.URL + `/files/`,
		`johnnybt_demo-0.1.0-py3-none-any.whl#sha256=`,
		`johnnybt_demo-0.2.0-py3-none-any.whl#sha256=`,
		`data-requires-python="&gt;=3.10"`,
		`data-core-metadata="sha256=`,
		`data-dist-info-metadata="sha256=`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("project page missing %q:\n%s", want, body)
		}
	}
	// Newest version first.
	if strings.Index(body, "0.2.0-py3") > strings.Index(body, "0.1.0-py3") {
		t.Error("files should be listed newest version first")
	}
}

func TestSimpleProjectAccessControl(t *testing.T) {
	f := newFixture(t)
	seed(t, f)
	if status, _, _ := f.get("/simple/other-pkg/", f.aliceTok, ""); status != http.StatusForbidden {
		t.Errorf("alice on other-pkg: %d, want 403", status)
	}
	if status, _, _ := f.get("/simple/johnnybt-demo/", f.bobTok, ""); status != http.StatusForbidden {
		t.Errorf("bob on johnnybt-demo: %d, want 403", status)
	}
	if status, _, _ := f.get("/simple/other-pkg/", f.adminTok, ""); status != 200 {
		t.Errorf("admin on other-pkg: %d, want 200", status)
	}
	if status, _, _ := f.get("/simple/does-not-exist/", f.adminTok, ""); status != http.StatusNotFound {
		t.Errorf("unknown package: %d, want 404", status)
	}
	// Expired entitlement behaves like none.
	past := time.Now().Add(-time.Minute)
	f.store.GrantEntitlement(context.Background(), f.alice.ID, mustPkg(t, f, "johnnybt-demo").ID, &past, f.admin.ID)
	if status, _, _ := f.get("/simple/johnnybt-demo/", f.aliceTok, ""); status != http.StatusForbidden {
		t.Errorf("expired entitlement: %d, want 403", status)
	}
}

func TestSimpleProjectRedirectsToNormalizedName(t *testing.T) {
	f := newFixture(t)
	seed(t, f)
	res := f.do("GET", "/simple/JohnnyBT_Demo/", f.aliceTok, nil, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != "/simple/johnnybt-demo/" {
		t.Errorf("got %d → %q", res.StatusCode, res.Header.Get("Location"))
	}
	res = f.do("GET", "/simple/johnnybt-demo", f.aliceTok, nil, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != "/simple/johnnybt-demo/" {
		t.Errorf("missing slash: got %d → %q", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestSimpleProjectJSON(t *testing.T) {
	f := newFixture(t)
	seed(t, f)
	f.store.SetYanked(context.Background(), mustRelease(t, f, "johnnybt-demo", "0.1.0").ID, true, "bad build")

	status, body, _ := f.get("/simple/johnnybt-demo/", f.aliceTok, jsonMediaType)
	if status != 200 {
		t.Fatalf("status %d: %s", status, body)
	}
	var doc struct {
		Name     string   `json:"name"`
		Versions []string `json:"versions"`
		Files    []struct {
			Filename       string            `json:"filename"`
			URL            string            `json:"url"`
			Hashes         map[string]string `json:"hashes"`
			RequiresPython string            `json:"requires-python"`
			CoreMetadata   map[string]string `json:"core-metadata"`
			Yanked         any               `json:"yanked"`
			Size           int64             `json:"size"`
			UploadTime     string            `json:"upload-time"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err, body)
	}
	if doc.Name != "johnnybt-demo" || len(doc.Versions) != 2 || doc.Versions[0] != "0.2.0" {
		t.Errorf("name/versions wrong: %+v", doc)
	}
	if len(doc.Files) != 2 {
		t.Fatalf("files: %+v", doc.Files)
	}
	newest := doc.Files[0]
	if !strings.HasPrefix(newest.URL, f.srv.URL+"/files/") || len(newest.Hashes["sha256"]) != 64 || newest.RequiresPython != ">=3.10" || newest.CoreMetadata["sha256"] == "" || newest.Size == 0 || newest.UploadTime == "" {
		t.Errorf("newest file entry incomplete: %+v", newest)
	}
	if newest.Yanked != false {
		t.Errorf("0.2.0 should not be yanked: %v", newest.Yanked)
	}
	if doc.Files[1].Yanked != "bad build" {
		t.Errorf("yanked reason should be surfaced, got %v", doc.Files[1].Yanked)
	}
}

func mustRelease(t *testing.T, f *fixture, pkg, version string) *store.Release {
	t.Helper()
	r, err := f.store.GetRelease(context.Background(), mustPkg(t, f, pkg).ID, version)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// --- files ------------------------------------------------------------------

func TestFileDownload(t *testing.T) {
	f := newFixture(t)
	seed(t, f)
	name, whl := demoWheel(t, "0.2.0")
	file, _, _, _ := f.store.FileByName(context.Background(), name)
	path := "/files/" + file.SHA256 + "/" + name

	status, body, hdr := f.get(path, f.aliceTok, "")
	if status != 200 || body != string(whl) {
		t.Fatalf("download: %d, %d bytes (want %d)", status, len(body), len(whl))
	}
	if hdr.Get("Content-Type") != "application/octet-stream" || hdr.Get("ETag") != `"`+file.SHA256+`"` {
		t.Errorf("headers: %v", hdr)
	}

	status, meta, hdr := f.get(path+".metadata", f.aliceTok, "")
	if status != 200 || !strings.Contains(meta, "Name: johnnybt-demo") || !strings.HasPrefix(hdr.Get("Content-Type"), "text/plain") {
		t.Errorf("metadata download: %d %q", status, meta)
	}

	if status, _, _ := f.get(path, f.bobTok, ""); status != http.StatusForbidden {
		t.Errorf("bob download: %d, want 403", status)
	}
	if status, _, _ := f.get("/files/"+strings.Repeat("0", 64)+"/"+name, f.aliceTok, ""); status != http.StatusNotFound {
		t.Errorf("wrong sha: %d, want 404", status)
	}
	if status, _, _ := f.get("/files/"+file.SHA256+"/missing.whl", f.aliceTok, ""); status != http.StatusNotFound {
		t.Errorf("unknown filename: %d, want 404", status)
	}

	// Range requests work (uv uses them for lazy wheel reads).
	res := f.do("GET", path, f.aliceTok, nil, map[string]string{"Range": "bytes=0-3"})
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusPartialContent || string(b) != "PK\x03\x04" {
		t.Errorf("range request: %d %q", res.StatusCode, b)
	}
}

func TestDownloadsAreLogged(t *testing.T) {
	f := newFixture(t)
	seed(t, f)
	name, _ := demoWheel(t, "0.2.0")
	file, _, _, _ := f.store.FileByName(context.Background(), name)
	path := "/files/" + file.SHA256 + "/" + name

	f.get(path, f.aliceTok, "")
	f.get(path+".metadata", f.aliceTok, "") // metadata fetches are not downloads
	f.get(path, f.bobTok, "")               // denied → not logged

	rows, _ := f.store.ListDownloads(context.Background(), 10)
	if len(rows) != 1 || rows[0].Username != "alice" || rows[0].Filename != name || rows[0].PackageName != "johnnybt-demo" {
		t.Errorf("download log = %+v", rows)
	}
}

func TestBaseURLOverride(t *testing.T) {
	f := newFixture(t)
	seed(t, f)
	mux := http.NewServeMux()
	(&Handler{Store: f.store, Blobs: f.blobs, Ingester: nil, BaseURL: "https://pypi.example.com", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/simple/johnnybt-demo/", nil)
	req.SetBasicAuth("__token__", f.aliceTok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), `href="https://pypi.example.com/files/`) {
		t.Errorf("BaseURL not used in links:\n%s", b)
	}
}

func TestForwardedHeadersDeriveBaseURL(t *testing.T) {
	f := newFixture(t)
	seed(t, f)
	res := f.do("GET", "/simple/johnnybt-demo/", f.aliceTok, nil, map[string]string{
		"X-Forwarded-Proto": "https", "X-Forwarded-Host": "pypi.example.com",
	})
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), `href="https://pypi.example.com/files/`) {
		t.Errorf("forwarded headers ignored:\n%s", b)
	}
}

// Index pages differ per token, so a client cache shared between users (uv on
// one machine, a corporate proxy) must never replay them for someone else.
func TestSimpleResponsesAreNotCacheable(t *testing.T) {
	f := newFixture(t)
	seed(t, f)
	for _, c := range []struct{ path, accept string }{
		{"/simple/", ""}, {"/simple/", jsonMediaType},
		{"/simple/johnnybt-demo/", ""}, {"/simple/johnnybt-demo/", jsonMediaType},
	} {
		_, _, hdr := f.get(c.path, f.aliceTok, c.accept)
		if cc := hdr.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("%s (Accept %q): Cache-Control = %q, want no-store", c.path, c.accept, cc)
		}
	}
}

// redirectingBlobs stands in for an S3-style backend that hands out signed URLs.
type redirectingBlobs struct {
	blob.Store
	base string
}

func (r redirectingBlobs) SignedURL(_ context.Context, key string, ttl time.Duration) (string, error) {
	return r.base + "/" + key + "?X-Amz-Expires=" + fmt.Sprint(int(ttl.Seconds())), nil
}

func TestFileDownloadRedirectsToSignedURL(t *testing.T) {
	f := newFixture(t)
	seed(t, f)
	name, _ := demoWheel(t, "0.2.0")
	file, _, _, _ := f.store.FileByName(context.Background(), name)

	mux := http.NewServeMux()
	(&Handler{Store: f.store, Blobs: redirectingBlobs{Store: f.blobs, base: "https://bucket.oss-cn-hangzhou.aliyuncs.com"},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(path, token string) *http.Response {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.SetBasicAuth("__token__", token)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}
	path := "/files/" + file.SHA256 + "/" + name

	res := get(path, f.aliceTok)
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status %d, want 302", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://bucket.oss-cn-hangzhou.aliyuncs.com/"+file.SHA256+"/"+name) || !strings.Contains(loc, "X-Amz-Expires=600") {
		t.Errorf("Location = %q", loc)
	}
	if res := get(path+".metadata", f.aliceTok); res.StatusCode != http.StatusFound || !strings.HasSuffix(strings.SplitN(res.Header.Get("Location"), "?", 2)[0], ".metadata") {
		t.Errorf("metadata redirect: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	// Access control happens before the redirect, and downloads are still logged.
	if res := get(path, f.bobTok); res.StatusCode != http.StatusForbidden {
		t.Errorf("bob: %d, want 403", res.StatusCode)
	}
	rows, _ := f.store.ListDownloads(context.Background(), 10)
	if len(rows) != 1 || rows[0].Username != "alice" {
		t.Errorf("download log = %+v", rows)
	}
}
