package pypi

import (
	"errors"
	"net/http"

	"github.com/quant-on-quest/johnnybt-pypi/internal/auth"
)

// legacyUpload implements the multipart form that twine and `uv publish`
// POST to (the same shape as https://upload.pypi.org/legacy/).
func (h *Handler) legacyUpload(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if !p.CanWrite() {
		http.Error(w, "403 Forbidden: uploading requires an admin token with write scope", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.Ingester.MaxSize+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "400 Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()
	if action := r.FormValue(":action"); action != "" && action != "file_upload" {
		http.Error(w, "400 Bad Request: unsupported :action "+action, http.StatusBadRequest)
		return
	}
	part, hdr, err := r.FormFile("content")
	if err != nil {
		http.Error(w, "400 Bad Request: missing content field", http.StatusBadRequest)
		return
	}
	defer part.Close()

	res, err := h.Ingester.Ingest(r.Context(), hdr.Filename, part, r.FormValue("sha256_digest"), p.User.ID)
	if err != nil {
		h.writeIngestError(w, err)
		return
	}
	h.Log.Info("uploaded", "package", res.Package.Name, "version", res.Release.Version, "file", res.File.Filename, "by", p.User.Username)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK\n"))
}

// writeIngestError maps ingest failures to the status codes twine expects.
func (h *Handler) writeIngestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrFileExists):
		http.Error(w, "400 Bad Request: "+err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrBadFile), errors.Is(err, ErrDigestMismatch):
		http.Error(w, "400 Bad Request: "+err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrTooLarge):
		http.Error(w, "413 Payload Too Large: "+err.Error(), http.StatusRequestEntityTooLarge)
	default:
		h.Log.Error("ingest failed", "err", err)
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
	}
}
