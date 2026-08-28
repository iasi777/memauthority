// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultread

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/iasi777/v-memory/internal/checklist"
)

func TestReadHandoffReturnsStructuredTodoChecklist(t *testing.T) {
	root := fixtureRepo(t)
	path := filepath.Join(root, "demo", "交接.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\n## 已知问题 / 待办\n- [ ] alpha\n- [x] done\n- [ ] <待办 1>\n```md\n- [ ] fenced\n```\n")...)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	svc, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}

	full := svc.Read(ReadArgs{URI: "memory://projects/demo/handoff"})
	items, ok := full["checklist_items"].([]checklist.Item)
	if !ok || len(items) != 2 {
		t.Fatalf("full checklist_items=%#v", full["checklist_items"])
	}
	if items[0].Text != "alpha" || items[0].Checked || items[1].Text != "done" || !items[1].Checked {
		t.Fatalf("unexpected checklist items: %#v", items)
	}
	if items[0].Heading != checklist.TodoHeading || !strings.HasPrefix(items[0].ItemRef, "todo:") {
		t.Fatalf("checklist metadata missing: %#v", items[0])
	}

	todo := svc.Read(ReadArgs{URI: "memory://projects/demo/handoff", Heading: checklist.TodoHeading})
	if got, ok := todo["checklist_items"].([]checklist.Item); !ok || len(got) != 2 {
		t.Fatalf("todo heading checklist_items=%#v", todo["checklist_items"])
	}
	overview := svc.Read(ReadArgs{URI: "memory://projects/demo/handoff", Heading: "Overview"})
	if _, exists := overview["checklist_items"]; exists {
		t.Fatalf("unrelated heading exposed checklist metadata: %#v", overview)
	}
	progress := svc.Read(ReadArgs{URI: "memory://projects/demo/progress"})
	if _, exists := progress["checklist_items"]; exists {
		t.Fatalf("non-handoff read exposed checklist metadata: %#v", progress)
	}
	if full["truncated"] == true {
		cursor, _ := full["cursor"].(string)
		continued := svc.Read(ReadArgs{URI: "memory://projects/demo/handoff", Cursor: cursor})
		if _, exists := continued["checklist_items"]; exists {
			t.Fatalf("cursor continuation repeated checklist metadata: %#v", continued)
		}
	}
}

func TestRouteReadRevisionAndCursor(t *testing.T) {
	root := fixtureRepo(t)
	svc, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	byName := svc.Route("demo")
	if byName["status"] != "unique" || byName["project_id"] != "demo" {
		t.Fatalf("route by name: %#v", byName)
	}
	byAlias := svc.Route("demo-alias")
	if byAlias["status"] != "unique" || byAlias["project_id"] != "demo" {
		t.Fatalf("route by alias: %#v", byAlias)
	}
	resourceURIs, ok := byAlias["resource_uris"].(map[string]string)
	if !ok || len(resourceURIs) != 4 {
		t.Fatalf("route resource URIs: %#v", byAlias)
	}
	for _, role := range []string{"handoff", "rules", "progress", "pitfalls"} {
		want := "memory://projects/demo/" + role
		if resourceURIs[role] != want {
			t.Fatalf("route URI for %s=%q want %q", role, resourceURIs[role], want)
		}
	}
	if _, exists := resourceURIs["runtime"]; exists {
		t.Fatalf("runtime became a normal role URI: %#v", resourceURIs)
	}
	project := svc.projects["demo"]
	project.Lifecycle = "archived"
	svc.projects["demo"] = project
	archived := svc.Route("demo")
	if archived["status"] != "archived" || archived["project_id"] != "demo" {
		t.Fatalf("archived route: %#v", archived)
	}
	archivedURIs, ok := archived["resource_uris"].(map[string]string)
	if !ok || len(archivedURIs) != 4 || archivedURIs["handoff"] != "memory://projects/demo/handoff" {
		t.Fatalf("archived route resource URIs: %#v", archived)
	}
	project.Lifecycle = "active"
	svc.projects["demo"] = project

	page1 := svc.Read(ReadArgs{URI: "memory://projects/demo/handoff", Heading: "Long Section"})
	if page1["status"] != "ok" || page1["truncated"] != true {
		t.Fatalf("page1: %#v", page1)
	}
	content1 := page1["content"].(string)
	if len([]byte(content1)) > maxPageBytes || !strings.Contains(content1, "chunk-000") {
		t.Fatalf("bad page1")
	}
	rev := page1["revision"].(string)
	if rev != page1["content_hash"] || !strings.HasPrefix(rev, "sha256:") || len(rev) != 71 {
		t.Fatalf("bad revision %q", rev)
	}
	cursor := page1["cursor"].(string)
	page2 := svc.Read(ReadArgs{URI: "memory://projects/demo/handoff", Cursor: cursor})
	if page2["status"] != "ok" || page2["revision"] != rev || strings.Contains(page2["content"].(string), "chunk-000") {
		t.Fatalf("page2: %#v", page2)
	}

	one := 1
	line := svc.Read(ReadArgs{URI: "memory://projects/demo/handoff", LineStart: &one, LineEnd: &one, ExpectedResourceRevision: rev})
	if line["status"] != "ok" || line["line_start"] != 1 || line["line_end"] != 1 || line["cursor"] != nil {
		t.Fatalf("line read: %#v", line)
	}
	conflict := svc.Read(ReadArgs{URI: "memory://projects/demo/handoff", LineStart: &one, LineEnd: &one, ExpectedResourceRevision: "sha256:" + strings.Repeat("0", 64)})
	if conflict["status"] != "error" || conflict["code"] != "conflict" || conflict["current_revision"] != rev || conflict["content"] != nil {
		t.Fatalf("conflict: %#v", conflict)
	}
}

func TestMemoryReadURIErrorExplainsCanonicalGrammar(t *testing.T) {
	root := fixtureRepo(t)
	svc, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	tests := []struct {
		name string
		uri  string
	}{
		{name: "missing projects authority", uri: "memory://gold-calendar/handoff"},
		{name: "unknown role", uri: "memory://projects/demo/unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := svc.Read(ReadArgs{URI: tc.uri})
			message, _ := result["message"].(string)
			if result["status"] != "error" || result["code"] != "read_failed" {
				t.Fatalf("Read(%q): %#v", tc.uri, result)
			}
			if !strings.Contains(message, "memory://projects/{project_id}/{role}") {
				t.Fatalf("error lacks canonical URI format: %q", message)
			}
			if !strings.Contains(message, "handoff, rules, progress, pitfalls") {
				t.Fatalf("error lacks legal roles: %q", message)
			}
		})
	}

	valid := svc.Read(ReadArgs{URI: "memory://projects/demo/handoff"})
	if valid["status"] != "ok" || valid["project_id"] != "demo" || valid["role"] != "handoff" {
		t.Fatalf("canonical URI behavior changed: %#v", valid)
	}
}

func TestEncodedSlashAndSymlinkEscapeRejected(t *testing.T) {
	root := fixtureRepo(t)
	svc, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	bad := svc.Read(ReadArgs{URI: "memory://projects/demo%2Fother/handoff"})
	if bad["status"] != "error" || bad["code"] != "read_failed" {
		t.Fatalf("encoded slash: %#v", bad)
	}

	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside\n"), 0600); err != nil {
		t.Fatal(err)
	}
	role := filepath.Join(root, "demo", "交接.md")
	if err := os.Remove(role); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, role); err != nil {
		t.Fatal(err)
	}
	escaped := svc.Read(ReadArgs{URI: "memory://projects/demo/handoff"})
	if escaped["status"] != "error" || escaped["code"] != "read_failed" {
		t.Fatalf("symlink escape: %#v", escaped)
	}
}

func TestIndexRejectsBareDateAndTraversalProject(t *testing.T) {
	bare := []byte("schema_version: 4\nprojects:\n  demo:\n    description: ok\n    lifecycle: active\n    aliases: []\n    runtime_resource: null\n    last_updated: 2026-08-11\n    last_verified: '2026-08-11'\n")
	if _, err := parseIndex(bare); err == nil {
		t.Fatal("bare YAML date accepted")
	}
	traversal := []byte("schema_version: 4\nprojects:\n  ../demo:\n    description: ok\n    lifecycle: active\n    aliases: []\n    runtime_resource: null\n    last_updated: '2026-08-11'\n    last_verified: '2026-08-11'\n")
	if _, err := parseIndex(traversal); err == nil {
		t.Fatal("traversal project accepted")
	}
}

func TestCoreReadRejectsRuntimePseudoRole(t *testing.T) {
	root := fixtureRepo(t)
	svc, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	result := svc.Read(ReadArgs{URI: "memory://projects/demo/runtime"})
	if result["status"] != "error" || result["code"] != "read_failed" || !strings.Contains(result["message"].(string), "invalid role") {
		t.Fatalf("runtime became addressable as a memory role: %#v", result)
	}
}

func TestVaultReadPackageDoesNotDependOnRuntimeView(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "internal/runtimeview") {
			t.Fatalf("core vaultread package imports runtime subsystem in %s", entry.Name())
		}
	}
}

func fixtureRepo(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(file), "..", "..", "conformance", "fixtures", "route-read-revision")
	dst := filepath.Join(t.TempDir(), "vault")
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	git(t, dst, "init", "-q", "-b", "main")
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
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
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
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
