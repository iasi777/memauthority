// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iasi777/memauthority/internal/oauthserver"
)

func TestProtectedHTTPDiscoveryAndBearerChallenge(t *testing.T) {
	root := fixtureRepo(t)
	work := t.TempDir()
	password := filepath.Join(work, "password")
	if err := os.WriteFile(password, []byte("test-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	issuer := "http://127.0.0.1:9902"
	app, err := OpenWithOptions(root, work, OpenOptions{OAuth: &oauthserver.Config{
		Issuer: issuer, ClientID: "test-client",
		RedirectURIs: []string{"http://127.0.0.1:7777/callback"},
		Username:     "tester", PasswordFile: password, StateDB: filepath.Join(work, "oauth.sqlite3"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	recorder := httptest.NewRecorder()
	app.HTTPHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated MCP status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	challenge := recorder.Header().Get("WWW-Authenticate")
	if !strings.Contains(challenge, `Bearer`) || !strings.Contains(challenge, `resource_metadata="`+issuer+`/.well-known/oauth-protected-resource/mcp"`) || !strings.Contains(challenge, `scope="memory"`) {
		t.Fatalf("bad WWW-Authenticate: %q", challenge)
	}

	prmReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/.well-known/oauth-protected-resource/mcp", nil)
	prmRec := httptest.NewRecorder()
	app.HTTPHandler().ServeHTTP(prmRec, prmReq)
	if prmRec.Code != http.StatusOK {
		t.Fatalf("PRM status=%d body=%s", prmRec.Code, prmRec.Body.String())
	}
	var prm map[string]any
	if err := json.Unmarshal(prmRec.Body.Bytes(), &prm); err != nil {
		t.Fatal(err)
	}
	if prm["resource"] != issuer+"/mcp" {
		t.Fatalf("bad resource metadata: %#v", prm)
	}

	metaReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/.well-known/oauth-authorization-server", nil)
	metaRec := httptest.NewRecorder()
	app.HTTPHandler().ServeHTTP(metaRec, metaReq)
	var meta map[string]any
	if err := json.Unmarshal(metaRec.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	methods, _ := meta["token_endpoint_auth_methods_supported"].([]any)
	if len(methods) != 1 || methods[0] != "none" {
		t.Fatalf("public client metadata must advertise token auth none: %#v", meta)
	}
	if _, exists := meta["registration_endpoint"]; exists {
		t.Fatalf("dynamic registration was introduced: %#v", meta)
	}
	if out, err := os.ReadDir(root); err != nil || len(out) == 0 {
		t.Fatalf("vault unexpectedly unavailable: %v", err)
	}
	if status := gitOutput(t, root, "status", "--porcelain"); status != "" {
		t.Fatalf("OAuth HTTP touched Authority: %q", status)
	}
}

func TestOAuthStateInsideVaultRejected(t *testing.T) {
	root := fixtureRepo(t)
	work := t.TempDir()
	password := filepath.Join(work, "password")
	if err := os.WriteFile(password, []byte("test-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenWithOptions(root, work, OpenOptions{OAuth: &oauthserver.Config{
		Issuer: "http://127.0.0.1:9903", ClientID: "test-client",
		RedirectURIs: []string{"http://127.0.0.1:7777/callback"}, Username: "tester",
		PasswordFile: password, StateDB: filepath.Join(root, "oauth.sqlite3"),
	}})
	if err == nil || !strings.Contains(err.Error(), "outside Vault Authority") {
		t.Fatalf("OAuth state inside Authority was not rejected: %v", err)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
