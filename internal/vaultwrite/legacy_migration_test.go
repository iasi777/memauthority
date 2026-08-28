// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestLegacyFingerprintMatchesFrozenPython(t *testing.T) {
	cases := []struct {
		name, project, role, operation, want string
		payload                              any
	}{
		{"append-normalized", "demo", "progress", "append_progress", "sha256:df9f4edd2104d03a1ddb0f94a72413ae0e6bf7c5ef61a89cd505e3cb10efd79a", map[string]any{"summary": p5LegacyCleanText("  Re\u0301sume\u0301  "), "content": p5LegacyCleanText(" Body\r\nline  "), "evidence": []EvidenceInput{}}},
		{"sections", "demo", "handoff", "update_sections", "sha256:a818a3c95a27f9f942f20dcf39e9a48bfe9a2836c866a1be1b2c48dbdc99e874", map[string]any{"expected_resource_revision": "sha256:" + strings.Repeat("1", 64), "operations": p5LegacySectionOperations([]SectionOperationInput{{Operation: "replace", Heading: "Next", Content: stringPtr("Body")}}), "evidence": []EvidenceInput{}}},
		{"sections-delete-null", "demo", "rules", "update_sections", "sha256:107660f1d60c35e619444f5b5fa9be53b69fd0a9aca83b9360463f201c9940dc", map[string]any{"expected_resource_revision": "sha256:" + strings.Repeat("1", 64), "operations": p5LegacySectionOperations([]SectionOperationInput{{Operation: "delete", Heading: " Next "}}), "evidence": []EvidenceInput{}}},
		{"create", "alpha", "handoff", "create_project", "sha256:7069eda9e2499e2116ed17bdce2d2dc37abb6c1b6e509c458edb35d5ed86e62c", map[string]any{"description": "Demo", "lifecycle": "active", "local_path": nil, "aliases": []string{}, "expected_index_revision": "sha256:" + strings.Repeat("2", 64), "evidence": []EvidenceInput{}, "create_rules_role": true}},
		{"handoff", "demo", "handoff", "update_handoff", "sha256:c7c69f6cea6ca940aed476724d75925076f1dffb7302882aeabd2c147268f0bd", map[string]any{"expected_resource_revision": "sha256:" + strings.Repeat("4", 64), "section_updates": p5LegacySectionUpdates([]SectionUpdateInput{{Heading: " Next ", Replacement: " Body "}}), "evidence": []EvidenceInput{}}},
		{"verified", "demo", "handoff", "mark_verified", "sha256:49a40c6c7b4ea355759283bdcfcfd5320251114ff86d268859d2b5262cf2e81b", map[string]any{"expected_resource_revision": "sha256:" + strings.Repeat("5", 64), "verification_level": "local", "verified_scope": p5LegacyStrings([]string{" scope one "}), "basis": p5LegacyStrings([]string{" basis one "}), "result": p5LegacyCleanText(" passed "), "not_verified": p5LegacyStrings(nil), "evidence": []EvidenceInput{}}},
		{"pitfall", "demo", "pitfalls", "record_pitfall", "sha256:5000b35faa2d9d6e78c49f708043b6162c6fcced9fbc8ed3d8bc3a17dfa2d3c7", map[string]any{"title": p5LegacyCleanText(" Title "), "severity": "high", "tags": p5LegacyStrings([]string{" tag "}), "content": p5LegacyCleanText(" Body "), "related": p5LegacyRelated(""), "evidence": []EvidenceInput{}}},
		{"archive", "demo", "index", "archive_project", "sha256:efabdc4dd9669da94701af6c828d8ee2a1c045949726ec41851d89ac51ba7a65", map[string]any{"reason": p5LegacyCleanText(" Reason "), "expected_index_revision": "sha256:" + strings.Repeat("6", 64), "evidence": []EvidenceInput{}}},
		{"restore", "demo", "index", "restore_project", "sha256:6f5dfacd22efd40784625f7a52892d1b56d06985e4b1eea502277615f98ad543", map[string]any{"expected_index_revision": "sha256:" + strings.Repeat("7", 64), "evidence": []EvidenceInput{}}},
		{"migrate", "INDEX", "index", "migrate_lifecycle", "sha256:6376ad8e557d8661084a3f9d2873bf64df74c3a3758f177d5bc2c2a1f7daf5b5", map[string]any{"manifest": p5LegacyLifecycleManifest(LifecycleMigrationManifestInput{SchemaVersion: 1, ExpectedIndexRevision: "sha256:" + strings.Repeat("8", 64), ArchiveReason: " Historical ", Projects: []LifecycleMigrationProjectInput{{ProjectID: " demo ", SourceLifecycle: "active", TargetLifecycle: "active"}}}), "evidence": []EvidenceInput{}}},
		{"runtime", "demo", "runtime", "update_runtime", "sha256:7c6a66070315ab88cb17c4694724378cd38f50406feeee55445e2423613018c0", map[string]any{"runtime": map[string]any{"schema_version": 1, "project_id": "demo", "environments": []any{}, "relations": []any{}, "verification": map[string]any{"evidence": []any{}}}, "expected_resource_revision": nil, "expected_index_revision": "sha256:" + strings.Repeat("3", 64), "evidence": []EvidenceInput{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := legacyOperationFingerprint(tc.project, tc.role, tc.operation, tc.payload)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("legacy fingerprint=%s want=%s", got, tc.want)
			}
		})
	}
}

func TestImportActivePythonCallerKeyAndReplay(t *testing.T) {
	root := fixtureRepo(t)
	head, err := gitHead(root)
	if err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "python-index.sqlite3")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE operations (
operation_id TEXT PRIMARY KEY, fingerprint TEXT NOT NULL, caller_key TEXT,
payload_hash TEXT NOT NULL, operation TEXT NOT NULL, project_id TEXT NOT NULL,
role TEXT, status TEXT NOT NULL, result_json TEXT NOT NULL, resulting_commit TEXT,
entry_marker TEXT, created_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	rawKey := "  legacy-caller-key  "
	key := p5LegacyCleanText(rawKey)
	payload := map[string]any{"summary": p5LegacyCleanText(" Summary "), "content": p5LegacyCleanText(" Body "), "evidence": []EvidenceInput{}}
	fingerprint, err := legacyOperationFingerprint("demo", "progress", "append_progress", payload)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]any{"status": "committed", "project_id": "demo", "resource_uri": "memory://projects/demo/progress", "operation_id": "legacy-op", "new_commit": head, "idempotent_replay": false, "caller_key_hash": "sha256:" + hashText(key)}
	resultJSON, _ := json.Marshal(result)
	asOf := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	_, err = db.Exec(`INSERT INTO operations(operation_id,fingerprint,caller_key,payload_hash,operation,project_id,role,status,result_json,resulting_commit,entry_marker,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		"legacy-op", fingerprint, "sha256:"+hashText(key), "sha256:"+strings.Repeat("a", 64), "append_progress", "demo", "progress", "committed", string(resultJSON), head, nil, asOf.Add(-time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := ImportLegacyPythonIdempotencyWindow(root, dbPath, state, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if report.ImportedRows != 1 || report.CallerKeys != 1 || report.ExpiredRows != 0 {
		t.Fatalf("report=%#v", report)
	}
	raw, err := os.ReadFile(filepath.Join(state, "operations", "legacy-op.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), key) || !strings.Contains(string(raw), `"legacy_fingerprint"`) {
		t.Fatalf("unsafe/incomplete imported record: %s", raw)
	}

	writer, err := New(root, Config{WorkRoot: state, WriteEnabled: true, WriteSource: "slice9-test"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := gitHead(root)
	replay := writer.AppendProgress(AppendProgressArgs{ProjectID: "demo", Summary: " Summary ", Content: " Body ", ClientIdempotencyKey: rawKey})
	after, _ := gitHead(root)
	if replay["status"] != "committed" || replay["idempotent_replay"] != true || before != after {
		t.Fatalf("replay=%#v before=%s after=%s", replay, before, after)
	}
	mismatch := writer.AppendProgress(AppendProgressArgs{ProjectID: "demo", Summary: "Different", Content: "Body", ClientIdempotencyKey: rawKey})
	if mismatch["status"] != "error" || mismatch["rule"] != "idempotency_key" {
		t.Fatalf("mismatch=%#v", mismatch)
	}
}
