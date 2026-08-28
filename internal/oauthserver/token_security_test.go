// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package oauthserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
)

func TestRefreshReuseRevokesWholeFamily(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	cfg := testConfig(t, &now)
	srv, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	issued := fullGrant(t, srv, ScopeMemory, "v"+strings.Repeat("r", 63))
	rotatedRec := postForm(t, srv.TokenHandler(), "/token", url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {issued.Refresh},
		"client_id": {cfg.ClientID}, "resource": {srv.Resource()},
	})
	if rotatedRec.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", rotatedRec.Code, rotatedRec.Body.String())
	}
	var rotated map[string]any
	if err := jsonUnmarshal(rotatedRec.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	newAccess := rotated["access_token"].(string)
	newRefresh := rotated["refresh_token"].(string)
	if _, err := srv.VerifyToken(t.Context(), newAccess, nil); err != nil {
		t.Fatalf("new access invalid before reuse: %v", err)
	}

	replay := postForm(t, srv.TokenHandler(), "/token", url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {issued.Refresh},
		"client_id": {cfg.ClientID}, "resource": {srv.Resource()},
	})
	if replay.Code != http.StatusBadRequest || !strings.Contains(replay.Body.String(), "invalid_grant") {
		t.Fatalf("replay result: %d %s", replay.Code, replay.Body.String())
	}
	for label, token := range map[string]string{"old_access": issued.Access, "new_access": newAccess} {
		if _, err := srv.VerifyToken(t.Context(), token, nil); !errors.Is(err, mcpauth.ErrInvalidToken) {
			t.Fatalf("%s survived family reuse revocation: %v", label, err)
		}
	}
	familyRefresh := postForm(t, srv.TokenHandler(), "/token", url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {newRefresh},
		"client_id": {cfg.ClientID}, "resource": {srv.Resource()},
	})
	if familyRefresh.Code != http.StatusBadRequest || !strings.Contains(familyRefresh.Body.String(), "invalid_grant") {
		t.Fatalf("new family refresh survived reuse: %d %s", familyRefresh.Code, familyRefresh.Body.String())
	}
}

func TestLoginRateLimitMatchesCompatibilityBaseline(t *testing.T) {
	now := time.Date(2026, 8, 13, 2, 0, 0, 0, time.UTC)
	cfg := testConfig(t, &now)
	srv, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	transaction, csrf, cookie := loginFixture(t, srv)
	host := "198.51.100.25:4567"
	for i := 1; i <= 5; i++ {
		rec := loginPost(t, srv, transaction, csrf, cookie, host, srv.cfg.Username, "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d=%d body=%s", i, rec.Code, rec.Body.String())
		}
	}
	sixth := loginPost(t, srv, transaction, csrf, cookie, host, srv.cfg.Username, "wrong-password")
	if sixth.Code != http.StatusTooManyRequests || sixth.Header().Get("Retry-After") != "60" {
		t.Fatalf("sixth failure=%d retry=%q body=%s", sixth.Code, sixth.Header().Get("Retry-After"), sixth.Body.String())
	}

	now = now.Add(301 * time.Second)
	success := loginPost(t, srv, transaction, csrf, cookie, host, srv.cfg.Username, srv.password)
	if success.Code != http.StatusFound {
		t.Fatalf("post-window success=%d body=%s", success.Code, success.Body.String())
	}
	var attempts int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM login_attempts`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatalf("successful login did not clear host failures: %d rows", attempts)
	}
}

func loginFixture(t *testing.T, srv *Server) (string, string, *http.Cookie) {
	t.Helper()
	verifier := "v" + strings.Repeat("l", 63)
	a := authorize(t, srv, ScopeMemory, verifier, srv.Resource())
	if a.Code != http.StatusFound {
		t.Fatalf("authorize=%d %s", a.Code, a.Body.String())
	}
	loginURL, err := url.Parse(a.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, loginURL.String(), nil)
	rec := httptest.NewRecorder()
	srv.LoginHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login GET=%d %s", rec.Code, rec.Body.String())
	}
	tx := regexp.MustCompile(`name="transaction" value="([^"]+)"`).FindStringSubmatch(rec.Body.String())
	csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(rec.Body.String())
	cookies := rec.Result().Cookies()
	if len(tx) != 2 || len(csrf) != 2 || len(cookies) != 1 {
		t.Fatalf("login fixture malformed")
	}
	return tx[1], csrf[1], cookies[0]
}

func loginPost(t *testing.T, srv *Server, transaction, csrf string, cookie *http.Cookie, remoteAddr, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"transaction": {transaction}, "csrf": {csrf}, "username": {username}, "password": {password}}
	req := httptest.NewRequest(http.MethodPost, srv.cfg.Issuer+"/oauth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = remoteAddr
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.LoginHandler().ServeHTTP(rec, req)
	return rec
}

func jsonUnmarshal(raw []byte, out any) error { return json.Unmarshal(raw, out) }
