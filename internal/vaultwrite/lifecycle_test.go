// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLifecycleCreateArchiveRestorePreservesAuthorityContent(t *testing.T) {
	root := lifecycleFixtureRepo(t, "lifecycle-mutation")
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "unit-lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	indexRaw, err := os.ReadFile(filepath.Join(root, "INDEX.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	created := writer.CreateProject(CreateProjectArgs{ProjectID: "alpha", Description: "Lifecycle unit", Lifecycle: "active", ExpectedIndexRevision: revision(indexRaw), Aliases: []string{"alpha-alias"}, CreateRulesRole: true, ClientIdempotencyKey: "create-alpha"})
	if created["status"] != "committed" {
		t.Fatalf("create: %#v", created)
	}
	handoffPath := filepath.Join(root, "alpha", "交接.md")
	handoff, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(handoff), "项目: alpha") {
		t.Fatalf("handoff: %s", handoff)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha", "规范.md")); err != nil {
		t.Fatal(err)
	}

	indexRaw, _ = os.ReadFile(filepath.Join(root, "INDEX.yaml"))
	archived := writer.ArchiveProject(ArchiveProjectArgs{ProjectID: "alpha", Reason: "unit archive", ExpectedIndexRevision: revision(indexRaw), ClientIdempotencyKey: "archive-alpha"})
	if archived["status"] != "committed" {
		t.Fatalf("archive: %#v", archived)
	}
	if got, _ := os.ReadFile(handoffPath); string(got) != string(handoff) {
		t.Fatal("archive changed project content")
	}

	indexRaw, _ = os.ReadFile(filepath.Join(root, "INDEX.yaml"))
	restored := writer.RestoreProject(RestoreProjectArgs{ProjectID: "alpha", ExpectedIndexRevision: revision(indexRaw), ClientIdempotencyKey: "restore-alpha"})
	if restored["status"] != "committed" || restored["runtime_status"] != "runtime_missing" {
		t.Fatalf("restore: %#v", restored)
	}
	if got, _ := os.ReadFile(handoffPath); string(got) != string(handoff) {
		t.Fatal("restore changed project content")
	}
	if status := mustGit(t, root, "status", "--porcelain"); status != "" {
		t.Fatalf("dirty lifecycle worktree: %q", status)
	}
	if observed := writer.Observe("", ""); observed["operation_records"].(map[string]any)["count"] != 3 {
		t.Fatalf("operations: %#v", observed)
	}
}

func TestLegacyLifecycleMigrationIsOneTransaction(t *testing.T) {
	root := lifecycleFixtureRepo(t, "lifecycle-migration")
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "unit-migration"})
	if err != nil {
		t.Fatal(err)
	}
	if !writer.LegacyLifecycleReady() {
		t.Fatal("legacy migration mode not detected")
	}
	legacy, err := os.ReadFile(filepath.Join(root, "INDEX.md"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := LifecycleMigrationManifestInput{SchemaVersion: 1, ExpectedIndexRevision: revision(legacy), ArchiveReason: "unit migration archive", Projects: []LifecycleMigrationProjectInput{{ProjectID: "demo", SourceLifecycle: "active", TargetLifecycle: "active"}, {ProjectID: "other", SourceLifecycle: "dormant", TargetLifecycle: "archived"}}}
	before := mustGit(t, root, "rev-parse", "HEAD")
	result := writer.MigrateLifecycle(MigrateLifecycleArgs{Manifest: manifest, ClientIdempotencyKey: "migrate-once"})
	if result["status"] != "committed" {
		t.Fatalf("migrate: %#v", result)
	}
	if result["new_commit"] == before {
		t.Fatal("migration did not advance HEAD")
	}
	if _, err := os.Stat(filepath.Join(root, "INDEX.md")); !os.IsNotExist(err) {
		t.Fatalf("legacy INDEX.md still current: %v", err)
	}
	for _, path := range []string{"INDEX.yaml", "migrations/lifecycle-v1.yaml", "migrations/lifecycle-v1-source-INDEX.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
	replay := writer.MigrateLifecycle(MigrateLifecycleArgs{Manifest: manifest, ClientIdempotencyKey: "migrate-once"})
	if replay["status"] != "already_migrated" || replay["idempotent_replay"] != true {
		t.Fatalf("migration replay: %#v", replay)
	}
	if status := mustGit(t, root, "status", "--porcelain"); status != "" {
		t.Fatalf("dirty migrated worktree: %q", status)
	}
	if observed := writer.Observe("", ""); observed["operation_records"].(map[string]any)["count"] != 1 {
		t.Fatalf("migration operations: %#v", observed)
	}
}

func lifecycleFixtureRepo(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(file), "..", "..", "conformance", "fixtures", name)
	dst := filepath.Join(t.TempDir(), "vault")
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dst, "init", "-q", "-b", "main")
	mustGit(t, dst, "config", "user.name", "V-Memory Test")
	mustGit(t, dst, "config", "user.email", "v-memory-test@example.invalid")
	mustGit(t, dst, "add", "-A")
	mustGit(t, dst, "commit", "-q", "-m", "seed")
	return dst
}
