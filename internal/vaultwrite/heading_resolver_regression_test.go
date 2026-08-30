// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iasi777/v-memory/internal/vaultread"
)

func TestHeadingResolverPreservesLiteralSlashH2(t *testing.T) {
	const doc = "# 交接\n\n## 已知问题 / 待办\nold body\n\n## 当前状态\ncurrent\n"

	updated, err := replaceH2Body(doc, "已知问题 / 待办", "new body")
	if err != nil {
		t.Fatalf("literal slash H2 must resolve as an opaque title: %v", err)
	}
	if !strings.Contains(updated, "## 已知问题 / 待办\nnew body") {
		t.Fatalf("literal slash H2 was not updated:\n%s", updated)
	}
}

func TestHeadingResolverAcceptsRealRootPathWithoutCollapsingLiteralSlash(t *testing.T) {
	const doc = "# 交接\n\n## 已知问题 / 待办\nold body\n\n## 当前状态\ncurrent\n"

	updated, err := replaceH2Body(doc, "交接 / 已知问题 / 待办", "new body")
	if err != nil {
		t.Fatalf("real H1/H2 path must resolve: %v", err)
	}
	if !strings.Contains(updated, "## 已知问题 / 待办\nnew body") {
		t.Fatalf("real path did not update the slash-title H2:\n%s", updated)
	}
}

func TestUpdateHandoffLiteralSlashHeading(t *testing.T) {
	root := fixtureRepo(t)
	handoff := filepath.Join(root, "demo", "交接.md")
	raw, err := os.ReadFile(handoff)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\n## 已知问题 / 待办\n\nold body\n")...)
	if err := os.WriteFile(handoff, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "add slash heading")

	reader, err := vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	before := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff"})
	_ = reader.Close()

	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "heading-resolver-regression"})
	if err != nil {
		t.Fatal(err)
	}
	result := writer.UpdateHandoff(UpdateHandoffArgs{
		ProjectID: "demo", ExpectedResourceRevision: before["revision"].(string),
		SectionUpdates: []SectionUpdateInput{{Heading: "已知问题 / 待办", Replacement: "new body"}},
	})
	if result["status"] != "committed" {
		t.Fatalf("literal slash UpdateHandoff failed: %#v", result)
	}

	reader, err = vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	after := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff", Heading: "已知问题 / 待办"})
	_ = reader.Close()
	if after["status"] != "ok" || !strings.Contains(after["content"].(string), "new body") {
		t.Fatalf("updated slash heading not readable: %#v", after)
	}
}

func TestFullPathStillProtectsVerificationSection(t *testing.T) {
	root := fixtureRepo(t)
	reader, err := vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	before := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff"})
	_ = reader.Close()

	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "heading-resolver-regression"})
	if err != nil {
		t.Fatal(err)
	}
	result := writer.UpdateHandoff(UpdateHandoffArgs{
		ProjectID: "demo", ExpectedResourceRevision: before["revision"].(string),
		SectionUpdates: []SectionUpdateInput{{Heading: "交接 / 核验记录", Replacement: "must remain protected"}},
	})
	if result["status"] != "error" || result["code"] != "section_protected" {
		t.Fatalf("full-path protected section was not rejected: %#v", result)
	}
}
