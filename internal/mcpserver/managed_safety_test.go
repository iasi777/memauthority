// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iasi777/memauthority/internal/oauthserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReadOnlyStatusReportsRealWorkspaceAndHeadDrift(t *testing.T) {
	root := fixtureRepo(t)
	app, err := Open(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	owned := app.ownedHead
	role := filepath.Join(root, "demo", "进度.md")
	f, err := os.OpenFile(role, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\nDIRTY-READONLY-STATUS\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	session := inMemorySession(t, app)
	status := call(t, session, "memory_status", map[string]any{})
	if status["workspace_clean"] != false || status["snapshot_state"] != "degraded" || status["owned_head"] != owned {
		t.Fatalf("dirty read-only status: %#v", status)
	}
	dirty, _ := status["dirty_paths"].([]any)
	_ = dirty
}

func TestToolDescriptionsSchemasAndDestructiveHints(t *testing.T) {
	root := fixtureRepo(t)
	app, err := Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	session := inMemorySession(t, app)
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*mcp.Tool{}
	for _, tool := range listed.Tools {
		tools[tool.Name] = tool
	}

	destructive := map[string]bool{
		"memory_append_progress": false, "memory_record_pitfall": false, "memory_create_project": false,
		"memory_record_runtime_observation": false,
		"memory_update_handoff":             true, "memory_update_sections": true, "memory_mark_verified": true,
		"memory_archive_project": true, "memory_restore_project": true, "memory_migrate_lifecycle": true, "memory_update_runtime": true,
	}
	for name, want := range destructive {
		got := tools[name]
		if got == nil || got.Annotations == nil || got.Annotations.DestructiveHint == nil || *got.Annotations.DestructiveHint != want {
			t.Fatalf("%s destructive=%#v want %v", name, got, want)
		}
	}
	search := tools["memory_search"]
	if search == nil || !strings.Contains(search.Description, "50") {
		t.Fatalf("search limit absent from description: %#v", search)
	}
	if schemaPropertyDescription(search, "cross_project") == "" {
		t.Fatalf("search cross_project schema lacks model-facing description: %#v", search.InputSchema)
	}
	sections := tools["memory_update_sections"]
	if schemaPropertyDescription(sections, "operations") == "" {
		t.Fatalf("section operations schema lacks model-facing description")
	}
	if schemaPropertyDescription(tools["memory_create_project"], "lifecycle") == "" {
		t.Fatalf("create-project lifecycle schema lacks model-facing description")
	}
	route := tools["memory_route"]
	queryDescription := schemaPropertyDescription(route, "query")
	if route == nil || queryDescription == "" {
		t.Fatalf("route query schema lacks model-facing description")
	}
	for label, description := range map[string]string{"tool": route.Description, "query": queryDescription} {
		lower := strings.ToLower(description)
		if strings.Contains(lower, "substring") || !strings.Contains(lower, "exact") || !strings.Contains(lower, "alias") {
			t.Fatalf("route %s description contradicts exact ID/alias contract: %q", label, description)
		}
	}
}

func inMemorySession(t *testing.T, app *App) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := app.Server().Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "p4c-slice5", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func schemaPropertyDescription(tool *mcp.Tool, property string) string {
	if tool == nil {
		return ""
	}
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		return ""
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return ""
	}
	p, ok := props[property].(map[string]any)
	if !ok {
		return ""
	}
	d, _ := p["description"].(string)
	return d
}

func TestOAuthContainmentRejectsSymlinkIntoVault(t *testing.T) {
	root := fixtureRepo(t)
	work := t.TempDir()
	password := filepath.Join(work, "password")
	if err := os.WriteFile(password, []byte("test-password\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(work, "vault-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	_, err := OpenWithOptions(root, work, OpenOptions{OAuth: &oauthserver.Config{
		Issuer: "http://127.0.0.1:9915", ClientID: "test-client",
		RedirectURIs: []string{"http://127.0.0.1:7777/callback"}, Username: "tester",
		PasswordFile: password, StateDB: filepath.Join(link, "oauth.sqlite3"),
	}})
	if err == nil || !strings.Contains(err.Error(), "outside Vault Authority") {
		t.Fatalf("symlinked OAuth state path was not rejected: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "oauth.sqlite3")); !os.IsNotExist(statErr) {
		t.Fatalf("rejected symlink state path created Authority file: %v", statErr)
	}
}
