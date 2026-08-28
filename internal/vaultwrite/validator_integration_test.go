// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmptyVaultWriterStatusAndFirstCreate(t *testing.T) {
	root := fixtureRepo(t)
	if err := os.RemoveAll(filepath.Join(root, "demo")); err != nil {
		t.Fatal(err)
	}
	index := []byte("schema_version: 4\nprojects: {}\n")
	if err := os.WriteFile(filepath.Join(root, "INDEX.yaml"), index, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "_templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	template := []byte("---\n项目: <项目名>\n创建: YYYY-MM-DD\n最后更新: YYYY-MM-DD\n最后核验: 未核验\n说明: handoff\n---\n# 交接 - <项目名>\n\n## 核验记录\n\n- 核验于: 未核验\n")
	if err := os.WriteFile(filepath.Join(root, "_templates", "交接.md"), template, 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "empty schema4 vault")

	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice2"})
	if err != nil {
		t.Fatalf("writer open empty vault: %v", err)
	}
	status := writer.Status("")
	if status["index_state"] != "current" {
		t.Fatalf("status: %#v", status)
	}
	before, err := os.ReadFile(filepath.Join(root, "INDEX.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result := writer.CreateProject(CreateProjectArgs{ProjectID: "first-project", Description: "first", Lifecycle: "active", ExpectedIndexRevision: revision(before)})
	if result["status"] != "committed" {
		t.Fatalf("first create: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "first-project", "交接.md")); err != nil {
		t.Fatalf("handoff missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "first-project", "规范.md")); !os.IsNotExist(err) {
		t.Fatalf("unexpected optional rules: %v", err)
	}
}

func TestSchema4EmptyAlreadyMigrated(t *testing.T) {
	root := fixtureRepo(t)
	if err := os.RemoveAll(filepath.Join(root, "demo")); err != nil {
		t.Fatal(err)
	}
	raw := []byte("schema_version: 4\nprojects: {}\n")
	if err := os.WriteFile(filepath.Join(root, "INDEX.yaml"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "empty schema4")
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice2"})
	if err != nil {
		t.Fatal(err)
	}
	result := writer.MigrateLifecycle(MigrateLifecycleArgs{Manifest: LifecycleMigrationManifestInput{SchemaVersion: 1, ExpectedIndexRevision: revision(raw), ArchiveReason: "not used", Projects: []LifecycleMigrationProjectInput{{ProjectID: "placeholder", SourceLifecycle: "active", TargetLifecycle: "active"}}}})
	if result["status"] != "already_migrated" {
		t.Fatalf("empty schema4 migration state: %#v", result)
	}
}

func TestTransactionCandidateUsesSharedValidator(t *testing.T) {
	root := fixtureRepo(t)
	rules := filepath.Join(root, "demo", "规范.md")
	raw, err := os.ReadFile(rules)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "# 规范\n", "# Notes\n", 1))
	if err := os.WriteFile(rules, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "demo/规范.md")
	mustGit(t, root, "commit", "-q", "-m", "seed structurally invalid role")
	before := mustGit(t, root, "rev-parse", "HEAD")
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice2"})
	if err != nil {
		t.Fatal(err)
	}
	result := writer.AppendProgress(AppendProgressArgs{ProjectID: "demo", Summary: "must reject", Content: "candidate shares validator"})
	if result["status"] != "error" || result["code"] != "validation_failed" || result["rule"] != "vault" {
		t.Fatalf("shared candidate validator not enforced: %#v", result)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != before {
		t.Fatalf("rejected candidate moved HEAD: %s -> %s", before, got)
	}
}
