package api

import (
	"errors"
	"net/http"

	"github.com/quant-on-quest/johnnybt-pypi/internal/pkgmeta"
	"github.com/quant-on-quest/johnnybt-pypi/internal/pypi"
	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
)

func (a *API) listPackages(w http.ResponseWriter, r *http.Request, _ *store.User) {
	pkgs, err := a.Store.ListPackages(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	counts, err := a.Store.DownloadCounts(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	type row struct {
		store.PackageSummary
		DownloadCount int `json:"download_count"`
	}
	out := make([]row, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, row{PackageSummary: p, DownloadCount: counts[p.ID]})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) lookupPackage(w http.ResponseWriter, r *http.Request) (*store.Package, bool) {
	pkg, err := a.Store.GetPackage(r.Context(), pkgmeta.NormalizeName(r.PathValue("name")))
	if err != nil {
		a.fail(w, r, err)
		return nil, false
	}
	return pkg, true
}

func (a *API) getPackage(w http.ResponseWriter, r *http.Request, _ *store.User) {
	pkg, ok := a.lookupPackage(w, r)
	if !ok {
		return
	}
	rels, err := a.Store.ListReleases(r.Context(), pkg.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	type releaseView struct {
		store.Release
		Files []store.File `json:"files"`
	}
	views := make([]releaseView, 0, len(rels))
	for _, rel := range rels {
		files, err := a.Store.ListReleaseFiles(r.Context(), rel.ID)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		views = append(views, releaseView{Release: rel, Files: files})
	}
	users, err := a.Store.ListPackageEntitlements(r.Context(), pkg.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	latest, _ := a.Store.LatestVersion(r.Context(), pkg.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"package":        pkg,
		"latest_version": latest,
		"releases":       views,
		"users":          users,
	})
}

func (a *API) deletePackage(w http.ResponseWriter, r *http.Request, _ *store.User) {
	pkg, ok := a.lookupPackage(w, r)
	if !ok {
		return
	}
	files, err := a.Store.ListPackageFiles(r.Context(), pkg.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Store.DeletePackage(r.Context(), pkg.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	for _, f := range files {
		pypi.DeleteFileBlobs(r.Context(), a.Blobs, &f.File)
	}
	a.Log.Info("package deleted", "package", pkg.Name, "files", len(files))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) lookupRelease(w http.ResponseWriter, r *http.Request) (*store.Package, *store.Release, bool) {
	pkg, ok := a.lookupPackage(w, r)
	if !ok {
		return nil, nil, false
	}
	rel, err := a.Store.GetRelease(r.Context(), pkg.ID, r.PathValue("version"))
	if err != nil {
		a.fail(w, r, err)
		return nil, nil, false
	}
	return pkg, rel, true
}

func (a *API) yankRelease(w http.ResponseWriter, r *http.Request, _ *store.User) {
	pkg, rel, ok := a.lookupRelease(w, r)
	if !ok {
		return
	}
	var in struct {
		Yanked bool   `json:"yanked"`
		Reason string `json:"reason"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := a.Store.SetYanked(r.Context(), rel.ID, in.Yanked, in.Reason); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Log.Info("release yank changed", "package", pkg.Name, "version", rel.Version, "yanked", in.Yanked)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) deleteRelease(w http.ResponseWriter, r *http.Request, _ *store.User) {
	pkg, rel, ok := a.lookupRelease(w, r)
	if !ok {
		return
	}
	files, err := a.Store.ListReleaseFiles(r.Context(), rel.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Store.DeleteRelease(r.Context(), rel.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	for i := range files {
		pypi.DeleteFileBlobs(r.Context(), a.Blobs, &files[i])
	}
	a.Log.Info("release deleted", "package", pkg.Name, "version", rel.Version)
	w.WriteHeader(http.StatusNoContent)
}

// upload accepts one or more distribution files from the admin UI under the
// multipart field "files".
func (a *API) upload(w http.ResponseWriter, r *http.Request, u *store.User) {
	r.Body = http.MaxBytesReader(w, r.Body, 8*a.Ingester.MaxSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer r.MultipartForm.RemoveAll()
	heads := r.MultipartForm.File["files"]
	if len(heads) == 0 {
		heads = r.MultipartForm.File["file"]
	}
	if len(heads) == 0 {
		writeError(w, http.StatusBadRequest, "no files uploaded")
		return
	}
	type result struct {
		Filename string `json:"filename"`
		Package  string `json:"package,omitempty"`
		Version  string `json:"version,omitempty"`
		Error    string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(heads))
	failed := 0
	for _, hdr := range heads {
		part, err := hdr.Open()
		if err != nil {
			results = append(results, result{Filename: hdr.Filename, Error: err.Error()})
			failed++
			continue
		}
		res, err := a.Ingester.Ingest(r.Context(), hdr.Filename, part, "", u.ID)
		part.Close()
		if err != nil {
			msg := err.Error()
			if !errors.Is(err, pypi.ErrFileExists) && !errors.Is(err, pypi.ErrBadFile) && !errors.Is(err, pypi.ErrTooLarge) {
				a.Log.Error("upload failed", "file", hdr.Filename, "err", err)
				msg = "internal error"
			}
			results = append(results, result{Filename: hdr.Filename, Error: msg})
			failed++
			continue
		}
		a.Log.Info("uploaded", "package", res.Package.Name, "version", res.Release.Version, "file", res.File.Filename, "by", u.Username)
		results = append(results, result{Filename: hdr.Filename, Package: res.Package.Name, Version: res.Release.Version})
	}
	status := http.StatusOK
	if failed == len(heads) {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, results)
}
