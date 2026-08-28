// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package oauthserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
)

func TestCredentialCompatibility(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	cfg := testConfig(t, &now)
	dir := filepath.Dir(cfg.PasswordFile)
	secret := filepath.Join(dir, "client-secret")
	staticToken := filepath.Join(dir, "deployment-token")
	if err := os.WriteFile(secret, []byte("production-client-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staticToken, []byte(strings.Repeat("d", 64)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.ClientSecretFile = secret
	cfg.StaticTokenFile = staticToken

	srv, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	info, err := srv.VerifyToken(t.Context(), strings.Repeat("d", 64), nil)
	if err != nil || info.UserID != "local-deployment" || len(info.Scopes) != 1 || info.Scopes[0] != ScopeMemory {
		t.Fatalf("deployment static token not preserved: info=%#v err=%v", info, err)
	}
	if _, err := srv.VerifyToken(t.Context(), strings.Repeat("e", 64), nil); !errors.Is(err, mcpauth.ErrInvalidToken) {
		t.Fatalf("wrong deployment token accepted: %v", err)
	}

	meta := httptest.NewRecorder()
	srv.AuthorizationServerMetadataHandler().ServeHTTP(meta, httptest.NewRequest(http.MethodGet, cfg.Issuer+"/.well-known/oauth-authorization-server", nil))
	var metadata map[string]any
	if err := json.Unmarshal(meta.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	methods, _ := metadata["token_endpoint_auth_methods_supported"].([]any)
	if len(methods) != 2 || methods[0] != "client_secret_post" || methods[1] != "client_secret_basic" {
		t.Fatalf("confidential client metadata mismatch: %#v", metadata)
	}

	verifier := "v" + strings.Repeat("s", 63)
	code := authorizationCode(t, srv, ScopeMemory, verifier)
	missingSecret := postForm(t, srv.TokenHandler(), "/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {cfg.RedirectURIs[0]},
		"client_id": {cfg.ClientID}, "code_verifier": {verifier}, "resource": {srv.Resource()},
	})
	if missingSecret.Code != http.StatusUnauthorized || !strings.Contains(missingSecret.Body.String(), "invalid_client") {
		t.Fatalf("confidential client accepted without secret: %d %s", missingSecret.Code, missingSecret.Body.String())
	}
	good := postForm(t, srv.TokenHandler(), "/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {cfg.RedirectURIs[0]},
		"client_id": {cfg.ClientID}, "client_secret": {"production-client-secret"}, "code_verifier": {verifier}, "resource": {srv.Resource()},
	})
	if good.Code != http.StatusOK {
		t.Fatalf("confidential client token exchange failed: %d %s", good.Code, good.Body.String())
	}

	verifierBasic := "b" + strings.Repeat("s", 63)
	codeBasic := authorizationCode(t, srv, ScopeMemory, verifierBasic)
	basicForm := url.Values{
		"grant_type": {"authorization_code"}, "code": {codeBasic}, "redirect_uri": {cfg.RedirectURIs[0]},
		"code_verifier": {verifierBasic}, "resource": {srv.Resource()},
	}
	basicReq := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(basicForm.Encode()))
	basicReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	basicReq.SetBasicAuth(url.QueryEscape(cfg.ClientID), url.QueryEscape("production-client-secret"))
	basic := httptest.NewRecorder()
	srv.TokenHandler().ServeHTTP(basic, basicReq)
	if basic.Code != http.StatusOK {
		t.Fatalf("client_secret_basic token exchange failed: %d %s", basic.Code, basic.Body.String())
	}

	verifierMixed := "m" + strings.Repeat("s", 63)
	codeMixed := authorizationCode(t, srv, ScopeMemory, verifierMixed)
	mixedForm := url.Values{
		"grant_type": {"authorization_code"}, "code": {codeMixed}, "redirect_uri": {cfg.RedirectURIs[0]},
		"client_id": {cfg.ClientID}, "client_secret": {"production-client-secret"},
		"code_verifier": {verifierMixed}, "resource": {srv.Resource()},
	}
	mixedReq := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(mixedForm.Encode()))
	mixedReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mixedReq.SetBasicAuth(url.QueryEscape(cfg.ClientID), url.QueryEscape("production-client-secret"))
	mixed := httptest.NewRecorder()
	srv.TokenHandler().ServeHTTP(mixed, mixedReq)
	if mixed.Code != http.StatusUnauthorized || !strings.Contains(mixed.Body.String(), "invalid_client") {
		t.Fatalf("mixed client auth methods were not rejected: %d %s", mixed.Code, mixed.Body.String())
	}
}

func TestStaticTokenUsesLegacyFormatAndHTTPMiddleware(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	cfg := testConfig(t, &now)
	dir := filepath.Dir(cfg.PasswordFile)
	staticToken := filepath.Join(dir, "deployment-token-middleware")
	token := strings.Repeat("a", 64)
	if err := os.WriteFile(staticToken, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.StaticTokenFile = staticToken
	srv, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	protected := srv.Protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("static deployment token failed through bearer middleware: %d %s", rec.Code, rec.Body.String())
	}
	_ = srv.Close()

	if err := os.WriteFile(staticToken, []byte("too-short\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(cfg); err == nil || !strings.Contains(err.Error(), "64 hexadecimal") {
		t.Fatalf("weak deployment token was not rejected: %v", err)
	}

	cfg.StaticTokenFile = ""
	cfg.ClientSecretFile = "relative-client-secret"
	if _, err := Open(cfg); err == nil || !strings.Contains(err.Error(), "path must be absolute") {
		t.Fatalf("relative client secret path was not rejected: %v", err)
	}
}
