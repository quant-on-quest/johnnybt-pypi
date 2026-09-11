package pypi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/quant-on-quest/johnnybt-pypi/internal/auth"
	"github.com/quant-on-quest/johnnybt-pypi/internal/blob"
	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
)

const (
	repositoryVersion = "1.1"
	jsonMediaType     = "application/vnd.pypi.simple.v1+json"
	htmlMediaType     = "application/vnd.pypi.simple.v1+html"
	signedURLTTL      = 10 * time.Minute
)

type Handler struct {
	Store    *store.Store
	Blobs    blob.Store
	Ingester *Ingester
	// BaseURL overrides the scheme+host used in generated links.
	BaseURL string
	Log     *slog.Logger
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /simple/{$}", h.withAuth(h.simpleIndex))
	mux.HandleFunc("GET /simple/{name}/{$}", h.withAuth(h.simpleProject))
	mux.HandleFunc("GET /simple/{name}", h.withAuth(func(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
		http.Redirect(w, r, "/simple/"+r.PathValue("name")+"/", http.StatusMovedPermanently)
	}))
	mux.HandleFunc("GET /files/{sha256}/{filename}", h.withAuth(h.file))
	mux.HandleFunc("POST /legacy/{$}", h.withAuth(h.legacyUpload))
	mux.HandleFunc("POST /legacy", h.withAuth(h.legacyUpload))
}

type authedHandler func(w http.ResponseWriter, r *http.Request, p *auth.Principal)

func (h *Handler) withAuth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.Authenticate(r.Context(), h.Store, r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="johnnybt-pypi"`)
			http.Error(w, "401 Unauthorized: a valid API token is required", http.StatusUnauthorized)
			return
		}
		next(w, r, p)
	}
}

// canAccess reports whether the principal may see the package.
func (h *Handler) canAccess(ctx context.Context, p *auth.Principal, pkg *store.Package) bool {
	if p.User.IsAdmin {
		return true
	}
	ok, err := h.Store.HasAccess(ctx, p.User.ID, pkg.ID)
	return err == nil && ok
}

// baseURL is the externally visible origin for links in responses.
func (h *Handler) baseURL(r *http.Request) string {
	if h.BaseURL != "" {
		return h.BaseURL
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host
}

// wantsJSON implements the PEP 691 content negotiation.
func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return false
	}
	bestJSON, bestHTML := -1.0, -1.0
	for _, part := range strings.Split(accept, ",") {
		mt, q := parseAcceptPart(part)
		switch mt {
		case jsonMediaType:
			bestJSON = max(bestJSON, q)
		case htmlMediaType, "text/html":
			bestHTML = max(bestHTML, q)
		case "*/*", "application/*":
			bestHTML = max(bestHTML, q*0.5)
		}
	}
	return bestJSON > 0 && bestJSON >= bestHTML
}

func parseAcceptPart(part string) (string, float64) {
	fields := strings.Split(strings.TrimSpace(part), ";")
	mt := strings.ToLower(strings.TrimSpace(fields[0]))
	q := 1.0
	for _, f := range fields[1:] {
		f = strings.TrimSpace(f)
		if strings.HasPrefix(f, "q=") {
			fmt.Sscanf(f[2:], "%f", &q)
		}
	}
	return mt, q
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *Handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	h.Log.Error("pypi handler error", "path", r.URL.Path, "err", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
