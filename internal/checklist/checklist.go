// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package checklist

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const TodoHeading = "已知问题 / 待办"

var ErrItemNotFound = errors.New("checklist item_ref does not identify an item in the expected resource revision")

type MutationError struct {
	Index int
	Err   error
}

func (e *MutationError) Error() string { return e.Err.Error() }
func (e *MutationError) Unwrap() error { return e.Err }

type Item struct {
	ItemRef   string `json:"item_ref"`
	Heading   string `json:"heading"`
	Text      string `json:"text"`
	Checked   bool   `json:"checked"`
	LineStart int    `json:"line_start"`

	byteStart    int
	byteEnd      int
	markerOffset int
}

type Mutation struct {
	ItemRef string
	Remove  bool
	Checked *bool
}

type edit struct {
	start       int
	end         int
	replacement string
}

func ParseHandoff(raw []byte, resourceRevision string) []Item {
	lines := splitLines(raw)
	items := make([]Item, 0)
	inFence := false
	var fenceChar byte
	fenceLen := 0
	inTodo := false

	for i, line := range lines {
		logical := strings.TrimSuffix(strings.TrimSuffix(line.text, "\n"), "\r")
		if marker, count, ok := fence(logical); ok {
			if !inFence {
				inFence = true
				fenceChar = marker
				fenceLen = count
			} else if marker == fenceChar && count >= fenceLen {
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}

		if heading, level, ok := markdownHeading(logical); ok {
			if level == 2 && heading == TodoHeading {
				inTodo = true
				continue
			}
			if inTodo && level <= 2 {
				break
			}
			continue
		}
		if !inTodo {
			continue
		}
		checked, text, markerOffset, ok := checklistLine(logical)
		if !ok || isPlaceholder(text) {
			continue
		}
		lineStart := i + 1
		items = append(items, Item{
			ItemRef:      itemRef(resourceRevision, lineStart, text, checked),
			Heading:      TodoHeading,
			Text:         text,
			Checked:      checked,
			LineStart:    lineStart,
			byteStart:    line.start,
			byteEnd:      line.end,
			markerOffset: line.start + markerOffset,
		})
	}
	return items
}

func Apply(raw []byte, resourceRevision string, mutations []Mutation) ([]byte, error) {
	items := ParseHandoff(raw, resourceRevision)
	byRef := make(map[string]Item, len(items))
	for _, item := range items {
		byRef[item.ItemRef] = item
	}
	seen := map[string]struct{}{}
	edits := make([]edit, 0, len(mutations))
	for i, mutation := range mutations {
		if _, exists := seen[mutation.ItemRef]; exists {
			return nil, &MutationError{Index: i, Err: fmt.Errorf("duplicate checklist item_ref in one mutation batch")}
		}
		seen[mutation.ItemRef] = struct{}{}
		item, ok := byRef[mutation.ItemRef]
		if !ok {
			return nil, &MutationError{Index: i, Err: ErrItemNotFound}
		}
		if mutation.Remove {
			edits = append(edits, edit{start: item.byteStart, end: item.byteEnd})
			continue
		}
		if mutation.Checked == nil {
			return nil, &MutationError{Index: i, Err: fmt.Errorf("checklist checked state is required")}
		}
		replacement := " "
		if *mutation.Checked {
			replacement = "x"
		}
		edits = append(edits, edit{start: item.markerOffset, end: item.markerOffset + 1, replacement: replacement})
	}

	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	text := string(raw)
	for _, e := range edits {
		if e.start < 0 || e.end < e.start || e.end > len(text) {
			return nil, fmt.Errorf("checklist edit is outside the resource")
		}
		text = text[:e.start] + e.replacement + text[e.end:]
	}
	return []byte(text), nil
}

type lineSpan struct {
	text       string
	start, end int
}

func splitLines(raw []byte) []lineSpan {
	text := string(raw)
	parts := strings.SplitAfter(text, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	lines := make([]lineSpan, 0, len(parts))
	offset := 0
	for _, part := range parts {
		lines = append(lines, lineSpan{text: part, start: offset, end: offset + len(part)})
		offset += len(part)
	}
	return lines
}

func markdownHeading(line string) (string, int, bool) {
	leading := len(line) - len(strings.TrimLeft(line, " "))
	if leading > 3 {
		return "", 0, false
	}
	trimmed := line[leading:]
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || len(trimmed) <= level || trimmed[level] != ' ' {
		return "", 0, false
	}
	return strings.TrimSpace(trimmed[level+1:]), level, true
}

func checklistLine(line string) (bool, string, int, bool) {
	if !strings.HasPrefix(line, "- [") || len(line) < 6 || line[4] != ']' || line[5] != ' ' {
		return false, "", 0, false
	}
	marker := line[3]
	if marker != ' ' && marker != 'x' && marker != 'X' {
		return false, "", 0, false
	}
	text := strings.TrimSpace(line[6:])
	if text == "" {
		return false, "", 0, false
	}
	return marker == 'x' || marker == 'X', text, 3, true
}

func isPlaceholder(text string) bool {
	if text == "<待办>" {
		return true
	}
	return strings.HasPrefix(text, "<待办 ") && strings.HasSuffix(text, ">")
}

func itemRef(revision string, lineStart int, text string, checked bool) string {
	payload := revision + "\x00" + TodoHeading + "\x00" + strconv.Itoa(lineStart) + "\x00" + text + "\x00" + strconv.FormatBool(checked)
	sum := sha256.Sum256([]byte(payload))
	return "todo:" + hex.EncodeToString(sum[:])
}

func fence(line string) (byte, int, bool) {
	leading := len(line) - len(strings.TrimLeft(line, " "))
	if leading > 3 {
		return 0, 0, false
	}
	trimmed := line[leading:]
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0, false
	}
	marker := trimmed[0]
	count := 0
	for count < len(trimmed) && trimmed[count] == marker {
		count++
	}
	if count < 3 {
		return 0, 0, false
	}
	return marker, count, true
}
