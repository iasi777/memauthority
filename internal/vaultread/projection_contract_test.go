// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultread

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEmptyVaultReaderProjectionRouteSearch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "INDEX.yaml"), []byte("schema_version: 4\nprojects: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "add", "INDEX.yaml")
	cmd := exec.Command("git", "-C", root, "commit", "-q", "-m", "empty vault")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}

	svc, err := OpenWithOptions(root, OpenOptions{ProjectionPath: filepath.Join(t.TempDir(), "read-index.sqlite"), ForceRebuild: true})
	if err != nil {
		t.Fatalf("open empty vault: %v", err)
	}
	defer svc.Close()
	if got := len(svc.Projects()); got != 0 {
		t.Fatalf("projects=%d want 0", got)
	}
	route := svc.Route("anything")
	if route["status"] != "not_found" {
		t.Fatalf("route: %#v", route)
	}
	search := svc.Search(SearchArgs{Query: "anything", CrossProject: true})
	if search["status"] != "ok" {
		t.Fatalf("search: %#v", search)
	}
	if got := len(search["results"].([]map[string]any)); got != 0 {
		t.Fatalf("search results=%d", got)
	}
	if svc.IndexHead() != svc.SourceCommit() {
		t.Fatalf("projection head=%q source=%q", svc.IndexHead(), svc.SourceCommit())
	}
}

func TestPortableProjectIDAndCasefoldCollision(t *testing.T) {
	good := []byte("schema_version: 4\nprojects:\n  AgentDock:\n    description: ok\n    lifecycle: active\n    aliases: []\n    runtime_resource: null\n    last_updated: '2026-08-11'\n    last_verified: '未核验'\n")
	if _, err := parseIndex(good); err != nil {
		t.Fatalf("portable existing ID rejected: %v", err)
	}
	bad := []string{"_private", "project.name", "项目", "projects", "CON"}
	for _, id := range bad {
		raw := []byte("schema_version: 4\nprojects:\n  " + id + ":\n    description: ok\n    lifecycle: active\n    aliases: []\n    runtime_resource: null\n    last_updated: '2026-08-11'\n    last_verified: '未核验'\n")
		if _, err := parseIndex(raw); err == nil {
			t.Fatalf("invalid project ID %q accepted", id)
		}
	}
	collision := []byte("schema_version: 4\nprojects:\n  AgentDock:\n    description: one\n    lifecycle: active\n    aliases: []\n    runtime_resource: null\n    last_updated: '2026-08-11'\n    last_verified: '未核验'\n  agentdock:\n    description: two\n    lifecycle: active\n    aliases: []\n    runtime_resource: null\n    last_updated: '2026-08-11'\n    last_verified: '未核验'\n")
	if _, err := parseIndex(collision); err == nil {
		t.Fatal("ASCII-casefold project collision accepted")
	}
}
