package auth

import (
	"context"
	"net/http"

	"github.com/quant-on-quest/johnnybt-pypi/internal/store"
)

// Principal is the authenticated caller of a PyPI endpoint.
type Principal struct {
	User  *store.User
	Token *store.Token
}

// CanWrite reports whether the principal may upload distributions.
func (p *Principal) CanWrite() bool {
	return p.User.IsAdmin && p.Token.Scope == ScopeWrite
}

// TokenFromRequest pulls an API token out of HTTP Basic auth. pip/uv/twine
// send "__token__:<token>"; a bare "<token>:" is accepted too.
func TokenFromRequest(r *http.Request) (string, bool) {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return "", false
	}
	if LooksLikeToken(pass) {
		return pass, true
	}
	if LooksLikeToken(user) {
		return user, true
	}
	return "", false
}

// Authenticate resolves the request's token to a principal and records usage.
func Authenticate(ctx context.Context, st *store.Store, r *http.Request) (*Principal, bool) {
	plain, ok := TokenFromRequest(r)
	if !ok {
		return nil, false
	}
	tok, user, err := st.ActiveTokenByHash(ctx, HashToken(plain))
	if err != nil {
		return nil, false
	}
	_ = st.TouchToken(ctx, tok.ID)
	return &Principal{User: user, Token: tok}, true
}
