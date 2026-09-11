package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func get(t *testing.T, h http.Handler, path string) (int, string, http.Header) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, string(b), rec.Header()
}

func TestServesBuiltAssetsAndFallsBackToIndex(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":         {Data: []byte("<html>spa</html>")},
		"assets/app-abc.js":  {Data: []byte("console.log(1)")},
		"assets/app-abc.css": {Data: []byte("body{}")},
	}
	h := HandlerFS(fsys, Options{})

	status, body, hdr := get(t, h, "/")
	if status != 200 || body != "<html>spa</html>" || hdr.Get("Cache-Control") != "no-cache" {
		t.Errorf("/: %d %q %v", status, body, hdr)
	}
	for _, p := range []string{"/users/3", "/packages/johnnybt-demo", "/login", "/assets/"} {
		if status, body, _ := get(t, h, p); status != 200 || body != "<html>spa</html>" {
			t.Errorf("%s should fall back to index.html, got %d %q", p, status, body)
		}
	}
	status, body, hdr = get(t, h, "/assets/app-abc.js")
	if status != 200 || body != "console.log(1)" || !strings.Contains(hdr.Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %d %q %v", status, body, hdr)
	}
	if !strings.Contains(hdr.Get("Content-Type"), "javascript") {
		t.Errorf("asset content type: %q", hdr.Get("Content-Type"))
	}
	if status, _, _ := get(t, h, "/assets/missing.js"); status != 200 {
		// Unknown asset paths still get the SPA; the browser reports the 404 via module load.
		t.Errorf("missing asset: %d", status)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /: %d", rec.Code)
	}
}

func TestExplainsWhenUIIsNotBuilt(t *testing.T) {
	h := HandlerFS(fstest.MapFS{".gitkeep": {Data: []byte("")}}, Options{})
	status, body, _ := get(t, h, "/")
	if status != 200 || !strings.Contains(body, "make web") {
		t.Errorf("unbuilt UI: %d %q", status, body)
	}
	if status, body, _ := get(t, h, "/users/3"); status != 200 || !strings.Contains(body, "make web") {
		t.Errorf("unbuilt UI deep link: %d %q", status, body)
	}
}

func TestEmbeddedHandlerWorks(t *testing.T) {
	// Whatever is embedded right now, the root must answer with HTML.
	status, body, _ := get(t, Handler(Options{AdminPath: "/admin"}), "/")
	if status != 200 || !strings.Contains(body, "<html") {
		t.Errorf("embedded /: %d %q", status, body)
	}
}

func TestInjectsRuntimeConfigIntoIndex(t *testing.T) {
	fsys := fstest.MapFS{"index.html": {Data: []byte("<html><head><title>x</title></head><body></body></html>")}}
	h := HandlerFS(fsys, Options{AdminPath: "/manage-7"})
	for _, p := range []string{"/", "/manage-7/users/3"} {
		status, body, _ := get(t, h, p)
		if status != 200 {
			t.Fatalf("%s: %d", p, status)
		}
		want := `<script>window.__PYPI__={"adminPath":"/manage-7"};</script></head>`
		if !strings.Contains(body, want) {
			t.Errorf("%s: runtime config not injected:\n%s", p, body)
		}
	}
	// A path that could break out of the script tag is escaped, never executed.
	h = HandlerFS(fsys, Options{AdminPath: "/</script><script>alert(1)"})
	_, body, _ := get(t, h, "/")
	if strings.Contains(body, "</script><script>alert(1)") {
		t.Errorf("admin path must be JSON/HTML escaped:\n%s", body)
	}
}
