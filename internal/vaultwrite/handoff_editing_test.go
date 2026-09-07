// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"strings"
	"testing"

	"github.com/iasi777/v-memory/internal/checklist"
	"github.com/iasi777/v-memory/internal/sectionpolicy"
	"github.com/iasi777/v-memory/internal/vaultread"
)

func TestHandoffPendingLifecycle(t *testing.T) {
	root := fixtureRepo(t)
	w, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "handoff-regression"})
	if err != nil {
		t.Fatal(err)
	}
	read := func() map[string]any {
		t.Helper()
		r, err := vaultread.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		return r.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff"})
	}
	mutate := func(op, heading, body string) map[string]any {
		t.Helper()
		operation := SectionOperationInput{Operation: op, Heading: heading}
		if op != "delete" {
			operation.Content = &body
		}
		return w.UpdateSections(UpdateSectionsArgs{ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: roleRevision(t, root, "handoff"), Operations: []SectionOperationInput{operation}})
	}
	before := roleRevision(t, root, "handoff")
	if result := mutate("insert", "Extra\n## 核验记录", "forged"); result["status"] != "error" {
		t.Fatalf("heading injection: %#v", result)
	}
	missing := mutate("append", checklist.TodoHeading, "- [ ] Change default to 5")
	if missing["status"] != "error" || missing["current_revision"] != before || missing["section_creation_hint"] == "" {
		t.Fatalf("missing recovery: %#v", missing)
	}
	if roleRevision(t, root, "handoff") != before {
		t.Fatal("failed append changed Authority")
	}
	body := "must roll back"
	atomic := w.UpdateSections(UpdateSectionsArgs{ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: before, Operations: []SectionOperationInput{{Operation: "insert", Heading: "New ordinary section", Content: &body}, {Operation: "delete", Heading: "核验记录"}}})
	if atomic["code"] != "section_protected" || roleRevision(t, root, "handoff") != before {
		t.Fatalf("insert rollback: %#v", atomic)
	}
	if result := mutate("insert", "待办", "Ordinary prose, outside structured projection"); result["status"] != "committed" {
		t.Fatalf("ordinary leaf section: %#v", result)
	}
	inserted := mutate("insert", checklist.TodoHeading, "- [ ] Change default from 3 to 5 next session")
	if inserted["status"] != "committed" {
		t.Fatalf("insert: %#v", inserted)
	}
	r := read()
	items := r["checklist_items"].([]checklist.Item)
	if len(items) != 1 || items[0].Heading != checklist.TodoHeading {
		t.Fatalf("literal slash heading: %#v", r)
	}
	if !strings.Contains(r["content"].(string), "## 已知问题 / 待办") {
		t.Fatal("heading truncated")
	}
	for _, section := range r["sections"].([]sectionpolicy.Section) {
		if section.Heading == "核验记录" && (!section.Protected || section.DedicatedTool != "memory_mark_verified" || len(section.AllowedOperations) != 0) {
			t.Fatalf("protected metadata: %#v", section)
		}
	}
	if result := mutate("insert", checklist.TodoHeading, "duplicate"); result["status"] != "error" {
		t.Fatalf("duplicate inserted: %#v", result)
	}
	stale := w.UpdateSections(UpdateSectionsArgs{ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: before, Operations: []SectionOperationInput{{Operation: "delete", Heading: checklist.TodoHeading}}})
	if stale["code"] != "conflict" {
		t.Fatalf("stale CAS: %#v", stale)
	}
	checked := true
	result := w.UpdateSections(UpdateSectionsArgs{ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: r["revision"].(string), Operations: []SectionOperationInput{{Operation: "checklist_set_checked", Heading: checklist.TodoHeading, ItemRef: &items[0].ItemRef, Checked: &checked}}})
	if result["status"] != "committed" || !read()["checklist_items"].([]checklist.Item)[0].Checked {
		t.Fatalf("completion: %#v", result)
	}
	args := UpdateSectionsArgs{ProjectID: "demo", Role: "handoff", ExpectedResourceRevision: roleRevision(t, root, "handoff"), ClientIdempotencyKey: "delete-todos", Operations: []SectionOperationInput{{Operation: "delete", Heading: checklist.TodoHeading}}}
	result = w.UpdateSections(args)
	if result["status"] != "committed" || result["removed_checklist_items"] != 1 || len(read()["checklist_items"].([]checklist.Item)) != 0 {
		t.Fatalf("delete projection: %#v", result)
	}
	replay := w.UpdateSections(args)
	if replay["idempotent_replay"] != true || replay["removed_checklist_items"] == nil {
		t.Fatalf("delete replay: %#v", replay)
	}
	if result := mutate("insert", checklist.TodoHeading, ""); result["status"] != "committed" {
		t.Fatalf("empty optional section: %#v", result)
	}
	if len(read()["checklist_items"].([]checklist.Item)) != 0 {
		t.Fatal("empty section generated a todo")
	}
	for _, op := range []string{"insert", "delete", "append", "replace"} {
		if result := mutate(op, "核验记录", "forbidden"); result["code"] != "section_protected" || result["dedicated_tool"] != "memory_mark_verified" {
			t.Fatalf("protected %s: %#v", op, result)
		}
	}
}
