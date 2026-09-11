package api

import (
	"net/http"
	"strings"

	"github.com/quant-on-quest/johnnybt-pypi/internal/auth"
	"github.com/quant-on-quest/johnnybt-pypi/internal/pkgmeta"
	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
)

func (a *API) listUsers(w http.ResponseWriter, r *http.Request, _ *store.User) {
	users, err := a.Store.ListUsers(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

type userInput struct {
	Username string `json:"username"`
	Note     string `json:"note"`
}

func (in *userInput) validate() string {
	in.Username = strings.TrimSpace(in.Username)
	in.Note = strings.TrimSpace(in.Note)
	if in.Username == "" || len(in.Username) > 64 {
		return "username must be 1-64 characters"
	}
	if strings.ContainsAny(in.Username, ":/\\ \t\r\n") {
		return "username may not contain spaces, ':' or slashes"
	}
	return ""
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in userInput
	if err := readJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if _, err := a.Store.GetUserByUsername(r.Context(), in.Username); err == nil {
		writeError(w, http.StatusConflict, "username already exists")
		return
	}
	u, err := a.Store.CreateUser(r.Context(), in.Username, in.Note, false, "")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (a *API) lookupUser(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad user id")
		return nil, false
	}
	u, err := a.Store.GetUser(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return nil, false
	}
	return u, true
}

func (a *API) getUser(w http.ResponseWriter, r *http.Request, _ *store.User) {
	u, ok := a.lookupUser(w, r)
	if !ok {
		return
	}
	ents, err := a.Store.ListUserEntitlements(r.Context(), u.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	toks, err := a.Store.ListTokens(r.Context(), u.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":         u,
		"entitlements": ents,
		"tokens":       toks,
	})
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request, _ *store.User) {
	u, ok := a.lookupUser(w, r)
	if !ok {
		return
	}
	var in userInput
	if err := readJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if other, err := a.Store.GetUserByUsername(r.Context(), in.Username); err == nil && other.ID != u.ID {
		writeError(w, http.StatusConflict, "username already exists")
		return
	}
	if err := a.Store.UpdateUser(r.Context(), u.ID, in.Username, in.Note); err != nil {
		a.fail(w, r, err)
		return
	}
	updated, err := a.Store.GetUser(r.Context(), u.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request, me *store.User) {
	u, ok := a.lookupUser(w, r)
	if !ok {
		return
	}
	if u.ID == me.ID {
		writeError(w, http.StatusBadRequest, "you cannot delete yourself")
		return
	}
	if err := a.Store.DeleteUser(r.Context(), u.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Log.Info("user deleted", "user", u.Username, "by", me.Username)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) grantEntitlement(w http.ResponseWriter, r *http.Request, me *store.User) {
	u, ok := a.lookupUser(w, r)
	if !ok {
		return
	}
	pkg, err := a.Store.GetPackage(r.Context(), pkgmeta.NormalizeName(r.PathValue("package")))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in struct {
		ExpiresAt string `json:"expires_at"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	exp, err := parseExpiry(in.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.Store.GrantEntitlement(r.Context(), u.ID, pkg.ID, exp, me.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Log.Info("entitlement granted", "user", u.Username, "package", pkg.Name, "expires_at", in.ExpiresAt, "by", me.Username)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) revokeEntitlement(w http.ResponseWriter, r *http.Request, me *store.User) {
	u, ok := a.lookupUser(w, r)
	if !ok {
		return
	}
	pkg, err := a.Store.GetPackage(r.Context(), pkgmeta.NormalizeName(r.PathValue("package")))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Store.RevokeEntitlement(r.Context(), u.ID, pkg.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Log.Info("entitlement revoked", "user", u.Username, "package", pkg.Name, "by", me.Username)
	w.WriteHeader(http.StatusNoContent)
}

// createToken mints a token for a user; the plaintext is returned exactly once.
func (a *API) createToken(w http.ResponseWriter, r *http.Request, me *store.User) {
	u, ok := a.lookupUser(w, r)
	if !ok {
		return
	}
	var in struct {
		Name  string `json:"name"`
		Scope string `json:"scope"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Scope == "" {
		in.Scope = auth.ScopeRead
	}
	if !auth.ValidScope(in.Scope) {
		writeError(w, http.StatusBadRequest, "scope must be read or write")
		return
	}
	if in.Scope == auth.ScopeWrite && !u.IsAdmin {
		writeError(w, http.StatusBadRequest, "only admin users can hold write tokens")
		return
	}
	plain := auth.GenerateToken()
	tok, err := a.Store.CreateToken(r.Context(), u.ID, strings.TrimSpace(in.Name), auth.DisplayPrefix(plain), auth.HashToken(plain), in.Scope)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.Log.Info("token created", "user", u.Username, "scope", in.Scope, "prefix", tok.Prefix, "by", me.Username)
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":     plain,
		"info":      tok,
		"index_url": a.baseURL(r) + "/simple/",
	})
}

func (a *API) revokeToken(w http.ResponseWriter, r *http.Request, me *store.User) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad token id")
		return
	}
	tok, err := a.Store.GetToken(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Store.RevokeToken(r.Context(), tok.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Log.Info("token revoked", "prefix", tok.Prefix, "by", me.Username)
	w.WriteHeader(http.StatusNoContent)
}
