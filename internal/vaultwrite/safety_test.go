// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrimaryMismatchFencesWritesWithoutStateMovement(t *testing.T) {
	root := fixtureRepo(t)
	if err := os.WriteFile(filepath.Join(root, "PRIMARY"), []byte("node_id: node-a\nepoch: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "PRIMARY")
	mustGit(t, root, "commit", "-q", "-m", "primary")
	writer, err := New(root, Config{
		WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "conformance",
		RequirePrimary: true, PrimaryEpoch: 1, LocalNodeID: "node-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	before := mustGit(t, root, "rev-parse", "HEAD")
	status := writer.Status("")
	if status["read_only"] != true || status["write_mode"] != "degraded_read_only" || status["primary_active"] != false {
		t.Fatalf("fenced status: %#v", status)
	}
	primary := status["primary"].(map[string]any)
	if primary["state"] != "mismatch" || primary["node_id"] != "node-a" || primary["local_node_id"] != "node-b" {
		t.Fatalf("primary status: %#v", primary)
	}
	result := writer.AppendProgress(AppendProgressArgs{ProjectID: "demo", Summary: "fenced", Content: "must not commit"})
	if result["status"] != "error" || result["code"] != "read_only" {
		t.Fatalf("fenced mutation: %#v", result)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != before {
		t.Fatalf("fenced mutation moved HEAD: before=%s after=%s", before, got)
	}
	observed := writer.Observe("", "")
	if observed["operation_records"].(map[string]any)["count"] != 0 || observed["journal"].(map[string]any)["pending_count"] != 0 {
		t.Fatalf("fenced mutation created state: %#v", observed)
	}
}

func TestExternalRefMovementWinsAndStaleIndexBlocksRetry(t *testing.T) {
	root := fixtureRepo(t)
	work := t.TempDir()
	writer, err := New(root, Config{
		WorkRoot: work, WriteEnabled: true, WriteSource: "conformance",
		TestFault: "external_ref_movement_before_cas",
	})
	if err != nil {
		t.Fatal(err)
	}
	before := mustGit(t, root, "rev-parse", "HEAD")
	progressPath := filepath.Join(root, "demo", progressFilename)
	beforeContent, err := os.ReadFile(progressPath)
	if err != nil {
		t.Fatal(err)
	}
	result := writer.AppendProgress(AppendProgressArgs{
		ProjectID: "demo", Summary: "CAS race", Content: "candidate must not land", ClientIdempotencyKey: "cas-unit",
	})
	if result["status"] != "error" || result["code"] != "conflict" {
		t.Fatalf("CAS result: %#v", result)
	}
	after := mustGit(t, root, "rev-parse", "HEAD")
	if after == before {
		t.Fatal("external ref movement did not advance HEAD")
	}
	afterContent, err := os.ReadFile(progressPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterContent) != string(beforeContent) || strings.Contains(string(afterContent), "candidate must not land") {
		t.Fatal("losing candidate was materialized")
	}
	status := writer.Status("")
	if status["index_state"] != "stale" || status["read_only"] != true || status["write_mode"] != "degraded_read_only" {
		t.Fatalf("stale status: %#v", status)
	}
	retry := writer.AppendProgress(AppendProgressArgs{ProjectID: "demo", Summary: "retry", Content: "must remain blocked"})
	if retry["status"] != "error" || retry["code"] != "read_only" {
		t.Fatalf("stale retry: %#v", retry)
	}
	observed := writer.Observe("", "")
	if observed["operation_records"].(map[string]any)["count"] != 0 || observed["journal"].(map[string]any)["pending_count"] != 0 {
		t.Fatalf("CAS loser left partial state: %#v", observed)
	}
}

func TestSafetyRulesRejectBeforeAuthorityMovement(t *testing.T) {
	root := fixtureRepo(t)
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "conformance"})
	if err != nil {
		t.Fatal(err)
	}
	before := mustGit(t, root, "rev-parse", "HEAD")
	cases := []struct {
		name string
		args AppendProgressArgs
		rule string
	}{
		{"path", AppendProgressArgs{ProjectID: "../outside", Summary: "x", Content: "body"}, "project_id_path"},
		{"heading", AppendProgressArgs{ProjectID: "demo", Summary: "x", Content: "body\n\n## injected"}, "content_heading"},
		{"secret", AppendProgressArgs{ProjectID: "demo", Summary: "x", Content: "api_key: should-not-be-stored"}, "content_secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := writer.AppendProgress(tc.args)
			if result["status"] != "error" || result["code"] != "validation_failed" || result["rule"] != tc.rule {
				t.Fatalf("result: %#v", result)
			}
			if tc.name == "secret" && strings.Contains(strings.TrimSpace(renderMap(result)), "should-not-be-stored") {
				t.Fatalf("secret echoed: %#v", result)
			}
		})
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != before {
		t.Fatalf("safety rejection moved HEAD: before=%s after=%s", before, got)
	}
}

func TestAppendProgressNormalizesLeadingHeadingAndAllowsH3H4(t *testing.T) {
	root := fixtureRepo(t)
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "conformance"})
	if err != nil {
		t.Fatal(err)
	}
	result := writer.AppendProgress(AppendProgressArgs{
		ProjectID: "demo", Summary: "normalized markdown", Content: "## Caller supplied title\n\nbody\n\n### Detail\n- item",
	})
	if result["status"] != "committed" {
		t.Fatalf("normalized progress: %#v", result)
	}
	progressPath := filepath.Join(root, "demo", progressFilename)
	raw, err := os.ReadFile(progressPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "Caller supplied title") || !strings.Contains(text, "### Detail") || !strings.Contains(text, "body") {
		t.Fatalf("unexpected normalized progress:\n%s", text)
	}
}

func renderMap(value map[string]any) string {
	var b strings.Builder
	for k, v := range value {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(toText(v)), "\n", " "), "\r", " ")))
		b.WriteString(";")
	}
	return b.String()
}

func toText(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(func() string {
		return fmt.Sprint(v)
	}()), "\n", " "), "\r", " "))
}
