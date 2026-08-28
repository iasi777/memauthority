// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStreamableHTTPReadToolsAndRuntimeResource(t *testing.T) {
	root := fixtureRepo(t)
	app, err := Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	hs := httptest.NewServer(app.HTTPHandler())
	defer hs.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "transport-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: hs.URL, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		got[tool.Name] = tool
	}
	readTools := []string{"memory_route", "memory_read", "memory_search", "memory_project_runtime", "memory_status"}
	for _, name := range readTools {
		tool := got[name]
		if tool == nil || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("missing or non-read-only tool %s: %#v", name, tool)
		}
	}
	type mutationHints struct{ idempotent, destructive bool }
	mutationTools := map[string]mutationHints{
		"memory_append_progress":            {true, false},
		"memory_update_handoff":             {false, true},
		"memory_update_sections":            {false, true},
		"memory_mark_verified":              {false, true},
		"memory_record_pitfall":             {true, false},
		"memory_create_project":             {false, false},
		"memory_archive_project":            {true, true},
		"memory_restore_project":            {true, true},
		"memory_migrate_lifecycle":          {false, true},
		"memory_update_runtime":             {false, true},
		"memory_record_runtime_observation": {true, false},
	}
	for name, hints := range mutationTools {
		tool := got[name]
		if tool == nil || tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
			t.Fatalf("missing or read-only mutation tool %s: %#v", name, tool)
		}
		if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint != hints.destructive {
			t.Fatalf("mutation tool %s destructive hint=%v want %v", name, tool.Annotations.DestructiveHint, hints.destructive)
		}
		if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("mutation tool %s open-world hint mismatch: %#v", name, tool.Annotations)
		}
		if tool.Annotations.IdempotentHint != hints.idempotent {
			t.Fatalf("mutation tool %s idempotent hint=%v want %v", name, tool.Annotations.IdempotentHint, hints.idempotent)
		}
	}
	if len(got) != 16 {
		t.Fatalf("unexpected Slice-6 tool set (%d): %v", len(got), got)
	}

	route := call(t, session, "memory_route", map[string]any{"query": "baseline-demo"})
	if route["status"] != "unique" || route["project_id"] != "demo" {
		t.Fatalf("route: %#v", route)
	}
	resourceURIs, ok := route["resource_uris"].(map[string]any)
	if !ok || len(resourceURIs) != 4 || resourceURIs["handoff"] != "memory://projects/demo/handoff" {
		t.Fatalf("route resource URIs: %#v", route)
	}
	if _, exists := resourceURIs["runtime"]; exists {
		t.Fatalf("runtime became a normal role URI: %#v", route)
	}
	otherRoute := call(t, session, "memory_route", map[string]any{"query": "other"})
	otherURIs, ok := otherRoute["resource_uris"].(map[string]any)
	if otherRoute["status"] != "unique" || !ok || len(otherURIs) != 1 || otherURIs["handoff"] != "memory://projects/other/handoff" {
		t.Fatalf("route included absent optional roles: %#v", otherRoute)
	}
	runtimeView := call(t, session, "memory_project_runtime", map[string]any{"project_id": "demo", "host_id": "arm", "detail": "checks"})
	if runtimeView["status"] != "ok" {
		t.Fatalf("runtime: %#v", runtimeView)
	}

	raw, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "memory://projects/demo/runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Contents) != 1 || raw.Contents[0].MIMEType != "application/yaml" || !strings.Contains(raw.Contents[0].Text, "schema_version: 1") {
		t.Fatalf("runtime resource: %#v", raw)
	}
	pseudoRole := call(t, session, "memory_read", map[string]any{"uri": "memory://projects/demo/runtime"})
	if pseudoRole["status"] != "error" || pseudoRole["code"] != "read_failed" {
		t.Fatalf("runtime became memory role: %#v", pseudoRole)
	}

	before := gitOutput(t, root, "rev-parse", "HEAD")
	disabled := call(t, session, "memory_append_progress", map[string]any{
		"project_id": "demo", "summary": "disabled transport", "content": "must not commit",
	})
	if disabled["status"] != "error" || disabled["code"] != "write_disabled" {
		t.Fatalf("default write gate: %#v", disabled)
	}
	status := call(t, session, "memory_status", map[string]any{})
	if status["write_enabled"] != false || status["read_only"] != true {
		t.Fatalf("default status: %#v", status)
	}
	if after := gitOutput(t, root, "rev-parse", "HEAD"); after != before {
		t.Fatalf("disabled mutation moved HEAD: before=%s after=%s", before, after)
	}
	if dirty := gitOutput(t, root, "status", "--porcelain"); dirty != "" {
		t.Fatalf("disabled mutation dirtied Authority: %q", dirty)
	}
}

func call(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	out, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("%s structured result type %T", name, result.StructuredContent)
	}
	return out
}

func fixtureRepo(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(file), "..", "..", "conformance", "fixtures", "runtime-read")
	dst := filepath.Join(t.TempDir(), "vault")
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	git(t, dst, "init", "-q", "-b", "main")
	git(t, dst, "config", "user.name", "Fixture")
	git(t, dst, "config", "user.email", "fixture@example.invalid")
	git(t, dst, "add", ".")
	cmd := exec.Command("git", "-C", dst, "commit", "-q", "-m", "seed")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid", "GIT_AUTHOR_DATE=2026-08-11T00:00:00Z", "GIT_COMMITTER_DATE=2026-08-11T00:00:00Z")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	return dst
}
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	a := append([]string{"-C", dir}, args...)
	if out, err := exec.Command("git", a...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		_, ce := io.Copy(out, in)
		xe := out.Close()
		if ce != nil {
			return ce
		}
		return xe
	})
}

func TestWriteEnabledHTTPRequiresOAuth(t *testing.T) {
	root := fixtureRepo(t)
	app, err := OpenWithOptions(root, t.TempDir(), OpenOptions{WriteEnabled: true, WriteSource: "guard-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	req := httptest.NewRequest("POST", "http://127.0.0.1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	rec := httptest.NewRecorder()
	app.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), "requires OAuth") {
		t.Fatalf("write-enabled HTTP without OAuth was not rejected: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRuntimeMutationTransportRejectsUnknownCommandField(t *testing.T) {
	root := fixtureRepo(t)
	app, err := OpenWithOptions(root, t.TempDir(), OpenOptions{WriteEnabled: true, WriteSource: "schema-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := app.Server().Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "schema-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	status := call(t, session, "memory_status", map[string]any{})
	indexRevision, _ := status["index_revision"].(string)
	before := gitOutput(t, root, "rev-parse", "HEAD")
	args := map[string]any{
		"project_id":              "other",
		"expected_index_revision": indexRevision,
		"runtime": map[string]any{
			"schema_version": 1,
			"project_id":     "other",
			"environments": []any{map[string]any{
				"environment_id": "amd-test", "host_id": "amd", "kind": "workspace", "purpose": "test",
				"description": "schema rejection", "workdir": "/workspace/other",
				"repository":   map[string]any{"path": "/workspace/other", "branch": "main"},
				"capabilities": []any{"test"}, "supervisor": map[string]any{"kind": "none"},
				"checks":     []any{map[string]any{"kind": "directory_exists", "path": "/workspace/other", "command": "echo forbidden"}},
				"boundaries": map[string]any{"production_writer": false, "mutation_requires_explicit_authorization": true},
			}},
			"relations": []any{}, "verification": map[string]any{},
		},
	}
	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_update_runtime", Arguments: args})
	if callErr == nil && result != nil && !result.IsError {
		t.Fatalf("unknown command field crossed typed MCP schema: %#v", result.StructuredContent)
	}
	if after := gitOutput(t, root, "rev-parse", "HEAD"); after != before {
		t.Fatalf("schema-rejected command material moved HEAD: before=%s after=%s", before, after)
	}
	if dirty := gitOutput(t, root, "status", "--porcelain"); dirty != "" {
		t.Fatalf("schema-rejected command material dirtied Authority: %q", dirty)
	}
}
