// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestServiceCommitIdentityIgnoresAmbientGitConfig(t *testing.T) {
	root := fixtureRepo(t)
	mustGit(t, root, "config", "--unset-all", "user.name")
	mustGit(t, root, "config", "--unset-all", "user.email")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "")
	t.Setenv("GIT_AUTHOR_EMAIL", "")
	t.Setenv("GIT_COMMITTER_NAME", "")
	t.Setenv("GIT_COMMITTER_EMAIL", "")

	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice1"})
	if err != nil {
		t.Fatal(err)
	}
	result := writer.AppendProgress(AppendProgressArgs{ProjectID: "demo", Summary: "identity", Content: "service owns commit identity"})
	if result["status"] != "committed" {
		t.Fatalf("mutation depends on ambient Git identity: %#v", result)
	}
	commit := result["new_commit"].(string)
	got := mustGit(t, root, "show", "-s", "--format=%an <%ae>|%cn <%ce>", commit)
	want := "V-Memory MCP <v-memory@v-memory.invalid>|V-Memory MCP <v-memory@v-memory.invalid>"
	if got != want {
		t.Fatalf("commit identity=%q want %q", got, want)
	}
}

func TestAppendProgressSuffixH1CanonicalTextAndLocalDate(t *testing.T) {
	root := fixtureRepo(t)
	path := filepath.Join(root, "demo", "进度.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(raw), "# 进度\n", "# 进度 - Display Name\n", 1)
	text += "Cafe\u0301   \n"
	text = strings.ReplaceAll(text, "\n", "\r\n")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "demo/进度.md")
	mustGit(t, root, "commit", "-q", "-m", "seed suffix h1 and noncanonical text")

	seoul := time.FixedZone("Asia/Seoul", 9*60*60)
	now := time.Date(2026, 8, 13, 0, 30, 0, 0, seoul) // 2026-08-12 15:30Z
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice1", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	result := writer.AppendProgress(AppendProgressArgs{ProjectID: "demo", Summary: "local day", Content: "NFC body Cafe\u0301   "})
	if result["status"] != "committed" {
		t.Fatalf("append suffix H1: %#v", result)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("\r")) {
		t.Fatalf("managed output retained CR bytes")
	}
	if bytes.Contains(out, []byte("Cafe\xcc\x81")) || !bytes.Contains(out, []byte("Caf\xc3\xa9")) {
		t.Fatalf("managed output is not NFC: %q", out)
	}
	for i, line := range strings.Split(string(out), "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Fatalf("line %d retains trailing ASCII whitespace: %q", i+1, line)
		}
	}
	if !strings.Contains(string(out), "# 进度 - Display Name\n\n## 2026-08-13") {
		t.Fatalf("suffix H1/local date not preserved: %s", out)
	}
}

func TestMarkVerifiedSynchronizesIndexLastVerified(t *testing.T) {
	root := fixtureRepo(t)
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.FixedZone("Asia/Seoul", 9*60*60))
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice1", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	result := writer.MarkVerified(MarkVerifiedArgs{
		ProjectID: "demo", ExpectedResourceRevision: roleRevision(t, root, "handoff"), VerificationLevel: "local",
		VerifiedScope: []string{"slice1"}, Basis: []string{"test"}, Result: "pass",
	})
	if result["status"] != "committed" {
		t.Fatalf("mark verified: %#v", result)
	}
	raw, err := os.ReadFile(filepath.Join(root, "INDEX.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc lifecycleIndex
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.Projects["demo"].LastVerified; got != "2026-08-13" {
		t.Fatalf("INDEX last_verified=%q want 2026-08-13", got)
	}
}

func TestMissingProgressMaterializesAndNoKeyDuplicateRejects(t *testing.T) {
	root := fixtureRepo(t)
	if err := os.Remove(filepath.Join(root, "demo", "进度.md")); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "remove optional progress")
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice1"})
	if err != nil {
		t.Fatal(err)
	}
	args := AppendProgressArgs{ProjectID: "demo", Summary: "materialize", Content: "same canonical progress body"}
	first := writer.AppendProgress(args)
	if first["status"] != "committed" {
		t.Fatalf("materialize progress: %#v", first)
	}
	raw, err := os.ReadFile(filepath.Join(root, "demo", "进度.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# 进度 - demo") {
		t.Fatalf("default progress shape: %s", raw)
	}
	head := mustGit(t, root, "rev-parse", "HEAD")
	second := writer.AppendProgress(args)
	if second["status"] != "error" || second["code"] != "validation_failed" || second["rule"] != "duplicate_entry" {
		t.Fatalf("duplicate progress: %#v", second)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != head {
		t.Fatalf("duplicate moved HEAD: %s -> %s", head, got)
	}
}

func TestMissingPitfallMaterializesAndNoKeyDuplicateRejects(t *testing.T) {
	root := fixtureRepo(t)
	if err := os.Remove(filepath.Join(root, "demo", "踩坑.md")); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "remove optional pitfalls")
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice1"})
	if err != nil {
		t.Fatal(err)
	}
	args := RecordPitfallArgs{ProjectID: "demo", Title: "materialize", Severity: "medium", Tags: []string{"slice1"}, Content: "same canonical pitfall body"}
	first := writer.RecordPitfall(args)
	if first["status"] != "committed" {
		t.Fatalf("materialize pitfall: %#v", first)
	}
	raw, err := os.ReadFile(filepath.Join(root, "demo", "踩坑.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# 踩坑 - demo") {
		t.Fatalf("default pitfall shape: %s", raw)
	}
	head := mustGit(t, root, "rev-parse", "HEAD")
	second := writer.RecordPitfall(args)
	if second["status"] != "error" || second["code"] != "validation_failed" || second["rule"] != "duplicate_entry" {
		t.Fatalf("duplicate pitfall: %#v", second)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != head {
		t.Fatalf("duplicate moved HEAD: %s -> %s", head, got)
	}
}

func TestUpdateSectionsMissingRoleUsesRoleSpecificError(t *testing.T) {
	root := fixtureRepo(t)
	if err := os.Remove(filepath.Join(root, "demo", "规范.md")); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "remove optional rules")
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice1"})
	if err != nil {
		t.Fatal(err)
	}
	body := "replacement"
	result := writer.UpdateSections(UpdateSectionsArgs{
		ProjectID: "demo", Role: "rules", ExpectedResourceRevision: "sha256:" + strings.Repeat("0", 64),
		Operations: []SectionOperationInput{{Operation: "replace", Heading: "Rule", Content: &body}},
	})
	if result["status"] != "error" || result["code"] != "validation_failed" || result["rule"] != "rules" {
		t.Fatalf("missing-role error: %#v", result)
	}
}

func TestLifecycleIndexRevisionUsesCanonicalCommittedBytes(t *testing.T) {
	root := fixtureRepo(t)
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice1"})
	if err != nil {
		t.Fatal(err)
	}
	status := writer.Status("")
	current, _ := status["index_revision"].(string)
	result := writer.ArchiveProject(ArchiveProjectArgs{
		ProjectID: "demo", Reason: "Cafe\u0301 archive", ExpectedIndexRevision: current,
	})
	if result["status"] != "committed" {
		t.Fatalf("archive: %#v", result)
	}
	raw, err := os.ReadFile(filepath.Join(root, "INDEX.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if result["new_revision"] != revision(raw) {
		t.Fatalf("returned revision=%v actual=%s", result["new_revision"], revision(raw))
	}
	if bytes.Contains(raw, []byte("Cafe\xcc\x81")) || !bytes.Contains(raw, []byte("Caf\xc3\xa9")) {
		t.Fatalf("INDEX is not NFC: %q", raw)
	}
}
