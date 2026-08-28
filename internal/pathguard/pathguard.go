// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package pathguard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Contains reports whether candidate resolves to root or a descendant of root.
// Missing suffix components are allowed; all existing path prefixes are
// canonicalized through symlinks before containment is evaluated.
func Contains(root, candidate string) (bool, error) {
	canonicalRoot, err := canonical(root)
	if err != nil {
		return false, err
	}
	canonicalCandidate, err := canonical(candidate)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(canonicalRoot, canonicalCandidate)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

func canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	probe := abs
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", err
		}
		suffix = append(suffix, filepath.Base(probe))
		probe = parent
	}
}
