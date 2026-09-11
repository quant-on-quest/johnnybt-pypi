package pypi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/quant-on-quest/johnnybt-pypi/internal/auth"
	"github.com/quant-on-quest/johnnybt-pypi/internal/blob"
	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
)

// file serves GET /files/{sha256}/{filename} and the PEP 658 companion
// {filename}.metadata.
func (h *Handler) file(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	sha := r.PathValue("sha256")
	filename := r.PathValue("filename")
	isMetadata := strings.HasSuffix(filename, ".metadata")
	distName := strings.TrimSuffix(filename, ".metadata")

	f, rel, pkg, err := h.Store.FileByName(r.Context(), distName)
	if isNotFound(err) || (err == nil && f.SHA256 != sha) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if !h.canAccess(r.Context(), p, pkg) {
		http.Error(w, "403 Forbidden: your token is not entitled to "+pkg.Name, http.StatusForbidden)
		return
	}
	if isMetadata && f.MetadataSHA256 == "" {
		http.NotFound(w, r)
		return
	}
	key := f.BlobKey
	etag := `"` + f.SHA256 + `"`
	if isMetadata {
		key += ".metadata"
		etag = `"` + f.MetadataSHA256 + `"`
	}

	if url, err := h.Blobs.SignedURL(r.Context(), key, signedURLTTL); err != nil {
		h.serverError(w, r, err)
		return
	} else if url != "" {
		if !isMetadata {
			h.logDownload(r, p, f, rel, pkg)
		}
		http.Redirect(w, r, url, http.StatusFound)
		return
	}

	obj, err := h.Blobs.Open(r.Context(), key)
	if errors.Is(err, blob.ErrNotFound) {
		h.Log.Error("blob missing for indexed file", "key", key)
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	defer obj.Close()
	if !isMetadata {
		h.logDownload(r, p, f, rel, pkg)
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if isMetadata {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	http.ServeContent(w, r, filename, obj.ModTime, obj)
}

func (h *Handler) logDownload(r *http.Request, p *auth.Principal, f *store.File, rel *store.Release, pkg *store.Package) {
	d := &store.Download{
		UserID:    &p.User.ID,
		TokenID:   &p.Token.ID,
		FileID:    &f.ID,
		PackageID: &pkg.ID,
		Filename:  f.Filename,
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
	}
	if err := h.Store.LogDownload(r.Context(), d); err != nil {
		h.Log.Warn("log download", "err", err)
	}
}
