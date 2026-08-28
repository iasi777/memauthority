// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultread

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchReportsHardLimitTruncation(t *testing.T) {
	root := fixtureRepo(t)
	path := filepath.Join(root, "demo", "进度.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.Write(raw)
	for i := 0; i < 60; i++ {
		b.WriteString("\n## Slice5 Entry ")
		b.WriteString(fmt.Sprintf("%02d", i))
		b.WriteString("\n\nslice5searchtoken\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "demo/进度.md")
	cmd := exec.Command("git", "-C", root, "commit", "-q", "-m", "many search sections")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	svc, err := OpenWithOptions(root, OpenOptions{ProjectionPath: filepath.Join(t.TempDir(), "search.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	result := svc.Search(SearchArgs{Query: "slice5searchtoken", ProjectID: "demo"})
	if result["status"] != "ok" {
		t.Fatalf("search: %#v", result)
	}
	rows := result["results"].([]map[string]any)
	if len(rows) != 50 {
		t.Fatalf("rows=%d want 50", len(rows))
	}
	if result["truncated"] != true {
		t.Fatalf("truncated=%#v want true", result["truncated"])
	}
}
