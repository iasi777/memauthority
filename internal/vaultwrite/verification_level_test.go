// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"strings"
	"testing"

	"github.com/iasi777/v-memory/internal/vaultread"
)

func TestMarkVerifiedAcceptsDocumentedVerificationLevels(t *testing.T) {
	tests := []struct {
		level    string
		rendered string
	}{
		{level: "local", rendered: "本地核验"},
		{level: "runtime", rendered: "运行时核验"},
	}
	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			root := fixtureRepo(t)
			writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "verification-level-test"})
			if err != nil {
				t.Fatal(err)
			}
			result := writer.MarkVerified(MarkVerifiedArgs{
				ProjectID: "demo", ExpectedResourceRevision: roleRevision(t, root, "handoff"), VerificationLevel: tc.level,
				VerifiedScope: []string{"contract"}, Basis: []string{"focused test"}, Result: "pass",
			})
			if result["status"] != "committed" {
				t.Fatalf("MarkVerified(%q): %#v", tc.level, result)
			}
			reader, err := vaultread.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			read := reader.Read(vaultread.ReadArgs{URI: "memory://projects/demo/handoff", Heading: "核验记录"})
			if read["status"] != "ok" || !strings.Contains(read["content"].(string), "核验级别: "+tc.rendered) {
				t.Fatalf("rendered verification level for %q: %#v", tc.level, read)
			}
		})
	}
}

func TestMarkVerifiedRejectsUndocumentedVerificationLevel(t *testing.T) {
	root := fixtureRepo(t)
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "verification-level-test"})
	if err != nil {
		t.Fatal(err)
	}
	before := mustGit(t, root, "rev-parse", "HEAD")
	result := writer.MarkVerified(MarkVerifiedArgs{
		ProjectID: "demo", ExpectedResourceRevision: roleRevision(t, root, "handoff"), VerificationLevel: "remote",
		VerifiedScope: []string{"contract"}, Basis: []string{"focused test"}, Result: "pass",
	})
	message, _ := result["message"].(string)
	if result["status"] != "error" || result["code"] != "validation_failed" || result["rule"] != "verification_level" || !strings.Contains(message, "local, runtime") {
		t.Fatalf("invalid verification level result: %#v", result)
	}
	if after := mustGit(t, root, "rev-parse", "HEAD"); after != before {
		t.Fatalf("invalid verification level moved HEAD: %s -> %s", before, after)
	}
}
