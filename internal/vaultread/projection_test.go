// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultread

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestProjectionDeleteAndCorruptionRebuildPreserveAuthority(t *testing.T) {
	root := searchFixtureRepo(t)
	dbPath := filepath.Join(t.TempDir(), "projection.sqlite")
	authorityPath := filepath.Join(root, "alpha", "交接.md")
	before, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash := sha256.Sum256(before)

	first := openSearchService(t, root, dbPath)
	result1 := first.Search(SearchArgs{Query: "中文检索词", ProjectID: "alpha"})
	if result1["status"] != "ok" || result1["match_type"] != "fts5_trigram" {
		t.Fatalf("initial search: %#v", result1)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	second := openSearchService(t, root, dbPath)
	result2 := second.Search(SearchArgs{Query: "中文检索词", ProjectID: "alpha"})
	if !reflect.DeepEqual(result1, result2) {
		t.Fatalf("delete/rebuild changed result:\nfirst=%#v\nsecond=%#v", result1, result2)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(dbPath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	third := openSearchService(t, root, dbPath)
	result3 := third.Search(SearchArgs{Query: "中文检索词", ProjectID: "alpha"})
	if !reflect.DeepEqual(result1, result3) {
		t.Fatalf("corruption/rebuild changed result:\nfirst=%#v\nthird=%#v", result1, result3)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatal(err)
	}
	afterHash := sha256.Sum256(after)
	if afterHash != beforeHash {
		t.Fatal("Authority content changed while rebuilding projection")
	}
}

func TestProjectionRebuildsToCurrentGitHead(t *testing.T) {
	root := searchFixtureRepo(t)
	dbPath := filepath.Join(t.TempDir(), "projection.sqlite")

	first := openSearchService(t, root, dbPath)
	oldHead := first.head
	if first.projection.head != oldHead {
		t.Fatalf("initial index head=%s git head=%s", first.projection.head, oldHead)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	rolePath := filepath.Join(root, "alpha", "交接.md")
	file, err := os.OpenFile(rolePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n## Fresh Head\n\nheadadvanceuniquetoken is visible only after the new commit.\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "alpha/交接.md")
	commit := fmt.Sprintf("head-%s", t.Name())
	cmd := gitCommitCommand(root, commit)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}

	second := openSearchService(t, root, dbPath)
	defer second.Close()
	if second.head == oldHead {
		t.Fatal("Git HEAD did not advance")
	}
	if second.projection.head != second.head {
		t.Fatalf("rebuilt index head=%s git head=%s", second.projection.head, second.head)
	}
	result := second.Search(SearchArgs{Query: "headadvanceuniquetoken", ProjectID: "alpha"})
	if result["status"] != "ok" {
		t.Fatalf("search after head advance: %#v", result)
	}
	hits, ok := result["results"].([]map[string]any)
	if !ok || len(hits) != 1 || hits[0]["heading"] != "交接 / Fresh Head" {
		t.Fatalf("new head was not indexed: %#v", result)
	}
}

func openSearchService(t *testing.T, root, dbPath string) *Service {
	t.Helper()
	svc, err := OpenWithOptions(root, OpenOptions{ProjectionPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func searchFixtureRepo(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(file), "..", "..", "conformance", "fixtures", "read-index-search")
	dst := filepath.Join(t.TempDir(), "vault")
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	git(t, dst, "init", "-q", "-b", "main")
	git(t, dst, "add", ".")
	cmd := gitCommitCommand(dst, "seed")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	return dst
}

func gitCommitCommand(dir, message string) *exec.Cmd {
	cmd := exec.Command("git", "-C", dir, "commit", "-q", "-m", message)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
		"GIT_AUTHOR_DATE=2026-08-11T00:00:00Z", "GIT_COMMITTER_DATE=2026-08-11T00:00:00Z")
	return cmd
}
