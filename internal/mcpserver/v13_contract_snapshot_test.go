// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestV13RuntimeEnabledContractSnapshot(t *testing.T) {
	root := fixtureRepoFrom(t, "route-read-revision")
	app, err := OpenWithOptions(root, t.TempDir(), OpenOptions{RuntimeEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	session := inMemorySession(t, app)

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONSnapshotEqual(t, tools.Tools, "../../docs/contract/v1.3/mcp-tools.json")

	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONSnapshotEqual(t, templates.ResourceTemplates, "../../docs/contract/v1.3/mcp-resource-templates.json")
}

func assertJSONSnapshotEqual(t *testing.T, current any, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	currentRaw, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal(currentRaw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime contract differs from frozen snapshot %s", path)
	}
}
