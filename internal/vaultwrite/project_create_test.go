// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iasi777/v-memory/internal/vaultvalidate"
)

func TestCreateProjectNeedsNoVaultTemplate(t *testing.T) {
	root := fixtureRepo(t)
	if err := os.RemoveAll(filepath.Join(root, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "_templates")); err != nil {
		t.Fatal(err)
	}
	raw := []byte("schema_version: 4\nprojects: {}\n")
	if err := os.WriteFile(filepath.Join(root, "INDEX.yaml"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "empty no-template vault")

	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "p4c-slice5"})
	if err != nil {
		t.Fatal(err)
	}
	result := writer.CreateProject(CreateProjectArgs{ProjectID: "first", Description: "first project", Lifecycle: "active", ExpectedIndexRevision: revision(raw)})
	if result["status"] != "committed" {
		t.Fatalf("template-free create failed: %#v", result)
	}
	if err := vaultvalidate.ValidateAuthorityRoot(root); err != nil {
		t.Fatalf("program-owned project is not a valid Authority tree: %v", err)
	}
}
