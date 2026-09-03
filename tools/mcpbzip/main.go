// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

// Command mcpbzip creates a deterministic MCPB ZIP archive from a validated staging directory.
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: mcpbzip <staging-dir> <output.mcpb>")
		os.Exit(2)
	}
	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		panic(err)
	}
	out := os.Args[2]

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular bundle entry %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		panic(err)
	}
	sort.Strings(files)

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		panic(err)
	}
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	zw := zip.NewWriter(f)
	fixed := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, rel := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil {
			panic(err)
		}
		hdr := &zip.FileHeader{Name: strings.TrimPrefix(rel, "./"), Method: zip.Deflate}
		hdr.SetModTime(fixed)
		hdr.SetMode(info.Mode().Perm())
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			panic(err)
		}
		src, err := os.Open(path)
		if err != nil {
			panic(err)
		}
		if _, err := io.Copy(w, src); err != nil {
			src.Close()
			panic(err)
		}
		if err := src.Close(); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
}
