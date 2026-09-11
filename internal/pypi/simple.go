package pypi

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"

	"github.com/quant-on-quest/johnnybt-pypi/internal/auth"
	"github.com/quant-on-quest/johnnybt-pypi/internal/pkgmeta"
	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
)

// simpleIndex serves GET /simple/ — only the projects the caller may install.
func (h *Handler) simpleIndex(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var (
		pkgs []store.PackageSummary
		err  error
	)
	if p.User.IsAdmin {
		pkgs, err = h.Store.ListPackages(r.Context())
	} else {
		pkgs, err = h.Store.ListPackagesForUser(r.Context(), p.User.ID)
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("Vary", "Accept")
	if wantsJSON(r) {
		projects := make([]map[string]string, 0, len(pkgs))
		for _, pk := range pkgs {
			projects = append(projects, map[string]string{"name": pk.Name})
		}
		writeSimpleJSON(w, map[string]any{
			"meta":     map[string]string{"api-version": repositoryVersion},
			"projects": projects,
		})
		return
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	b.WriteString(`<meta name="pypi:repository-version" content="` + repositoryVersion + "\">\n")
	b.WriteString("<title>Simple index</title>\n</head>\n<body>\n")
	for _, pk := range pkgs {
		b.WriteString(`<a href="/simple/` + pk.NormalizedName + `/">` + html.EscapeString(pk.Name) + "</a><br>\n")
	}
	b.WriteString("</body>\n</html>\n")
	writeSimpleHTML(w, b.String())
}

// simpleProject serves GET /simple/{name}/.
func (h *Handler) simpleProject(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	name := r.PathValue("name")
	normalized := pkgmeta.NormalizeName(name)
	if name != normalized {
		http.Redirect(w, r, "/simple/"+normalized+"/", http.StatusMovedPermanently)
		return
	}
	pkg, err := h.Store.GetPackage(r.Context(), normalized)
	if isNotFound(err) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if !h.canAccess(r.Context(), p, pkg) {
		http.Error(w, "403 Forbidden: your token is not entitled to "+pkg.Name+" (or the entitlement has expired)", http.StatusForbidden)
		return
	}
	files, err := h.Store.ListPackageFiles(r.Context(), pkg.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	base := h.baseURL(r)
	w.Header().Set("Vary", "Accept")

	if wantsJSON(r) {
		versions := []string{}
		seen := map[string]bool{}
		out := make([]map[string]any, 0, len(files))
		for _, f := range files {
			if !seen[f.Version] {
				seen[f.Version] = true
				versions = append(versions, f.Version)
			}
			entry := map[string]any{
				"filename":    f.Filename,
				"url":         fileURL(base, &f.File),
				"hashes":      map[string]string{"sha256": f.SHA256},
				"size":        f.Size,
				"upload-time": f.UploadedAt.UTC().Format("2006-01-02T15:04:05.000000Z"),
			}
			if f.RequiresPython != "" {
				entry["requires-python"] = f.RequiresPython
			}
			if f.MetadataSHA256 != "" {
				meta := map[string]string{"sha256": f.MetadataSHA256}
				entry["core-metadata"] = meta
				entry["dist-info-metadata"] = meta
			}
			if f.Yanked {
				if f.YankedReason != "" {
					entry["yanked"] = f.YankedReason
				} else {
					entry["yanked"] = true
				}
			} else {
				entry["yanked"] = false
			}
			out = append(out, entry)
		}
		writeSimpleJSON(w, map[string]any{
			"meta":     map[string]string{"api-version": repositoryVersion},
			"name":     normalized,
			"versions": versions,
			"files":    out,
		})
		return
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	b.WriteString(`<meta name="pypi:repository-version" content="` + repositoryVersion + "\">\n")
	b.WriteString("<title>Links for " + html.EscapeString(pkg.Name) + "</title>\n</head>\n<body>\n")
	b.WriteString("<h1>Links for " + html.EscapeString(pkg.Name) + "</h1>\n")
	for _, f := range files {
		b.WriteString(`<a href="` + fileURL(base, &f.File) + "#sha256=" + f.SHA256 + `"`)
		if f.RequiresPython != "" {
			b.WriteString(` data-requires-python="` + html.EscapeString(f.RequiresPython) + `"`)
		}
		if f.MetadataSHA256 != "" {
			b.WriteString(` data-core-metadata="sha256=` + f.MetadataSHA256 + `"`)
			b.WriteString(` data-dist-info-metadata="sha256=` + f.MetadataSHA256 + `"`)
		}
		if f.Yanked {
			b.WriteString(` data-yanked="` + html.EscapeString(f.YankedReason) + `"`)
		}
		b.WriteString(">" + html.EscapeString(f.Filename) + "</a><br>\n")
	}
	b.WriteString("</body>\n</html>\n")
	writeSimpleHTML(w, b.String())
}

// fileURL builds the absolute download link for a stored file.
func fileURL(base string, f *store.File) string {
	return base + "/files/" + f.SHA256 + "/" + f.Filename
}

// Index pages are tailored to the caller's entitlements, so a cache shared
// between users (uv's on-disk cache, a proxy) must never replay them.
func writeSimpleJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", jsonMediaType)
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func writeSimpleHTML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(body))
}
