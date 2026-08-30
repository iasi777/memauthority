// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type freezeManifest struct {
	NormativeContract struct {
		Files []struct {
			Path      string `json:"path"`
			SHA256    string `json:"sha256"`
			SizeBytes int    `json:"size_bytes"`
		} `json:"files"`
		IdentitySHA256 string `json:"identity_sha256"`
	} `json:"normative_contract"`
}

func TestFrozenContractManifestIntegrity(t *testing.T) {
	manifests, err := filepath.Glob("../../docs/contract/*/MANIFEST.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) == 0 {
		t.Fatal("no frozen contract manifests found")
	}
	for _, manifestPath := range manifests {
		manifestPath := manifestPath
		t.Run(filepath.Base(filepath.Dir(manifestPath)), func(t *testing.T) {
			raw, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			var manifest freezeManifest
			if err := json.Unmarshal(raw, &manifest); err != nil {
				t.Fatal(err)
			}
			identity := sha256.New()
			for _, file := range manifest.NormativeContract.Files {
				fileRaw, err := os.ReadFile(filepath.Join("../..", filepath.FromSlash(file.Path)))
				if err != nil {
					t.Fatalf("read %s: %v", file.Path, err)
				}
				if len(fileRaw) != file.SizeBytes {
					t.Fatalf("%s size=%d want %d", file.Path, len(fileRaw), file.SizeBytes)
				}
				sum := sha256.Sum256(fileRaw)
				got := hex.EncodeToString(sum[:])
				if got != file.SHA256 {
					t.Fatalf("%s sha256=%s want %s", file.Path, got, file.SHA256)
				}
				fmt.Fprintf(identity, "%s  %s\n", file.SHA256, file.Path)
			}
			if got := hex.EncodeToString(identity.Sum(nil)); got != manifest.NormativeContract.IdentitySHA256 {
				t.Fatalf("contract identity=%s want %s", got, manifest.NormativeContract.IdentitySHA256)
			}
		})
	}
}
