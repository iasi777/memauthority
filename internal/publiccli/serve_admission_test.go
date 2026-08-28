// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package publiccli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeAdmissionRequiresCleanValidVaultAndExternalStateDir(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "vault")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", vault}, &out, &errOut); code != 0 {
		t.Fatalf("init=%d stderr=%s", code, errOut.String())
	}
	state := filepath.Join(t.TempDir(), "state")
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"serve", "--vault", vault, "--state-dir", state}, &out, &errOut); code != 0 {
		t.Fatalf("serve empty stdio=%d stderr=%s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(state, "read-index.sqlite")); err != nil {
		t.Fatalf("state projection missing: %v", err)
	}

	if err := os.WriteFile(filepath.Join(vault, "README.md"), []byte("dirty\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"serve", "--vault", vault, "--state-dir", state}, &out, &errOut); code != 1 {
		t.Fatalf("dirty serve=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "clean") {
		t.Fatalf("dirty refusal=%q", errOut.String())
	}
	_ = os.Remove(filepath.Join(vault, "README.md"))

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"serve", "--vault", vault, "--state-dir", filepath.Join(vault, "state")}, &out, &errOut); code != 1 {
		t.Fatalf("inside state=%d stderr=%s", code, errOut.String())
	}
}

func TestServeRejectsInvalidCommittedVault(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "vault")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", vault}, &out, &errOut); code != 0 {
		t.Fatal(code)
	}
	if err := os.WriteFile(filepath.Join(vault, "INDEX.yaml"), []byte("schema_version: 4\nprojects:\n  bad.project: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", vault, "add", "INDEX.yaml")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("add: %v %s", e, b)
	}
	cmd = exec.Command("git", "-C", vault, "commit", "-q", "-m", "invalid")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=X", "GIT_AUTHOR_EMAIL=x@example.invalid", "GIT_COMMITTER_NAME=X", "GIT_COMMITTER_EMAIL=x@example.invalid")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("commit: %v %s", e, b)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"serve", "--vault", vault, "--state-dir", filepath.Join(t.TempDir(), "state")}, &out, &errOut); code != 1 {
		t.Fatalf("invalid serve=%d stderr=%s", code, errOut.String())
	}
}
