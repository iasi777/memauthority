// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/iasi777/v-memory/internal/vaultread"
)

func TestAppendProgressTransactionReplayAndMismatch(t *testing.T) {
	root := fixtureRepo(t)
	work := t.TempDir()
	writer, err := New(root, Config{WorkRoot: work, WriteEnabled: true, WriteSource: "conformance"})
	if err != nil {
		t.Fatal(err)
	}
	before := mustGit(t, root, "rev-parse", "HEAD")
	args := AppendProgressArgs{
		ProjectID: "demo", Summary: "Unit append", Content: "Mutation kernel body.", ClientIdempotencyKey: "append-unit",
	}
	result := writer.AppendProgress(args)
	if result["status"] != "committed" || result["idempotent_replay"] != false {
		t.Fatalf("append result: %#v", result)
	}
	commit, _ := result["new_commit"].(string)
	if commit == "" || commit == before {
		t.Fatalf("commit did not advance: before=%s result=%#v", before, result)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != commit {
		t.Fatalf("HEAD=%s commit=%s", got, commit)
	}
	if status := mustGit(t, root, "status", "--porcelain"); status != "" {
		t.Fatalf("worktree not clean: %q", status)
	}

	reader, err := vaultread.OpenWithOptions(root, vaultread.OpenOptions{ProjectionPath: filepath.Join(work, "read-index.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	read := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/progress"})
	if read["status"] != "ok" || !strings.Contains(read["content"].(string), "Mutation kernel body.") || read["source_commit"] != commit {
		_ = reader.Close()
		t.Fatalf("read after append: %#v", read)
	}
	indexHead := reader.IndexHead()
	_ = reader.Close()
	if indexHead != commit {
		t.Fatalf("index head=%s commit=%s", indexHead, commit)
	}
	operationID := result["operation_id"].(string)
	observed := writer.Observe(indexHead, operationID)
	if observed["git_head"] != commit {
		t.Fatalf("observe head: %#v", observed)
	}
	opRecords := observed["operation_records"].(map[string]any)
	if opRecords["count"] != 1 {
		t.Fatalf("operation records: %#v", opRecords)
	}
	journal := observed["journal"].(map[string]any)
	if journal["pending_count"] != 0 {
		t.Fatalf("pending journal: %#v", journal)
	}

	journalPath := filepath.Join(work, "transactions", operationID+".json")
	raw, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	var j journalRecord
	if err := json.Unmarshal(raw, &j); err != nil {
		t.Fatal(err)
	}
	if j.State != "finalized" || j.PreviousCommit != before || j.CandidateCommit != commit {
		t.Fatalf("journal: %#v", j)
	}

	replay := writer.AppendProgress(args)
	if replay["status"] != "committed" || replay["idempotent_replay"] != true || replay["new_commit"] != commit || replay["operation_id"] != operationID {
		t.Fatalf("replay: %#v", replay)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != commit {
		t.Fatalf("replay moved HEAD: %s", got)
	}

	mismatch := args
	mismatch.Content = "different payload"
	rejected := writer.AppendProgress(mismatch)
	if rejected["status"] != "error" || rejected["code"] != "validation_failed" || rejected["rule"] != "idempotency_key" {
		t.Fatalf("mismatch result: %#v", rejected)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != commit {
		t.Fatalf("mismatch moved HEAD: %s", got)
	}
}

func TestWriteDisabledCannotMutate(t *testing.T) {
	root := fixtureRepo(t)
	work := t.TempDir()
	writer, err := New(root, Config{WorkRoot: work, WriteEnabled: false, WriteSource: "conformance"})
	if err != nil {
		t.Fatal(err)
	}
	before := mustGit(t, root, "rev-parse", "HEAD")
	result := writer.AppendProgress(AppendProgressArgs{
		ProjectID: "demo", Summary: "disabled", Content: "must not mutate", ClientIdempotencyKey: "disabled",
	})
	if result["status"] != "error" || result["code"] != "write_disabled" {
		t.Fatalf("disabled result: %#v", result)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != before {
		t.Fatalf("disabled mutation moved HEAD: before=%s after=%s", before, got)
	}
	if _, err := os.Stat(filepath.Join(work, "operations")); !os.IsNotExist(err) {
		t.Fatalf("disabled mutation created operations state: %v", err)
	}
}

func TestRenderProgressMatchesFrozenBlackBoxShape(t *testing.T) {
	raw := []byte("---\n项目: demo\n最后更新: 2026-08-10\n---\n# 进度\n\n## Progress\n\nold\n")
	out, err := renderProgress(raw, "summary", "body", strings.Repeat("a", 64), "2026-08-11")
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"最后更新: 2026-08-11",
		"# 进度\n\n## 2026-08-11\n\n**做了什么**：\n- summary\n\nbody",
		"<!-- v-memory-entry:" + strings.Repeat("a", 64) + " -->",
		"## Progress\n\nold",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered progress missing %q:\n%s", want, text)
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
	mustGit(t, dst, "init", "-q", "-b", "main")
	mustGit(t, dst, "config", "user.name", "V-Memory Test")
	mustGit(t, dst, "config", "user.email", "v-memory-test@example.invalid")
	mustGit(t, dst, "add", "-A")
	cmd := exec.Command("git", "-C", dst, "commit", "-q", "-m", "seed")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE=2026-08-11T00:00:00Z", "GIT_COMMITTER_DATE=2026-08-11T00:00:00Z")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	return dst
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
			return os.MkdirAll(target, info.Mode().Perm())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
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

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
