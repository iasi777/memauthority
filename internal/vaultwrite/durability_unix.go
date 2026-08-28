// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package vaultwrite

import (
	"fmt"
	"os"
)

func syncDurableDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open durable state directory: %w", err)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("fsync durable state directory: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("close durable state directory: %w", err)
	}
	return nil
}
