// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package gitview

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResolveCommit resolves requested (or HEAD when empty) to one immutable commit ID.
func ResolveCommit(root, requested string) (string, error) {
	spec := strings.TrimSpace(requested)
	if spec == "" {
		spec = "HEAD"
	}
	out, err := exec.Command("git", "-C", root, "rev-parse", "--verify", spec+"^{commit}").Output()
	if err != nil {
		return "", fmt.Errorf("read Git commit: %w", err)
	}
	commit := strings.TrimSpace(string(out))
	if commit == "" {
		return "", fmt.Errorf("read Git commit: empty revision")
	}
	return commit, nil
}

// ReadFile returns bytes from one relative path at an already-resolved commit.
// It never reads the worktree and invokes Git without a shell.
func ReadFile(root, commit, relativePath string) ([]byte, error) {
	rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relativePath)))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		return nil, fmt.Errorf("invalid Git tree path")
	}
	out, err := exec.Command("git", "-C", root, "show", commit+":"+rel).Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}
