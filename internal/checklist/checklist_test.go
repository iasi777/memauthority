// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package checklist

import (
	"strings"
	"testing"
)

func TestParseHandoffReturnsOnlyTopLevelTodoChecklist(t *testing.T) {
	raw := []byte("# 交接\n\n## 已知问题 / 待办\n- [ ] alpha\n- [x] done\n- [X] upper\n  - [ ] nested\n- ordinary\n- [ ] <待办 1>\n```md\n- [ ] fenced\n```\n\n## 当前状态\n- [ ] outside\n")
	items := ParseHandoff(raw, "sha256:test")
	if len(items) != 3 {
		t.Fatalf("items=%#v", items)
	}
	want := []struct {
		text    string
		checked bool
		line    int
	}{{"alpha", false, 4}, {"done", true, 5}, {"upper", true, 6}}
	for i, expected := range want {
		item := items[i]
		if item.Text != expected.text || item.Checked != expected.checked || item.LineStart != expected.line || item.Heading != TodoHeading || !strings.HasPrefix(item.ItemRef, "todo:") {
			t.Fatalf("item[%d]=%#v want %#v", i, item, expected)
		}
	}
}

func TestApplyChecklistMutationsUsesSnapshotRefsAndSupportsBatch(t *testing.T) {
	raw := []byte("# 交接\n\n## 已知问题 / 待办\n- [ ] first\n- [ ] second\n- [x] third\n\n## 当前状态\nok\n")
	items := ParseHandoff(raw, "sha256:test")
	checked := true
	updated, err := Apply(raw, "sha256:test", []Mutation{
		{ItemRef: items[0].ItemRef, Remove: true},
		{ItemRef: items[1].ItemRef, Checked: &checked},
		{ItemRef: items[2].ItemRef, Remove: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if strings.Contains(text, "first") || strings.Contains(text, "third") || !strings.Contains(text, "- [x] second") || !strings.Contains(text, "## 当前状态\nok") {
		t.Fatalf("unexpected update:\n%s", text)
	}
}

func TestApplyRejectsUnknownAndDuplicateRefs(t *testing.T) {
	raw := []byte("# 交接\n\n## 已知问题 / 待办\n- [ ] one\n")
	items := ParseHandoff(raw, "sha256:test")
	if _, err := Apply(raw, "sha256:test", []Mutation{{ItemRef: "todo:missing", Remove: true}}); err == nil {
		t.Fatal("unknown item_ref accepted")
	}
	if _, err := Apply(raw, "sha256:test", []Mutation{{ItemRef: items[0].ItemRef, Remove: true}, {ItemRef: items[0].ItemRef, Remove: true}}); err == nil {
		t.Fatal("duplicate item_ref accepted")
	}
}
