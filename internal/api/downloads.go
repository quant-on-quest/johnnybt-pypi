package api

import (
	"net/http"
	"strconv"

	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
)

func (a *API) listDownloads(w http.ResponseWriter, r *http.Request, _ *store.User) {
	limit := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 2000 {
		limit = v
	}
	rows, err := a.Store.ListDownloads(r.Context(), limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
