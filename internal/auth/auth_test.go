package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Errorf("unexpected hash format: %s", hash)
	}
	if !VerifyPassword(hash, "correct horse") {
		t.Error("correct password rejected")
	}
	if VerifyPassword(hash, "wrong") {
		t.Error("wrong password accepted")
	}
	if VerifyPassword("garbage", "x") || VerifyPassword("", "x") {
		t.Error("malformed hash accepted")
	}
}

func TestGeneratePasswordShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		p := GeneratePassword()
		parts := strings.Split(p, "-")
		if len(parts) != 4 {
			t.Fatalf("password %q should have 4 groups", p)
		}
		for _, part := range parts {
			if len(part) != 4 {
				t.Fatalf("group %q in %q should be 4 chars", part, p)
			}
		}
		if strings.ContainsAny(p, "0O1lI") {
			t.Errorf("password %q contains ambiguous characters", p)
		}
		if seen[p] {
			t.Errorf("duplicate password generated: %q", p)
		}
		seen[p] = true
	}
}

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Error("short password accepted")
	}
	if ValidatePassword("long enough") != nil {
		t.Error("valid password rejected")
	}
}

func TestTokenShape(t *testing.T) {
	tok := GenerateToken()
	if !LooksLikeToken(tok) {
		t.Fatalf("generated token %q fails LooksLikeToken", tok)
	}
	if !strings.HasPrefix(tok, TokenPrefix) || len(tok) != len(TokenPrefix)+tokenLen {
		t.Errorf("bad token shape %q", tok)
	}
	if DisplayPrefix(tok) != tok[:DisplayLen] {
		t.Errorf("DisplayPrefix = %q", DisplayPrefix(tok))
	}
	if HashToken(tok) == HashToken(tok+"x") || len(HashToken(tok)) != 64 {
		t.Error("HashToken not a distinct sha256 hex")
	}
	if GenerateToken() == tok {
		t.Error("tokens should not repeat")
	}
	for _, bad := range []string{"", "jbt_", "abc", TokenPrefix + strings.Repeat("a", tokenLen-1)} {
		if LooksLikeToken(bad) {
			t.Errorf("LooksLikeToken(%q) should be false", bad)
		}
	}
}

func TestTokenFromRequest(t *testing.T) {
	tok := GenerateToken()
	cases := []struct {
		user, pass string
		want       string
		ok         bool
	}{
		{"__token__", tok, tok, true},
		{"anything", tok, tok, true},
		{tok, "", tok, true},
		{"user", "password", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/simple/", nil)
		r.SetBasicAuth(c.user, c.pass)
		got, ok := TokenFromRequest(r)
		if ok != c.ok || got != c.want {
			t.Errorf("TokenFromRequest(%q:%q) = (%q, %v), want (%q, %v)", c.user, c.pass, got, ok, c.want, c.ok)
		}
	}
	r := httptest.NewRequest("GET", "/simple/", nil)
	if _, ok := TokenFromRequest(r); ok {
		t.Error("request without auth header should not yield a token")
	}
}

func TestValidScope(t *testing.T) {
	if !ValidScope(ScopeRead) || !ValidScope(ScopeWrite) || ValidScope("admin") || ValidScope("") {
		t.Error("ValidScope wrong")
	}
}
