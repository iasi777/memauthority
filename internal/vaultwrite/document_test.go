// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/iasi777/memauthority/internal/vaultread"
)

func TestProtectedVerificationRoutingIsAtomic(t *testing.T) {
	root := fixtureRepo(t)
	work := t.TempDir()
	writer, err := New(root, Config{WorkRoot: work, WriteEnabled: true, WriteSource: "conformance"})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	beforeRead := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff"})
	_ = reader.Close()
	revision := beforeRead["revision"].(string)
	beforeHead := mustGit(t, root, "rev-parse", "HEAD")

	direct := writer.UpdateHandoff(UpdateHandoffArgs{
		ProjectID: "demo", ExpectedResourceRevision: revision,
		SectionUpdates: []SectionUpdateInput{{Heading: "核验记录", Replacement: "must not write"}},
	})
	if direct["status"] != "error" || direct["code"] != "section_protected" {
		t.Fatalf("direct protected update: %#v", direct)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != beforeHead {
		t.Fatalf("protected UpdateHandoff moved HEAD: %s -> %s", beforeHead, got)
	}

	first := "This replacement must roll back."
	protected := "Protected verification must not be changed here."
	batch := writer.UpdateSections(UpdateSectionsArgs{
		ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: revision,
		Operations: []SectionOperationInput{
			{Operation: "replace", Heading: "Overview", Content: &first},
			{Operation: "replace", Heading: "核验记录", Content: &protected},
		},
	})
	if batch["status"] != "error" || batch["code"] != "section_protected" || batch["all_rolled_back"] != true || batch["failed_operation_index"] != 1 {
		t.Fatalf("protected batch: %#v", batch)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != beforeHead {
		t.Fatalf("protected batch moved HEAD: %s -> %s", beforeHead, got)
	}
	reader, err = vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	afterReject := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff", Heading: "Overview"})
	_ = reader.Close()
	if !strings.Contains(afterReject["content"].(string), "intentionally deterministic") {
		t.Fatalf("batch leaked first candidate operation: %#v", afterReject)
	}

	verified := writer.MarkVerified(MarkVerifiedArgs{
		ProjectID: "demo", ExpectedResourceRevision: revision, VerificationLevel: "local",
		VerifiedScope: []string{"unit routing"}, Basis: []string{"protected-path test"},
		Result: "Dedicated verification path committed.", NotVerified: []string{"production"},
	})
	if verified["status"] != "committed" {
		t.Fatalf("mark verified: %#v", verified)
	}
	reader, err = vaultread.OpenWithOptions(root, vaultread.OpenOptions{ProjectionPath: filepath.Join(work, "read-index.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	verification := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff", Heading: "核验记录"})
	_ = reader.Close()
	if verification["status"] != "ok" || !strings.Contains(verification["content"].(string), "Dedicated verification path committed.") {
		t.Fatalf("verification not visible: %#v", verification)
	}
}

func TestNoopHandoffUpdateDoesNotCreateAuthorityCommit(t *testing.T) {
	root := fixtureRepo(t)
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "unit-noop"})
	if err != nil {
		t.Fatal(err)
	}

	first := writer.UpdateHandoff(UpdateHandoffArgs{
		ProjectID: "demo", ExpectedResourceRevision: roleRevision(t, root, "handoff"),
		SectionUpdates: []SectionUpdateInput{{Heading: "Overview", Replacement: "Stable no-op body."}},
	})
	if first["status"] != "committed" {
		t.Fatalf("first update: %#v", first)
	}

	beforeHead := mustGit(t, root, "rev-parse", "HEAD")
	beforeCount := mustGit(t, root, "rev-list", "--count", "HEAD")
	second := writer.UpdateHandoff(UpdateHandoffArgs{
		ProjectID: "demo", ExpectedResourceRevision: roleRevision(t, root, "handoff"),
		SectionUpdates: []SectionUpdateInput{{Heading: "Overview", Replacement: "Stable no-op body."}},
	})
	if second["status"] != "committed" {
		t.Fatalf("no-op update: %#v", second)
	}
	if got := second["new_commit"]; got != beforeHead {
		t.Fatalf("no-op result commit=%v want existing HEAD %s", got, beforeHead)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != beforeHead {
		t.Fatalf("no-op update moved HEAD: %s -> %s", beforeHead, got)
	}
	if got := mustGit(t, root, "rev-list", "--count", "HEAD"); got != beforeCount {
		t.Fatalf("no-op update created a commit: count %s -> %s", beforeCount, got)
	}
}

func TestRecordPitfallServerRendersHeadingAndMetadata(t *testing.T) {
	root := fixtureRepo(t)
	work := t.TempDir()
	writer, err := New(root, Config{WorkRoot: work, WriteEnabled: true, WriteSource: "conformance"})
	if err != nil {
		t.Fatal(err)
	}
	result := writer.RecordPitfall(RecordPitfallArgs{
		ProjectID: "demo", Title: "Unit pitfall", Severity: "high", Tags: []string{"one", "two"},
		Content: "Body only.", Related: "Unit context", ClientIdempotencyKey: "pitfall-unit",
	})
	if result["status"] != "committed" {
		t.Fatalf("record pitfall: %#v", result)
	}
	reader, err := vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	read := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/pitfalls"})
	_ = reader.Close()
	text := read["content"].(string)
	for _, want := range []string{"## [Unit pitfall] - ", "`severity: high` · `tags: one, two` · `related: Unit context`", "Body only.", "<!-- v-memory-entry:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("pitfall rendering missing %q:\n%s", want, text)
		}
	}
}
