// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package vaultwrite

import (
	"fmt"
	"os"
)

// Windows does not support os.File.Sync on directory handles. The durable file
// itself is flushed before rename; here we still open and close the parent to
// fail closed if the directory becomes inaccessible.
func syncDurableDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open durable state directory: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("close durable state directory: %w", err)
	}
	return nil
}
