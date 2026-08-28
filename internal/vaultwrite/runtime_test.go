// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/iasi777/v-memory/internal/runtimeview"
	"github.com/iasi777/v-memory/internal/vaultread"
)

func TestRuntimeUpdateCreateReplaceAndCAS(t *testing.T) {
	root := runtimeWriteFixtureRepo(t)
	work := t.TempDir()
	writer, err := New(root, Config{WorkRoot: work, WriteEnabled: true, WriteSource: "unit-runtime", Now: fixedRuntimeNow})
	if err != nil {
		t.Fatal(err)
	}
	index0, err := writer.currentIndexRevision()
	if err != nil {
		t.Fatal(err)
	}
	before := mustGit(t, root, "rev-parse", "HEAD")
	create := UpdateRuntimeArgs{ProjectID: "other", Runtime: testRuntimeManifest("other", "created", "directory_exists"), ExpectedIndexRevision: index0, ClientIdempotencyKey: "runtime-create-unit"}
	created := writer.UpdateRuntime(create)
	if created["status"] != "committed" || created["previous_revision"] != nil {
		t.Fatalf("create runtime: %#v", created)
	}
	createdCommit := created["new_commit"].(string)
	createdRevision := created["new_revision"].(string)
	if createdCommit == before {
		t.Fatal("runtime create did not advance HEAD")
	}
	index1, _ := writer.currentIndexRevision()
	if index1 == index0 {
		t.Fatal("runtime create did not register resource in INDEX")
	}
	assertRuntimeView(t, root, "other", "created", createdRevision)

	replay := writer.UpdateRuntime(create)
	if replay["status"] != "committed" || replay["idempotent_replay"] != true || replay["new_commit"] != createdCommit {
		t.Fatalf("runtime create replay: %#v", replay)
	}

	replaceExpected := createdRevision
	replace := UpdateRuntimeArgs{ProjectID: "other", Runtime: testRuntimeManifest("other", "replaced", "file_exists"), ExpectedResourceRevision: &replaceExpected, ExpectedIndexRevision: index1, ClientIdempotencyKey: "runtime-replace-unit"}
	replaced := writer.UpdateRuntime(replace)
	if replaced["status"] != "committed" || replaced["previous_revision"] != createdRevision {
		t.Fatalf("replace runtime: %#v", replaced)
	}
	replacedCommit := replaced["new_commit"].(string)
	replacedRevision := replaced["new_revision"].(string)
	index2, _ := writer.currentIndexRevision()
	if index2 != index1 {
		t.Fatalf("runtime replace changed INDEX revision: %s -> %s", index1, index2)
	}
	assertRuntimeView(t, root, "other", "replaced", replacedRevision)

	stale := createdRevision
	rejected := writer.UpdateRuntime(UpdateRuntimeArgs{ProjectID: "other", Runtime: testRuntimeManifest("other", "stale", "file_exists"), ExpectedResourceRevision: &stale, ExpectedIndexRevision: index2, ClientIdempotencyKey: "runtime-stale-unit"})
	if rejected["status"] != "error" || rejected["code"] != "conflict" || rejected["current_revision"] != replacedRevision {
		t.Fatalf("stale runtime result: %#v", rejected)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != replacedCommit {
		t.Fatalf("stale runtime moved HEAD: %s", got)
	}
	observed := writer.Observe("", "")
	if observed["operation_records"].(map[string]any)["count"] != 2 || observed["journal"].(map[string]any)["pending_count"] != 0 {
		t.Fatalf("runtime transaction evidence: %#v", observed)
	}
}

func TestRuntimeClosedCheckRejectsBeforeAuthorityMovement(t *testing.T) {
	root := runtimeWriteFixtureRepo(t)
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "unit-runtime", Now: fixedRuntimeNow})
	if err != nil {
		t.Fatal(err)
	}
	indexRevision, _ := writer.currentIndexRevision()
	before := mustGit(t, root, "rev-parse", "HEAD")
	bad := testRuntimeManifest("other", "bad", "shell")
	result := writer.UpdateRuntime(UpdateRuntimeArgs{ProjectID: "other", Runtime: bad, ExpectedIndexRevision: indexRevision})
	if result["status"] != "error" || result["code"] != "validation_failed" || result["rule"] != "runtime_check" {
		t.Fatalf("closed check rejection: %#v", result)
	}
	findings, ok := result["validation_errors"].([]runtimeview.ValidationFinding)
	if !ok || len(findings) != 1 || findings[0].Path != "environments[0].checks[0].kind" {
		t.Fatalf("closed check detailed findings: %#v", result["validation_errors"])
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != before {
		t.Fatalf("invalid runtime moved HEAD: %s", got)
	}
	observed := writer.Observe("", "")
	if observed["operation_records"].(map[string]any)["count"] != 0 || observed["journal"].(map[string]any)["pending_count"] != 0 {
		t.Fatalf("invalid runtime created durable transaction state: %#v", observed)
	}
}

func TestRuntimeStaticValidationPrecedesIndexCASAndAggregates(t *testing.T) {
	root := runtimeWriteFixtureRepo(t)
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "unit-runtime", Now: fixedRuntimeNow})
	if err != nil {
		t.Fatal(err)
	}
	before := mustGit(t, root, "rev-parse", "HEAD")
	bad := testRuntimeManifest("other", "bad", "directory_exists")
	bad.Environments[0].HostID = "local-linux"
	bad.Environments[0].Kind = "virtual_machine"
	bad.Environments[0].Capabilities = append(bad.Environments[0].Capabilities, "root")
	staleIndex := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	result := writer.UpdateRuntime(UpdateRuntimeArgs{
		ProjectID: "other", Runtime: bad, ExpectedIndexRevision: staleIndex, ClientIdempotencyKey: "runtime-invalid-before-cas",
	})
	if result["status"] != "error" || result["code"] != "validation_failed" || result["rule"] != "runtime_capabilities" {
		t.Fatalf("static validation did not precede INDEX CAS: %#v", result)
	}
	findings, ok := result["validation_errors"].([]runtimeview.ValidationFinding)
	if !ok || len(findings) < 3 {
		t.Fatalf("expected aggregated validation_errors: %#v", result["validation_errors"])
	}
	wantPaths := map[string]bool{
		"environments[0].capabilities[2]": false,
		"environments[0].host_id":         false,
		"environments[0].kind":            false,
	}
	for _, finding := range findings {
		if _, exists := wantPaths[finding.Path]; exists {
			wantPaths[finding.Path] = true
		}
	}
	for path, seen := range wantPaths {
		if !seen {
			t.Errorf("missing validation path %s in %#v", path, findings)
		}
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != before {
		t.Fatalf("invalid runtime moved HEAD: %s", got)
	}
	observed := writer.Observe("", "")
	if observed["operation_records"].(map[string]any)["count"] != 0 || observed["journal"].(map[string]any)["pending_count"] != 0 {
		t.Fatalf("invalid runtime created durable transaction state: %#v", observed)
	}
}

func TestRuntimeGitChecksDefaultToRepositoryTarget(t *testing.T) {
	root := runtimeWriteFixtureRepo(t)
	writer, err := New(root, Config{WorkRoot: t.TempDir(), WriteEnabled: true, WriteSource: "unit-runtime", Now: fixedRuntimeNow})
	if err != nil {
		t.Fatal(err)
	}
	indexRevision, _ := writer.currentIndexRevision()
	manifest := testRuntimeManifest("other", "git shorthand", "directory_exists")
	manifest.Environments[0].Checks = []RuntimeCheckInput{{Kind: "git_status"}, {Kind: "git_head"}}
	result := writer.UpdateRuntime(UpdateRuntimeArgs{ProjectID: "other", Runtime: manifest, ExpectedIndexRevision: indexRevision})
	if result["status"] != "committed" {
		t.Fatalf("git shorthand runtime: %#v", result)
	}
	raw, err := os.ReadFile(filepath.Join(root, "projects", "other", "runtime.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if got := strings.Count(text, "target: repository"); got != 2 {
		t.Fatalf("canonical git targets=%d want 2\n%s", got, text)
	}
}

func TestRuntimeObservationIsIdempotentAndDoesNotMoveAuthority(t *testing.T) {
	root := runtimeWriteFixtureRepo(t)
	work := t.TempDir()
	writer, err := New(root, Config{WorkRoot: work, WriteEnabled: true, WriteSource: "unit-runtime", Now: fixedRuntimeNow})
	if err != nil {
		t.Fatal(err)
	}
	before := mustGit(t, root, "rev-parse", "HEAD")
	raw, err := os.ReadFile(filepath.Join(root, "projects", "demo", "runtime.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rev := revision(raw)
	env := "arm-production"
	args := RecordRuntimeObservationArgs{ProjectID: "demo", Observation: "runtime_conflict", HostID: "arm", EnvironmentID: &env, RuntimeResourceRevision: &rev, ClientIdempotencyKey: "obs-unit"}
	result := writer.RecordRuntimeObservation(args)
	if result["status"] != "recorded" || result["idempotent_replay"] != false || result["audit_state"] != "recorded" {
		t.Fatalf("observation result: %#v", result)
	}
	if got := mustGit(t, root, "rev-parse", "HEAD"); got != before {
		t.Fatalf("observation moved HEAD: %s", got)
	}
	replay := writer.RecordRuntimeObservation(args)
	if replay["status"] != "already_recorded" || replay["idempotent_replay"] != true {
		t.Fatalf("observation replay: %#v", replay)
	}
	badRev := "sha256:" + string(make([]byte, 64))
	_ = badRev
	stale := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	conflict := writer.RecordRuntimeObservation(RecordRuntimeObservationArgs{ProjectID: "demo", Observation: "runtime_conflict", HostID: "arm", EnvironmentID: &env, RuntimeResourceRevision: &stale, ClientIdempotencyKey: "obs-stale-unit"})
	if conflict["status"] != "error" || conflict["code"] != "conflict" || conflict["current_revision"] != rev {
		t.Fatalf("observation conflict: %#v", conflict)
	}
	observed := writer.Observe("", "")
	if observed["operation_records"].(map[string]any)["count"] != 0 || observed["journal"].(map[string]any)["pending_count"] != 0 || observed["git_head"] != before {
		t.Fatalf("observation changed Authority tuple: %#v", observed)
	}
	if entries, err := os.ReadDir(filepath.Join(work, "runtime-observations")); err != nil || len(entries) != 1 {
		t.Fatalf("runtime observation durable evidence: entries=%d err=%v", len(entries), err)
	}
}

func fixedRuntimeNow() time.Time { return time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC) }

func testRuntimeManifest(projectID, description, checkKind string) RuntimeManifestInput {
	path := "/workspace/" + projectID
	checkPath := path
	if checkKind == "file_exists" {
		checkPath += "/go.mod"
	}
	return RuntimeManifestInput{
		SchemaVersion: 1,
		ProjectID:     projectID,
		Environments: []RuntimeEnvironmentInput{{
			EnvironmentID: "amd-test", HostID: "amd", Kind: "workspace", Purpose: "test", Description: description,
			Workdir: path, Repository: &RuntimeRepositoryInput{Path: path, Branch: "main"}, Capabilities: []string{"test", "runtime_check"},
			Supervisor: RuntimeSupervisorInput{Kind: "none"}, Checks: []RuntimeCheckInput{{Kind: checkKind, Path: &checkPath}},
			Boundaries: RuntimeBoundariesInput{ProductionWriter: false, MutationRequiresExplicitAuthorization: true},
		}},
		Relations:    []RuntimeRelationInput{},
		Verification: RuntimeVerificationInput{},
	}
}

func assertRuntimeView(t *testing.T, root, projectID, description, wantRevision string) {
	t.Helper()
	reader, err := vaultread.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	projects := reader.Projects()
	head := reader.SourceCommit()
	_ = reader.Close()
	regs := map[string]runtimeview.Registration{}
	for _, p := range projects {
		resource := ""
		if p.RuntimeResource != nil {
			resource = *p.RuntimeResource
		}
		regs[p.ID] = runtimeview.Registration{ProjectID: p.ID, Lifecycle: p.Lifecycle, RuntimeResource: resource}
	}
	view := runtimeview.New(root, head, regs)
	result := view.ProjectRuntime(runtimeview.QueryArgs{ProjectID: projectID, Detail: "summary"})
	if result["status"] != "ok" || result["resource_revision"] != wantRevision || result["environments"].([]map[string]any)[0]["description"] != description {
		t.Fatalf("runtime read after write: %#v", result)
	}
}

func runtimeWriteFixtureRepo(t *testing.T) string {
	t.Helper()
	_, file, _, _ := goruntime.Caller(0)
	src := filepath.Join(filepath.Dir(file), "..", "..", "conformance", "fixtures", "runtime-read")
	dst := filepath.Join(t.TempDir(), "vault")
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dst, "init", "-q", "-b", "main")
	mustGit(t, dst, "config", "user.name", "V-Memory Test")
	mustGit(t, dst, "config", "user.email", "v-memory-test@example.invalid")
	mustGit(t, dst, "add", "-A")
	mustGit(t, dst, "commit", "-q", "-m", "seed")
	return dst
}
