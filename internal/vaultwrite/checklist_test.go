// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iasi777/v-memory/internal/checklist"
	"github.com/iasi777/v-memory/internal/vaultread"
)

func TestUpdateSectionsChecklistMutationsUseReadItemRefs(t *testing.T) {
	root := fixtureRepo(t)
	seedTodoHandoff(t, root)
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "checklist-test"})
	if err != nil {
		t.Fatal(err)
	}

	reader, err := vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	before := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff", Heading: checklist.TodoHeading})
	_ = reader.Close()
	items, ok := before["checklist_items"].([]checklist.Item)
	if !ok || len(items) != 2 {
		t.Fatalf("checklist_items=%#v", before["checklist_items"])
	}
	revision := before["revision"].(string)
	checked := true
	result := writer.UpdateSections(UpdateSectionsArgs{
		ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: revision,
		Operations: []SectionOperationInput{
			{Operation: "checklist_remove", Heading: checklist.TodoHeading, ItemRef: stringPtr(items[0].ItemRef)},
			{Operation: "checklist_set_checked", Heading: checklist.TodoHeading, ItemRef: stringPtr(items[1].ItemRef), Checked: &checked},
		},
	})
	if result["status"] != "committed" {
		t.Fatalf("checklist update: %#v", result)
	}

	reader, err = vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	after := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff", Heading: checklist.TodoHeading})
	_ = reader.Close()
	content := after["content"].(string)
	if strings.Contains(content, "first todo") || !strings.Contains(content, "- [x] second todo") {
		t.Fatalf("unexpected checklist content:\n%s", content)
	}
	afterItems := after["checklist_items"].([]checklist.Item)
	if len(afterItems) != 1 || afterItems[0].Text != "second todo" || !afterItems[0].Checked {
		t.Fatalf("unexpected checklist metadata: %#v", afterItems)
	}

	beforeHead := mustGit(t, root, "rev-parse", "HEAD")
	stale := writer.UpdateSections(UpdateSectionsArgs{
		ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: after["revision"].(string),
		Operations: []SectionOperationInput{{Operation: "checklist_remove", Heading: checklist.TodoHeading, ItemRef: stringPtr(items[1].ItemRef)}},
	})
	if stale["status"] != "error" || stale["code"] != "checklist_item_not_found" || !strings.Contains(stale["message"].(string), "re-read") {
		t.Fatalf("stale item_ref: %#v", stale)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != beforeHead {
		t.Fatalf("rejected stale item_ref moved HEAD: %s -> %s", beforeHead, got)
	}
}

func TestUpdateSectionsChecklistValidation(t *testing.T) {
	root := fixtureRepo(t)
	seedTodoHandoff(t, root)
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "checklist-validation"})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	read := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff", Heading: checklist.TodoHeading})
	_ = reader.Close()
	item := read["checklist_items"].([]checklist.Item)[0]
	revision := read["revision"].(string)
	body := "not allowed"
	checked := true

	tests := []struct {
		name string
		args UpdateSectionsArgs
		want string
	}{
		{
			name: "wrong role",
			args: UpdateSectionsArgs{ProjectID: "demo", Role: "progress", ExpectedResourceRevision: revision, Operations: []SectionOperationInput{{Operation: "checklist_remove", Heading: checklist.TodoHeading, ItemRef: stringPtr(item.ItemRef)}}},
			want: "only for handoff",
		},
		{
			name: "wrong heading",
			args: UpdateSectionsArgs{ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: revision, Operations: []SectionOperationInput{{Operation: "checklist_remove", Heading: "Overview", ItemRef: stringPtr(item.ItemRef)}}},
			want: checklist.TodoHeading,
		},
		{
			name: "content forbidden",
			args: UpdateSectionsArgs{ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: revision, Operations: []SectionOperationInput{{Operation: "checklist_remove", Heading: checklist.TodoHeading, ItemRef: stringPtr(item.ItemRef), Content: &body}}},
			want: "content must not be provided",
		},
		{
			name: "checked required",
			args: UpdateSectionsArgs{ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: revision, Operations: []SectionOperationInput{{Operation: "checklist_set_checked", Heading: checklist.TodoHeading, ItemRef: stringPtr(item.ItemRef)}}},
			want: "checked is required",
		},
		{
			name: "checked forbidden for remove",
			args: UpdateSectionsArgs{ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: revision, Operations: []SectionOperationInput{{Operation: "checklist_remove", Heading: checklist.TodoHeading, ItemRef: stringPtr(item.ItemRef), Checked: &checked}}},
			want: "checked must not be provided",
		},
		{
			name: "mixed batch",
			args: UpdateSectionsArgs{ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: revision, Operations: []SectionOperationInput{{Operation: "checklist_remove", Heading: checklist.TodoHeading, ItemRef: stringPtr(item.ItemRef)}, {Operation: "replace", Heading: "Overview", Content: &body}}},
			want: "cannot be mixed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			beforeHead := mustGit(t, root, "rev-parse", "HEAD")
			result := writer.UpdateSections(tc.args)
			if result["status"] != "error" || result["code"] != "validation_failed" || !strings.Contains(result["message"].(string), tc.want) {
				t.Fatalf("result=%#v", result)
			}
			if got := mustGit(t, root, "rev-parse", "HEAD"); got != beforeHead {
				t.Fatalf("validation failure moved HEAD: %s -> %s", beforeHead, got)
			}
		})
	}
}

func seedTodoHandoff(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "demo", "交接.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\n## 已知问题 / 待办\n- [ ] first todo\n- [ ] second todo\n")...)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "demo/交接.md")
	mustGit(t, root, "commit", "-q", "-m", "seed todo checklist")
}
