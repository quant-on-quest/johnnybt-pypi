// Package api is the JSON API behind the admin SPA plus the public
// token self-check endpoint.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/quant-on-quest/johnnybt-pypi/internal/auth"
	"github.com/quant-on-quest/johnnybt-pypi/internal/blob"
	"github.com/quant-on-quest/johnnybt-pypi/internal/config"
	"github.com/quant-on-quest/johnnybt-pypi/internal/pypi"
	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
)

const sessionCookie = "pypi_session"

type API struct {
	Store    *store.Store
	Blobs    blob.Store
	Ingester *pypi.Ingester
	Cfg      config.Config
	Log      *slog.Logger
}

func (a *API) Register(mux *http.ServeMux) {
	// public
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)
	mux.HandleFunc("GET /api/v1/auth/me", a.me)
	mux.HandleFunc("POST /api/v1/me", a.tokenSelfCheck)

	// admin only
	mux.HandleFunc("PUT /api/v1/auth/password", a.admin(a.changePassword))
	mux.HandleFunc("GET /api/v1/config", a.admin(a.serverConfig))

	mux.HandleFunc("GET /api/v1/packages", a.admin(a.listPackages))
	mux.HandleFunc("POST /api/v1/packages/upload", a.admin(a.upload))
	mux.HandleFunc("GET /api/v1/packages/{name}", a.admin(a.getPackage))
	mux.HandleFunc("DELETE /api/v1/packages/{name}", a.admin(a.deletePackage))
	mux.HandleFunc("POST /api/v1/packages/{name}/releases/{version}/yank", a.admin(a.yankRelease))
	mux.HandleFunc("DELETE /api/v1/packages/{name}/releases/{version}", a.admin(a.deleteRelease))

	mux.HandleFunc("GET /api/v1/users", a.admin(a.listUsers))
	mux.HandleFunc("POST /api/v1/users", a.admin(a.createUser))
	mux.HandleFunc("GET /api/v1/users/{id}", a.admin(a.getUser))
	mux.HandleFunc("PUT /api/v1/users/{id}", a.admin(a.updateUser))
	mux.HandleFunc("DELETE /api/v1/users/{id}", a.admin(a.deleteUser))
	mux.HandleFunc("PUT /api/v1/users/{id}/entitlements/{package}", a.admin(a.grantEntitlement))
	mux.HandleFunc("DELETE /api/v1/users/{id}/entitlements/{package}", a.admin(a.revokeEntitlement))
	mux.HandleFunc("POST /api/v1/users/{id}/tokens", a.admin(a.createToken))
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", a.admin(a.revokeToken))

	mux.HandleFunc("GET /api/v1/downloads", a.admin(a.listDownloads))
}

// --- plumbing ---------------------------------------------------------------

type adminHandler func(w http.ResponseWriter, r *http.Request, u *store.User)

func (a *API) admin(next adminHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := a.sessionUser(r)
		if u == nil {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		if !u.IsAdmin {
			writeError(w, http.StatusForbidden, "admin only")
			return
		}
		next(w, r, u)
	}
}

func (a *API) sessionUser(r *http.Request) *store.User {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	u, err := a.Store.UserBySession(r.Context(), c.Value)
	if err != nil {
		return nil
	}
	return u
}

func (a *API) setSessionCookie(w http.ResponseWriter, r *http.Request, id string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	a.Log.Error("api error", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

// baseURL mirrors the pypi package's derivation so install snippets match.
func (a *API) baseURL(r *http.Request) string {
	if a.Cfg.BaseURL != "" {
		return a.Cfg.BaseURL
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

// parseExpiry accepts "" (never), "YYYY-MM-DD" (end of that day in server
// local time) or a full RFC 3339 timestamp.
func parseExpiry(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		end := t.AddDate(0, 0, 1)
		return &end, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, errors.New("expires_at must be YYYY-MM-DD or RFC 3339")
	}
	return &t, nil
}

// --- auth -------------------------------------------------------------------

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := a.Store.GetUserByUsername(r.Context(), strings.TrimSpace(in.Username))
	if err != nil || u.PasswordHash == "" || !auth.VerifyPassword(u.PasswordHash, in.Password) {
		// Burn comparable time on unknown users so timing does not leak existence.
		if err != nil {
			auth.VerifyPassword("$argon2id$v=19$m=65536,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", in.Password)
		}
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if !u.IsAdmin {
		writeError(w, http.StatusForbidden, "only admins can log in")
		return
	}
	id, err := a.Store.CreateSession(r.Context(), u.ID, a.Cfg.SessionTTL)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.setSessionCookie(w, r, id, a.Cfg.SessionTTL)
	writeJSON(w, http.StatusOK, u)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = a.Store.DeleteSession(r.Context(), c.Value)
	}
	a.setSessionCookie(w, r, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	u := a.sessionUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !auth.VerifyPassword(u.PasswordHash, in.Current) {
		writeError(w, http.StatusForbidden, "current password is wrong")
		return
	}
	if err := auth.ValidatePassword(in.New); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(in.New)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Store.SetPassword(r.Context(), u.ID, hash); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) serverConfig(w http.ResponseWriter, r *http.Request, _ *store.User) {
	base := a.baseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"base_url":   base,
		"index_url":  base + "/simple/",
		"upload_url": base + "/legacy/",
	})
}

// tokenSelfCheck lets a customer paste their token to see what it unlocks.
func (a *API) tokenSelfCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Token = strings.TrimSpace(in.Token)
	if !auth.LooksLikeToken(in.Token) {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	tok, u, err := a.Store.ActiveTokenByHash(r.Context(), auth.HashToken(in.Token))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or revoked token")
		return
	}
	var pkgs any
	if u.IsAdmin {
		pkgs, err = a.Store.ListPackages(r.Context())
	} else {
		pkgs, err = a.Store.ListUserEntitlements(r.Context(), u.ID)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	base := a.baseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"username":  u.Username,
		"note":      u.Note,
		"is_admin":  u.IsAdmin,
		"token":     tok,
		"packages":  pkgs,
		"index_url": base + "/simple/",
	})
}
