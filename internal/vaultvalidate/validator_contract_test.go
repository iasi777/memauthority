// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultvalidate

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestValidDirtyCandidateAndNonCanonicalTextPasses(t *testing.T) {
	root := validationFixture(t)
	p := filepath.Join(root, "demo", "规范.md")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.ReplaceAll(string(raw), "\n", "\r\n") + "Cafe\u0301   \r\n")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(root)
	if !report.Valid || len(report.Errors) != 0 {
		t.Fatalf("dirty/noncanonical candidate should remain structurally valid: %#v", report)
	}
}

func TestAggregatesStableSecretSafeErrors(t *testing.T) {
	root := validationFixture(t)
	rules := filepath.Join(root, "demo", "规范.md")
	raw, err := os.ReadFile(rules)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(raw), "说明: Conformance rules fixture.\n", "", 1)
	text = strings.Replace(text, "# 规范\n", "# Notes\n", 1)
	if err := os.WriteFile(rules, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo", "random-memory.md"), []byte("ignored?\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "README-link")); err != nil {
		t.Fatal(err)
	}
	progress := filepath.Join(root, "demo", "进度.md")
	prow, err := os.ReadFile(progress)
	if err != nil {
		t.Fatal(err)
	}
	secretValue := "pass" + "word: p4c-slice2-sentinel"
	if err := os.WriteFile(progress, append(prow, []byte("\n"+secretValue+"\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	report := Validate(root)
	if report.Valid {
		t.Fatalf("invalid tree passed: %#v", report)
	}
	want := map[string]bool{"frontmatter_required": false, "role_heading": false, "project_namespace": false, "symlink_forbidden": false, "secret_detected": false}
	for _, f := range report.Errors {
		if _, ok := want[f.Code]; ok {
			want[f.Code] = true
		}
	}
	for code, seen := range want {
		if !seen {
			t.Fatalf("missing %s in %#v", code, report.Errors)
		}
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "p4c-slice2-sentinel") {
		t.Fatal("validator leaked secret material")
	}
	for i := 1; i < len(report.Errors); i++ {
		if findingSortKey(report.Errors[i-1]) > findingSortKey(report.Errors[i]) {
			t.Fatalf("findings unstable: %#v", report.Errors)
		}
	}
}

func TestRuntimeAndCrossFileConsistency(t *testing.T) {
	root := validationFixture(t)
	idx := filepath.Join(root, "INDEX.yaml")
	raw, err := os.ReadFile(idx)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(raw), "runtime_resource: null", "runtime_resource: projects/demo/runtime.yaml", 1)
	text = strings.Replace(text, "last_verified: '2026-08-11'", "last_verified: '2026-08-10'", 1)
	if err := os.WriteFile(idx, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(root)
	codes := map[string]bool{}
	for _, f := range report.Errors {
		codes[f.Code] = true
	}
	if !codes["runtime_missing"] || !codes["index_last_verified_mismatch"] {
		t.Fatalf("runtime/cross-file errors: %#v", report.Errors)
	}
}

func TestUnregisteredProjectLikeDirectoryRejectedButSupportAllowed(t *testing.T) {
	root := validationFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tools", "note.md"), []byte("tooling\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if report := Validate(root); !report.Valid {
		t.Fatalf("support root rejected: %#v", report.Errors)
	}
	rogue := filepath.Join(root, "forgotten")
	if err := os.MkdirAll(rogue, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rogue, "交接.md"), []byte("forgotten\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(root)
	found := false
	for _, f := range report.Errors {
		if f.Code == "unregistered_project_directory" {
			found = true
		}
	}
	if !found {
		t.Fatalf("forgotten project-like directory accepted: %#v", report.Errors)
	}
}

func findingSortKey(f Finding) string {
	return f.Path + "\x00" + fmtInt(f.Line) + "\x00" + f.Code + "\x00" + f.Field
}
func fmtInt(v int) string { s := strconv.Itoa(v); return strings.Repeat("0", 9-len(s)) + s }

func validationFixture(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(file), "..", "..", "conformance", "fixtures", "route-read-revision")
	dst := filepath.Join(t.TempDir(), "vault")
	if err := copyTreeForValidation(src, dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

func copyTreeForValidation(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
