// Package web embeds the built Vue SPA and serves it with history-mode
// fallback so deep links like /users/3 load index.html.
package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// injectRuntimeConfig puts window.__PYPI__ into <head> so the SPA learns the
// admin prefix without a request. json.Marshal escapes "<" and ">" as \u003c
// and \u003e, so a hostile value cannot close the script tag.
func injectRuntimeConfig(index []byte, opts Options) []byte {
	cfg, _ := json.Marshal(map[string]string{"adminPath": opts.AdminPath})
	script := []byte("<script>window.__PYPI__=" + string(cfg) + ";</script></head>")
	if i := bytes.Index(index, []byte("</head>")); i >= 0 {
		return append(append(append([]byte{}, index[:i]...), script...), index[i+len("</head>"):]...)
	}
	return index
}

const notBuiltPage = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>johnnybt-pypi</title></head>
<body style="font-family:system-ui;padding:2rem;max-width:40rem">
<h1>johnnybt-pypi</h1>
<p>管理界面还没有构建进这个二进制。运行 <code>make web</code>（或 <code>cd web &amp;&amp; pnpm build</code>）后重新 <code>go build</code>。</p>
<p>仓库接口 <code>/simple/</code> 和上传接口 <code>/legacy/</code> 不受影响，可以正常使用。</p>
</body></html>
`

// Options is the runtime configuration handed to the SPA.
type Options struct {
	// AdminPath is where the admin UI lives; the SPA builds its routes from it.
	AdminPath string
}

// Handler serves the SPA embedded at build time.
func Handler(opts Options) http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return HandlerFS(sub, opts)
}

// HandlerFS serves a Vite build from fsys. API and repository routes are
// registered on the mux with higher specificity, so this only sees UI paths.
func HandlerFS(fsys fs.FS, opts Options) http.Handler {
	index, err := fs.ReadFile(fsys, "index.html")
	built := err == nil
	if built {
		index = injectRuntimeConfig(index, opts)
	}
	files := http.FS(fsys)
	fileServer := http.FileServer(files)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := path.Clean("/" + r.URL.Path)
		if built && p != "/" && p != "/index.html" {
			if f, err := files.Open(p); err == nil {
				st, statErr := f.Stat()
				f.Close()
				if statErr == nil && !st.IsDir() {
					if strings.HasPrefix(p, "/assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					fileServer.ServeHTTP(w, r)
					return
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if !built {
			w.Write([]byte(notBuiltPage))
			return
		}
		w.Write(index)
	})
}
