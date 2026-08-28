// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReadDuringMutationUsesOldOwnedSnapshotUntilPublish(t *testing.T) {
	root := fixtureRepo(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	app, err := OpenWithOptions(root, t.TempDir(), OpenOptions{
		WriteEnabled: true, WriteSource: "p4c-slice4",
		TestBeforeViewPublish: func(_, _ string) { once.Do(func() { close(entered); <-release }) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	session := p4cSlice4Session(t, app)
	defer session.Close()

	before := call(t, session, "memory_read", map[string]any{"uri": "memory://projects/demo/progress"})
	oldHead := before["source_commit"].(string)
	token := "SNAPSHOT-PUBLISH-TOKEN"
	resultCh := make(chan map[string]any, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_append_progress", Arguments: map[string]any{"project_id": "demo", "summary": "snapshot publish", "content": token}})
		if err != nil {
			errCh <- err
			return
		}
		out, _ := res.StructuredContent.(map[string]any)
		resultCh <- out
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("mutation did not reach pre-publish hook")
	}
	actualHead := gitOutput(t, root, "rev-parse", "HEAD")
	if actualHead == oldHead {
		t.Fatal("writer had not committed before pre-publish hook")
	}

	during := call(t, session, "memory_read", map[string]any{"uri": "memory://projects/demo/progress"})
	if during["source_commit"] != oldHead || strings.Contains(during["content"].(string), token) {
		t.Fatalf("read observed half-published snapshot: %#v", during)
	}
	search := call(t, session, "memory_search", map[string]any{"query": token, "cross_project": true})
	if search["index_head"] != oldHead || len(search["results"].([]any)) != 0 {
		t.Fatalf("search observed half-published snapshot: %#v", search)
	}
	close(release)
	select {
	case err := <-errCh:
		t.Fatal(err)
	case result := <-resultCh:
		if result["status"] != "committed" {
			t.Fatalf("mutation=%#v", result)
		}
		newHead := result["new_commit"].(string)
		after := call(t, session, "memory_read", map[string]any{"uri": "memory://projects/demo/progress"})
		if after["source_commit"] != newHead || !strings.Contains(after["content"].(string), token) {
			t.Fatalf("new snapshot not published: %#v", after)
		}
		status := call(t, session, "memory_status", map[string]any{})
		if status["owned_head"] != newHead || status["snapshot_state"] != "current" {
			t.Fatalf("status=%#v", status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mutation did not complete")
	}
}

func TestExternalDirtyAndHeadMovementDegradeWithoutAdoption(t *testing.T) {
	root := fixtureRepo(t)
	app, err := OpenWithOptions(root, t.TempDir(), OpenOptions{WriteEnabled: true, WriteSource: "p4c-slice4"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	session := p4cSlice4Session(t, app)
	defer session.Close()
	baseline := call(t, session, "memory_read", map[string]any{"uri": "memory://projects/demo/progress"})
	owned := baseline["source_commit"].(string)
	path := filepath.Join(root, "demo", "进度.md")
	raw, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(raw, []byte("\nDIRTY-EXTERNAL\n")...), 0644); err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(root, "projects", "demo", "runtime.yaml")
	runtimeRaw, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimePath, append(runtimeRaw, []byte("\nforbidden_dirty_field: true\n")...), 0644); err != nil {
		t.Fatal(err)
	}
	runtimeView := call(t, session, "memory_project_runtime", map[string]any{"project_id": "demo", "detail": "summary"})
	if runtimeView["status"] != "ok" || runtimeView["source_commit"] != owned {
		t.Fatalf("dirty runtime leaked into owned view: %#v", runtimeView)
	}
	status := call(t, session, "memory_status", map[string]any{})
	if status["snapshot_state"] != "degraded" || status["owned_head"] != owned {
		t.Fatalf("dirty status=%#v", status)
	}
	read := call(t, session, "memory_read", map[string]any{"uri": "memory://projects/demo/progress"})
	if read["source_commit"] != owned || strings.Contains(read["content"].(string), "DIRTY-EXTERNAL") {
		t.Fatalf("dirty adopted=%#v", read)
	}
	blocked := call(t, session, "memory_append_progress", map[string]any{"project_id": "demo", "summary": "blocked", "content": "blocked dirty"})
	if blocked["status"] != "error" || blocked["rule"] != "snapshot_drift" {
		t.Fatalf("dirty mutation=%#v", blocked)
	}

	git(t, root, "reset", "--hard", owned)
	raw, _ = os.ReadFile(path)
	if err := os.WriteFile(path, append(raw, []byte("\nHEAD-EXTERNAL\n")...), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "demo/进度.md")
	cmd := exec.Command("git", "-C", root, "commit", "-q", "-m", "external head move")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=External", "GIT_AUTHOR_EMAIL=external@example.invalid", "GIT_COMMITTER_NAME=External", "GIT_COMMITTER_EMAIL=external@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	moved := gitOutput(t, root, "rev-parse", "HEAD")
	if moved == owned {
		t.Fatal("HEAD did not move")
	}
	status = call(t, session, "memory_status", map[string]any{})
	if status["snapshot_state"] != "degraded" || status["owned_head"] != owned || status["git_head"] != moved {
		t.Fatalf("head status=%#v", status)
	}
	read = call(t, session, "memory_read", map[string]any{"uri": "memory://projects/demo/progress"})
	if read["source_commit"] != owned || strings.Contains(read["content"].(string), "HEAD-EXTERNAL") {
		t.Fatalf("head silently adopted=%#v", read)
	}
	blocked = call(t, session, "memory_append_progress", map[string]any{"project_id": "demo", "summary": "blocked", "content": "blocked head"})
	if blocked["status"] != "error" || blocked["rule"] != "snapshot_drift" {
		t.Fatalf("head mutation=%#v", blocked)
	}
}

func TestMutationCriticalSectionSerializesWriters(t *testing.T) {
	root := fixtureRepo(t)
	var active atomic.Int32
	var maxActive atomic.Int32
	app, err := OpenWithOptions(root, t.TempDir(), OpenOptions{WriteEnabled: true, WriteSource: "p4c-slice4", TestMutationCriticalSection: func() {
		n := active.Add(1)
		for {
			old := maxActive.Load()
			if n <= old || maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(60 * time.Millisecond)
		active.Add(-1)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	session := p4cSlice4Session(t, app)
	defer session.Close()
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan map[string]any, 2)
	errs := make(chan error, 2)
	for _, body := range []string{"serialized-one", "serialized-two"} {
		body := body
		go func() {
			defer wg.Done()
			res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_append_progress", Arguments: map[string]any{"project_id": "demo", "summary": body, "content": body}})
			if err != nil {
				errs <- err
				return
			}
			out, _ := res.StructuredContent.(map[string]any)
			results <- out
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if maxActive.Load() != 1 {
		t.Fatalf("mutation critical section overlap=%d", maxActive.Load())
	}
	committed := 0
	for r := range results {
		if r["status"] == "committed" {
			committed++
		} else {
			t.Fatalf("mutation failed: %#v", r)
		}
	}
	if committed != 2 {
		t.Fatalf("committed=%d", committed)
	}
}

func TestTwoMutationsSameExpectedRevisionSerializeToCASConflict(t *testing.T) {
	root := fixtureRepo(t)
	app, err := OpenWithOptions(root, t.TempDir(), OpenOptions{WriteEnabled: true, WriteSource: "p4c-slice4", TestMutationCriticalSection: func() { time.Sleep(40 * time.Millisecond) }})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	session := p4cSlice4Session(t, app)
	defer session.Close()
	current := call(t, session, "memory_read", map[string]any{"uri": "memory://projects/demo/progress"})
	rev := current["revision"].(string)
	results := make(chan map[string]any, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for _, body := range []string{"serialized replacement one", "serialized replacement two"} {
		body := body
		go func() {
			defer wg.Done()
			res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_update_sections", Arguments: map[string]any{"project_id": "demo", "role": "progress", "expected_resource_revision": rev, "operations": []any{map[string]any{"operation": "replace", "heading": "Baseline", "content": body}}}})
			if err != nil {
				errs <- err
				return
			}
			out, _ := res.StructuredContent.(map[string]any)
			results <- out
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	committed, conflicts := 0, 0
	for r := range results {
		switch {
		case r["status"] == "committed":
			committed++
		case r["status"] == "error" && r["code"] == "conflict":
			conflicts++
		default:
			t.Fatalf("unexpected result: %#v", r)
		}
	}
	if committed != 1 || conflicts != 1 {
		t.Fatalf("committed=%d conflicts=%d", committed, conflicts)
	}
	status := call(t, session, "memory_status", map[string]any{})
	if status["snapshot_state"] != "current" || status["owned_head"] != gitOutput(t, root, "rev-parse", "HEAD") {
		t.Fatalf("final status=%#v", status)
	}
}

func p4cSlice4Session(t *testing.T, app *App) *mcp.ClientSession {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := app.Server().Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "p4c-slice4", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}
