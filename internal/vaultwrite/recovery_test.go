// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"fmt"
	"strings"
	"testing"
)

func TestStartupRecoveryPreparedRefAndRollbackStates(t *testing.T) {
	cases := []struct {
		phase      string
		recovered  int
		discarded  int
		rolledBack int
		contains   bool
	}{
		{phase: "journal_persisted_before_ref", discarded: 1},
		{phase: "ref_updated_before_materialize", recovered: 1, contains: true},
		{phase: "materialized_before_index", recovered: 1, contains: true},
		{phase: "index_failure_before_rollback", rolledBack: 1},
	}
	for _, tc := range cases {
		t.Run(tc.phase, func(t *testing.T) {
			root := fixtureRepo(t)
			work := t.TempDir()
			writer, err := New(root, Config{
				WorkRoot: work, WriteEnabled: true, WriteSource: "unit-crash",
				CrashFaultPhase: tc.phase,
				CrashHook:       func(phase string) { panic("crash:" + phase) },
			})
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if r := recover(); r == nil {
						t.Fatalf("fault %s did not crash", tc.phase)
					}
				}()
				writer.AppendProgress(AppendProgressArgs{
					ProjectID: "demo", Summary: "recover " + tc.phase,
					Content: "body " + tc.phase, ClientIdempotencyKey: "key-" + tc.phase,
				})
			}()
			restarted, err := New(root, Config{WorkRoot: work, WriteEnabled: true, WriteSource: "unit-crash"})
			if err != nil {
				t.Fatal(err)
			}
			summary := restarted.RecoverySummary()
			if summary["recovered_count"] != tc.recovered || summary["discarded_count"] != tc.discarded || summary["rolled_back_count"] != tc.rolledBack {
				t.Fatalf("recovery summary for %s: %#v", tc.phase, summary)
			}
			raw := mustGit(t, root, "show", "HEAD:demo/进度.md")
			gotContains := strings.Contains(raw, "body "+tc.phase)
			if gotContains != tc.contains {
				t.Fatalf("content presence=%v want=%v for %s", gotContains, tc.contains, tc.phase)
			}
			if status := mustGit(t, root, "status", "--porcelain"); status != "" {
				t.Fatalf("dirty after recovery: %q", status)
			}
			obs := restarted.Observe(restarted.indexHead, "")
			if obs["journal"].(map[string]any)["pending_count"] != 0 || obs["index"].(map[string]any)["state"] != "current" {
				t.Fatalf("post recovery tuple: %#v", obs)
			}
		})
	}
}

func TestLaterEquivalentAppendReplaysRepresentedEntry(t *testing.T) {
	root := fixtureRepo(t)
	work := t.TempDir()
	writer, err := New(root, Config{WorkRoot: work, WriteEnabled: true, WriteSource: "unit-idempotency"})
	if err != nil {
		t.Fatal(err)
	}
	firstArgs := AppendProgressArgs{ProjectID: "demo", Summary: "stable", Content: "stable represented entry", ClientIdempotencyKey: "first-key"}
	first := writer.AppendProgress(firstArgs)
	if first["status"] != "committed" {
		t.Fatalf("first: %#v", first)
	}
	later := writer.AppendProgress(AppendProgressArgs{ProjectID: "demo", Summary: "later", Content: "later entry", ClientIdempotencyKey: "later-key"})
	if later["status"] != "committed" {
		t.Fatalf("later: %#v", later)
	}
	replayArgs := firstArgs
	replayArgs.ClientIdempotencyKey = "different-key"
	replay := writer.AppendProgress(replayArgs)
	if replay["idempotent_replay"] != true || replay["operation_id"] != first["operation_id"] || replay["new_commit"] != first["new_commit"] {
		t.Fatalf("later replay: %#v first=%#v", replay, first)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != later["new_commit"] {
		t.Fatalf("replay moved current HEAD: %s later=%v", got, later["new_commit"])
	}
	obs := writer.Observe(writer.indexHead, fmt.Sprint(first["operation_id"]))
	if obs["operation_records"].(map[string]any)["count"] != 2 {
		t.Fatalf("operation count after replay: %#v", obs)
	}
}
