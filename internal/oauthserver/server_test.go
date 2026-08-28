// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package oauthserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
)

type issuedTokens struct {
	Access  string
	Refresh string
	Scope   string
}

func TestAuthorizationCodeRestartExpiryAndRefreshRotation(t *testing.T) {
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	cfg := testConfig(t, &now)
	srv, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	issued := fullGrant(t, srv, ScopeMemory, "v"+strings.Repeat("x", 63))
	if issued.Scope != ScopeMemory || issued.Access == "" || issued.Refresh == "" {
		t.Fatalf("issued tokens: %#v", issued)
	}
	if _, err := srv.VerifyToken(t.Context(), issued.Access, nil); err != nil {
		t.Fatalf("verify before restart: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if _, err := restarted.VerifyToken(t.Context(), issued.Access, nil); err != nil {
		t.Fatalf("persisted token invalid after restart: %v", err)
	}

	now = now.Add(2 * time.Hour)
	if _, err := restarted.VerifyToken(t.Context(), issued.Access, nil); !errors.Is(err, mcpauth.ErrInvalidToken) {
		t.Fatalf("expired access token accepted: %v", err)
	}

	refresh := postForm(t, restarted.TokenHandler(), "/token", url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {issued.Refresh},
		"client_id":     {cfg.ClientID},
		"resource":      {restarted.Resource()},
	})
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", refresh.Code, refresh.Body.String())
	}
	var rotated map[string]any
	if err := json.Unmarshal(refresh.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	newAccess, _ := rotated["access_token"].(string)
	newRefresh, _ := rotated["refresh_token"].(string)
	if newAccess == "" || newRefresh == "" || newRefresh == issued.Refresh {
		t.Fatalf("refresh rotation failed: %#v", rotated)
	}
	if _, err := restarted.VerifyToken(t.Context(), newAccess, nil); err != nil {
		t.Fatalf("rotated access token invalid: %v", err)
	}
	replay := postForm(t, restarted.TokenHandler(), "/token", url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {issued.Refresh},
		"client_id":     {cfg.ClientID},
		"resource":      {restarted.Resource()},
	})
	if replay.Code != http.StatusBadRequest || !strings.Contains(replay.Body.String(), "invalid_grant") {
		t.Fatalf("old refresh token replay accepted: %d %s", replay.Code, replay.Body.String())
	}
}

func TestScopeResourcePKCEAndRawTokenStorage(t *testing.T) {
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	cfg := testConfig(t, &now)
	srv, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	wrongResource := authorize(t, srv, ScopeMemory, "v"+strings.Repeat("a", 63), "https://wrong.example/mcp")
	if wrongResource.Code != http.StatusBadRequest || !strings.Contains(wrongResource.Body.String(), "invalid_target") {
		t.Fatalf("wrong resource accepted: %d %s", wrongResource.Code, wrongResource.Body.String())
	}
	wrongScope := authorize(t, srv, ScopeOfflineAccess, "v"+strings.Repeat("b", 63), srv.Resource())
	if wrongScope.Code != http.StatusBadRequest || !strings.Contains(wrongScope.Body.String(), "invalid_scope") {
		t.Fatalf("scope without memory accepted: %d %s", wrongScope.Code, wrongScope.Body.String())
	}

	verifier := "v" + strings.Repeat("c", 63)
	code := authorizationCode(t, srv, ScopeMemory, verifier)
	wrongVerifier := postForm(t, srv.TokenHandler(), "/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURIs[0]},
		"client_id":     {cfg.ClientID},
		"code_verifier": {"wrong-verifier"},
		"resource":      {srv.Resource()},
	})
	if wrongVerifier.Code != http.StatusBadRequest || !strings.Contains(wrongVerifier.Body.String(), "invalid_grant") {
		t.Fatalf("wrong PKCE verifier accepted: %d %s", wrongVerifier.Code, wrongVerifier.Body.String())
	}
	good := exchangeCode(t, srv, cfg, code, verifier)
	var body map[string]any
	if err := json.Unmarshal(good.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	access := body["access_token"].(string)
	refresh := body["refresh_token"].(string)
	dbBytes, err := os.ReadFile(cfg.StateDB)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dbBytes, []byte(access)) || bytes.Contains(dbBytes, []byte(refresh)) {
		t.Fatal("OAuth DB contains raw bearer token material")
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(cfg.StateDB); err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("OAuth state DB permissions are not private: info=%v err=%v", info, err)
		}
	}
}

func testConfig(t *testing.T, now *time.Time) Config {
	t.Helper()
	dir := t.TempDir()
	password := filepath.Join(dir, "password")
	if err := os.WriteFile(password, []byte("test-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return Config{
		Issuer:       "http://127.0.0.1:9901",
		ClientID:     "test-client",
		RedirectURIs: []string{"http://127.0.0.1:7777/callback"},
		Username:     "tester",
		PasswordFile: password,
		StateDB:      filepath.Join(dir, "oauth.sqlite3"),
		Now:          func() time.Time { return *now },
	}
}

func fullGrant(t *testing.T, srv *Server, scope, verifier string) issuedTokens {
	t.Helper()
	code := authorizationCode(t, srv, scope, verifier)
	r := exchangeCode(t, srv, srv.cfg, code, verifier)
	if r.Code != http.StatusOK {
		t.Fatalf("token exchange: %d %s", r.Code, r.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return issuedTokens{Access: body["access_token"].(string), Refresh: body["refresh_token"].(string), Scope: body["scope"].(string)}
}

func authorizationCode(t *testing.T, srv *Server, scope, verifier string) string {
	t.Helper()
	a := authorize(t, srv, scope, verifier, srv.Resource())
	if a.Code != http.StatusFound {
		t.Fatalf("authorize: %d %s", a.Code, a.Body.String())
	}
	loginURL, err := url.Parse(a.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	getReq := httptest.NewRequest(http.MethodGet, loginURL.String(), nil)
	getRec := httptest.NewRecorder()
	srv.LoginHandler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("login GET: %d %s", getRec.Code, getRec.Body.String())
	}
	transaction := regexp.MustCompile(`name="transaction" value="([^"]+)"`).FindStringSubmatch(getRec.Body.String())
	csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(getRec.Body.String())
	if len(transaction) != 2 || len(csrf) != 2 {
		t.Fatalf("login form missing hidden fields: %s", getRec.Body.String())
	}
	cookies := getRec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "oauth_csrf" || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("bad CSRF cookie: %#v", cookies)
	}
	form := url.Values{"transaction": {transaction[1]}, "csrf": {csrf[1]}, "username": {srv.cfg.Username}, "password": {srv.password}}
	postReq := httptest.NewRequest(http.MethodPost, srv.cfg.Issuer+"/oauth/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(cookies[0])
	postRec := httptest.NewRecorder()
	srv.LoginHandler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusFound {
		t.Fatalf("login POST: %d %s", postRec.Code, postRec.Body.String())
	}
	callback, err := url.Parse(postRec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if callback.Query().Get("state") != "state123" || callback.Query().Get("iss") != srv.cfg.Issuer {
		t.Fatalf("callback state/issuer lost: %s", callback)
	}
	code := callback.Query().Get("code")
	if code == "" {
		t.Fatal("callback missing authorization code")
	}
	return code
}

func authorize(t *testing.T, srv *Server, scope, verifier, resource string) *httptest.ResponseRecorder {
	t.Helper()
	challenge := pkceChallenge(verifier)
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {srv.cfg.ClientID},
		"redirect_uri":          {srv.cfg.RedirectURIs[0]},
		"scope":                 {scope},
		"state":                 {"state123"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"resource":              {resource},
	}
	req := httptest.NewRequest(http.MethodGet, srv.cfg.Issuer+"/authorize?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	srv.AuthorizeHandler().ServeHTTP(rec, req)
	return rec
}

func exchangeCode(t *testing.T, srv *Server, cfg Config, code, verifier string) *httptest.ResponseRecorder {
	t.Helper()
	return postForm(t, srv.TokenHandler(), "/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURIs[0]},
		"client_id":     {cfg.ClientID},
		"code_verifier": {verifier},
		"resource":      {srv.Resource()},
	})
}

func postForm(t *testing.T, handler http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
