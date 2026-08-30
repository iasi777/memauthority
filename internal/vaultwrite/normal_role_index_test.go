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

	"github.com/iasi777/memauthority/internal/vaultread"
)

func TestNormalRoleMutationsSynchronizeIndexAndSkipSameDayRewrite(t *testing.T) {
	root := fixtureRepo(t)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	writer, err := New(root, Config{
		WorkRoot:     t.TempDir(),
		WriteEnabled: true,
		WriteSource:  "unit-normal-role-index",
		Now:          func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}

	type mutationCase struct {
		date     string
		role     string
		rolePath string
		mutate   func() map[string]any
	}
	cases := []mutationCase{
		{
			date: "2026-08-12", role: "progress", rolePath: "demo/进度.md",
			mutate: func() map[string]any {
				return writer.AppendProgress(AppendProgressArgs{
					ProjectID: "demo", Summary: "date sync progress", Content: "role and index move together",
					ClientIdempotencyKey: "normal-role-index-progress",
				})
			},
		},
		{
			date: "2026-08-13", role: "handoff", rolePath: "demo/交接.md",
			mutate: func() map[string]any {
				return writer.UpdateHandoff(UpdateHandoffArgs{
					ProjectID: "demo", ExpectedResourceRevision: roleRevision(t, root, "handoff"),
					SectionUpdates:       []SectionUpdateInput{{Heading: "Overview", Replacement: "handoff date synchronization"}},
					ClientIdempotencyKey: "normal-role-index-handoff",
				})
			},
		},
		{
			date: "2026-08-14", role: "rules", rolePath: "demo/规范.md",
			mutate: func() map[string]any {
				return writer.UpdateSections(UpdateSectionsArgs{
					ProjectID: "demo", Role: "rules", ExpectedResourceRevision: roleRevision(t, root, "rules"),
					Operations:           []SectionOperationInput{{Operation: "replace", Heading: "Rule", Content: stringPtr("rules date synchronization")}},
					ClientIdempotencyKey: "normal-role-index-rules",
				})
			},
		},
		{
			date: "2026-08-15", role: "pitfalls", rolePath: "demo/踩坑.md",
			mutate: func() map[string]any {
				return writer.RecordPitfall(RecordPitfallArgs{
					ProjectID: "demo", Title: "date synchronization", Severity: "low", Tags: []string{"contract"},
					Content: "pitfall date synchronization", ClientIdempotencyKey: "normal-role-index-pitfalls",
				})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			parsed, err := time.Parse("2006-01-02", tc.date)
			if err != nil {
				t.Fatal(err)
			}
			now = parsed.Add(12 * time.Hour)
			beforeHead := mustGit(t, root, "rev-parse", "HEAD")
			result := tc.mutate()
			if result["status"] != "committed" {
				t.Fatalf("mutation: %#v", result)
			}
			newCommit := result["new_commit"].(string)
			if got := mustGit(t, root, "-c", "core.quotePath=false", "diff", "--name-only", beforeHead, newCommit); got != "INDEX.yaml\n"+tc.rolePath {
				t.Fatalf("changed paths: %q", got)
			}
			if got := projectLastUpdated(t, root, "demo"); got != tc.date {
				t.Fatalf("project last_updated=%q want %q", got, tc.date)
			}
			roleRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(tc.rolePath)))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(roleRaw), "最后更新: "+tc.date) {
				t.Fatalf("role frontmatter did not update to %s", tc.date)
			}
		})
	}

	indexBeforeSameDay, err := os.ReadFile(filepath.Join(root, "INDEX.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	sameDayBefore := mustGit(t, root, "rev-parse", "HEAD")
	updated := writer.UpdateSections(UpdateSectionsArgs{
		ProjectID: "demo", Role: "rules", ExpectedResourceRevision: roleRevision(t, root, "rules"),
		Operations:           []SectionOperationInput{{Operation: "replace", Heading: "Rule", Content: stringPtr("same-day role update")}},
		ClientIdempotencyKey: "normal-role-index-same-day",
	})
	if updated["status"] != "committed" {
		t.Fatalf("same-day update: %#v", updated)
	}
	indexAfterSameDay, err := os.ReadFile(filepath.Join(root, "INDEX.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(indexBeforeSameDay, indexAfterSameDay) {
		t.Fatal("same-day role mutation rewrote INDEX.yaml")
	}
	if got := mustGit(t, root, "-c", "core.quotePath=false", "diff", "--name-only", sameDayBefore, updated["new_commit"].(string)); got != "demo/规范.md" {
		t.Fatalf("same-day mutation paths: %q", got)
	}
}

func roleRevision(t *testing.T, root, role string) string {
	t.Helper()
	reader, err := vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	result := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/" + role})
	if result["status"] != "ok" {
		t.Fatalf("%s read: %#v", role, result)
	}
	revision, ok := result["revision"].(string)
	if !ok || revision == "" {
		t.Fatalf("%s revision missing: %#v", role, result)
	}
	return revision
}

func projectLastUpdated(t *testing.T, root, projectID string) string {
	t.Helper()
	reader, err := vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, project := range reader.Projects() {
		if project.ID == projectID {
			return project.LastUpdated
		}
	}
	t.Fatalf("project %q not found", projectID)
	return ""
}

func stringPtr(value string) *string { return &value }
