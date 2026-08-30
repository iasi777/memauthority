// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package publiccli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iasi777/v-memory/internal/vaultvalidate"
)

func TestInitCreatesMinimalVaultWithoutAmbientGitIdentity(t *testing.T) {
	t.Setenv("GIT_AUTHOR_NAME", "Ambient Wrong")
	t.Setenv("GIT_AUTHOR_EMAIL", "wrong@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Ambient Wrong")
	t.Setenv("GIT_COMMITTER_EMAIL", "wrong@example.invalid")
	target := filepath.Join(t.TempDir(), "memory")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", target}, &out, &errOut); code != 0 {
		t.Fatalf("init code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Name())
	}
	want := []string{".git", ".gitattributes", "INDEX.yaml"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("root entries=%v want=%v", got, want)
	}
	if raw, _ := os.ReadFile(filepath.Join(target, "INDEX.yaml")); string(raw) != "schema_version: 4\nprojects: {}\n" {
		t.Fatalf("INDEX.yaml=%q", raw)
	}
	if raw, _ := os.ReadFile(filepath.Join(target, ".gitattributes")); string(raw) != "*.md text eol=lf\n*.yaml text eol=lf\n" {
		t.Fatalf(".gitattributes=%q", raw)
	}
	if report := vaultvalidate.Validate(target); !report.Valid {
		t.Fatalf("init vault invalid: %#v", report.Errors)
	}
	if got := gitOutput(t, target, "symbolic-ref", "--short", "HEAD"); got != "main" {
		t.Fatalf("branch=%q", got)
	}
	if got := gitOutput(t, target, "status", "--porcelain=v1"); got != "" {
		t.Fatalf("dirty=%q", got)
	}
	if got := gitOutput(t, target, "show", "-s", "--format=%an <%ae>|%cn <%ce>|%s", "HEAD"); got != "V-Memory MCP <v-memory@v-memory.invalid>|V-Memory MCP <v-memory@v-memory.invalid>|v-memory: initialize empty Vault" {
		t.Fatalf("commit identity/message=%q", got)
	}
}

func TestInitIsIdempotent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "memory")
	var firstOut, firstErr bytes.Buffer
	if code := Run([]string{"init", target}, &firstOut, &firstErr); code != 0 {
		t.Fatalf("first init=%d %s", code, firstErr.String())
	}
	head := gitOutput(t, target, "rev-parse", "HEAD")
	var secondOut, secondErr bytes.Buffer
	if code := Run([]string{"init", target}, &secondOut, &secondErr); code != 0 {
		t.Fatalf("second init=%d stdout=%q stderr=%q", code, secondOut.String(), secondErr.String())
	}
	if got := gitOutput(t, target, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD changed %s -> %s", head, got)
	}
	if got := gitOutput(t, target, "status", "--porcelain=v1"); got != "" {
		t.Fatalf("dirty after idempotent init=%q", got)
	}
	if !strings.Contains(secondOut.String(), "already initialized") {
		t.Fatalf("second stdout=%q", secondOut.String())
	}
}

func TestInitRefusesUnknownNonEmptyAndNestedGit(t *testing.T) {
	parent := t.TempDir()
	unknown := filepath.Join(parent, "unknown")
	if err := os.MkdirAll(unknown, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(unknown, "README.md")
	if err := os.WriteFile(sentinel, []byte("keep-me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", unknown}, &out, &errOut); code != 1 {
		t.Fatalf("unknown code=%d stderr=%q", code, errOut.String())
	}
	if raw, _ := os.ReadFile(sentinel); string(raw) != "keep-me\n" {
		t.Fatalf("sentinel changed=%q", raw)
	}
	if _, err := os.Stat(filepath.Join(unknown, ".git")); !os.IsNotExist(err) {
		t.Fatalf("unexpected .git: %v", err)
	}

	repo := filepath.Join(parent, "outer")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "init", "-q", "-b", "main")
	nested := filepath.Join(repo, "memory")
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"init", nested}, &out, &errOut); code != 1 {
		t.Fatalf("nested code=%d stderr=%q", code, errOut.String())
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("nested target unexpectedly created: %v", err)
	}

	repoSub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(repoSub, 0o755); err != nil {
		t.Fatal(err)
	}
	linkParent := filepath.Join(parent, "repo-sub-link")
	if err := os.Symlink(repoSub, linkParent); err != nil {
		t.Fatal(err)
	}
	nestedViaLink := filepath.Join(linkParent, "memory")
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"init", nestedViaLink}, &out, &errOut); code != 1 {
		t.Fatalf("symlink nested code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if _, err := os.Stat(filepath.Join(repoSub, "memory")); !os.IsNotExist(err) {
		t.Fatalf("symlink nested target unexpectedly created inside worktree: %v", err)
	}
}

func TestValidateDirtyDetachedCandidateHumanAndJSON(t *testing.T) {
	target := filepath.Join(t.TempDir(), "memory")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", target}, &out, &errOut); code != 0 {
		t.Fatalf("init=%d %s", code, errOut.String())
	}
	if err := os.MkdirAll(filepath.Join(target, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	index := "schema_version: 4\nprojects:\n  demo:\n    description: demo\n    lifecycle: active\n    aliases: []\n    runtime_resource: null\n    last_updated: \"2026-08-13\"\n    last_verified: \"未核验\"\n"
	handoff := "---\n项目: demo\n创建: 2026-08-13\n最后更新: 2026-08-13\n最后核验: 未核验\n说明: demo handoff\n---\n# 交接 - Demo\n"
	if err := os.WriteFile(filepath.Join(target, "INDEX.yaml"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "demo", "交接.md"), []byte(handoff), 0o644); err != nil {
		t.Fatal(err)
	}
	statusBefore := gitOutput(t, target, "status", "--porcelain=v1")
	if statusBefore == "" {
		t.Fatal("fixture should be dirty")
	}
	headBefore := gitOutput(t, target, "rev-parse", "HEAD")
	indexBefore, _ := os.ReadFile(filepath.Join(target, "INDEX.yaml"))
	handoffBefore, _ := os.ReadFile(filepath.Join(target, "demo", "交接.md"))

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"validate", target}, &out, &errOut); code != 0 {
		t.Fatalf("human validate=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if strings.TrimSpace(out.String()) != "VALID" {
		t.Fatalf("human output=%q", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"validate", "--json", target}, &out, &errOut); code != 0 {
		t.Fatalf("json validate=%d stderr=%q", code, errOut.String())
	}
	var report vaultvalidate.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json=%q err=%v", out.String(), err)
	}
	if !report.Valid || len(report.Errors) != 0 {
		t.Fatalf("report=%#v", report)
	}
	if got := gitOutput(t, target, "rev-parse", "HEAD"); got != headBefore {
		t.Fatalf("validate moved HEAD %s -> %s", headBefore, got)
	}
	if got := gitOutput(t, target, "status", "--porcelain=v1"); got != statusBefore {
		t.Fatalf("validate changed dirty state %q -> %q", statusBefore, got)
	}
	if raw, _ := os.ReadFile(filepath.Join(target, "INDEX.yaml")); !bytes.Equal(raw, indexBefore) {
		t.Fatal("validate modified INDEX.yaml")
	}
	if raw, _ := os.ReadFile(filepath.Join(target, "demo", "交接.md")); !bytes.Equal(raw, handoffBefore) {
		t.Fatal("validate modified handoff")
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"validate", "--fix", target}, &out, &errOut); code != 2 {
		t.Fatalf("--fix code=%d", code)
	}
}

func TestValidateInvalidAndRequiresGitRootHead(t *testing.T) {
	target := filepath.Join(t.TempDir(), "memory")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", target}, &out, &errOut); code != 0 {
		t.Fatalf("init=%d %s", code, errOut.String())
	}
	index := "schema_version: 4\nprojects:\n  demo:\n    description: demo\n    lifecycle: active\n    aliases: []\n    runtime_resource: null\n    last_updated: \"2026-08-13\"\n    last_verified: \"未核验\"\n"
	if err := os.WriteFile(filepath.Join(target, "INDEX.yaml"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"validate", "--json", target}, &out, &errOut); code != 1 {
		t.Fatalf("invalid code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	var report vaultvalidate.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Valid || !hasCode(report.Errors, "role_required") {
		t.Fatalf("report=%#v", report)
	}

	plain := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plain, "INDEX.yaml"), []byte("schema_version: 4\nprojects: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"validate", "--json", plain}, &out, &errOut); code != 1 {
		t.Fatalf("non-git code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !hasCode(report.Errors, "git_repository") {
		t.Fatalf("non-git report=%#v", report)
	}
}

func hasCode(findings []vaultvalidate.Finding, code string) bool {
	for _, f := range findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func gitRun(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
