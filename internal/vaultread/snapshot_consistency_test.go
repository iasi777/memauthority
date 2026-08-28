// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultread

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadAndSearchRemainBoundToOpenedCommit(t *testing.T) {
	root := p4cSlice4Vault(t)
	db := filepath.Join(t.TempDir(), "owned.sqlite")
	ownedBefore := p4cSlice4GitOut(t, root, "rev-parse", "HEAD")
	svc, err := OpenWithOptions(root, OpenOptions{ProjectionPath: db, SourceCommit: ownedBefore})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	owned := svc.SourceCommit()
	before := svc.Read(ReadArgs{URI: "memory://projects/demo/progress"})
	if before["status"] != "ok" || before["source_commit"] != owned {
		t.Fatalf("before=%#v", before)
	}
	if !strings.Contains(before["content"].(string), "Reference freeze fixture") {
		t.Fatalf("baseline content=%#v", before)
	}

	path := filepath.Join(root, "demo", "进度.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, []byte("\nDIRTY-SNAPSHOT-TOKEN\n")...), 0644); err != nil {
		t.Fatal(err)
	}

	dirtyRead := svc.Read(ReadArgs{URI: "memory://projects/demo/progress"})
	if dirtyRead["status"] != "ok" || dirtyRead["source_commit"] != owned {
		t.Fatalf("dirty read=%#v", dirtyRead)
	}
	if strings.Contains(dirtyRead["content"].(string), "DIRTY-SNAPSHOT-TOKEN") {
		t.Fatalf("dirty worktree leaked into owned read: %#v", dirtyRead)
	}
	dirtySearch := svc.Search(SearchArgs{Query: "DIRTY-SNAPSHOT-TOKEN", CrossProject: true})
	if dirtySearch["status"] != "ok" || dirtySearch["index_head"] != owned {
		t.Fatalf("dirty search=%#v", dirtySearch)
	}
	if len(dirtySearch["results"].([]map[string]any)) != 0 {
		t.Fatalf("dirty worktree leaked into search: %#v", dirtySearch)
	}

	p4cSlice4Git(t, root, "add", "demo/进度.md")
	p4cSlice4Commit(t, root, "external dirty token")
	moved := p4cSlice4GitOut(t, root, "rev-parse", "HEAD")
	if moved == owned {
		t.Fatal("external commit did not move HEAD")
	}
	movedRead := svc.Read(ReadArgs{URI: "memory://projects/demo/progress"})
	if movedRead["source_commit"] != owned || strings.Contains(movedRead["content"].(string), "DIRTY-SNAPSHOT-TOKEN") {
		t.Fatalf("external HEAD silently adopted: %#v", movedRead)
	}
}

func p4cSlice4Vault(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(file), "..", "..", "conformance", "fixtures", "runtime-read")
	dst := filepath.Join(t.TempDir(), "vault")
	if err := p4cSlice4Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	p4cSlice4Git(t, dst, "init", "-q", "-b", "main")
	p4cSlice4Git(t, dst, "add", ".")
	p4cSlice4Commit(t, dst, "seed")
	return dst
}

func p4cSlice4Commit(t *testing.T, root, message string) {
	t.Helper()
	cmd := exec.Command("git", "-C", root, "commit", "-q", "-m", message)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
}
func p4cSlice4Git(t *testing.T, root string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
func p4cSlice4GitOut(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
func p4cSlice4Copy(src, dst string) error {
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
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode().Perm())
	})
}
